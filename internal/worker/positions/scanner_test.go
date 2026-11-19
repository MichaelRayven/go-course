package positions_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
	positions "github.com/MichaelRayven/go-course/internal/worker/positions"
)

type fakeTicker struct {
	ch chan time.Time
	mu sync.Mutex
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{ch: make(chan time.Time, 10)}
}

func (f *fakeTicker) Chan() <-chan time.Time {
	return f.ch
}

func (f *fakeTicker) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
}

func (f *fakeTicker) Tick(t time.Time) {
	f.ch <- t
}

func TestScannerEnqueuesStaleTrips(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockstaleTripRepository(ctrl)

	trip1 := triprepo.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), LastSeen: time.Now().Add(-2 * time.Minute)}
	trip2 := triprepo.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), LastSeen: time.Now().Add(-3 * time.Minute)}

	mockRepo.EXPECT().
		FetchStaleTrips(gomock.Any(), gomock.Any(), 0).
		Return([]triprepo.StaleTrip{trip1, trip2}, nil).
		Times(1)

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	ticker := newFakeTicker()
	fixedNow := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	scanner, err := positions.NewScanner(
		mockRepo,
		q,
		60*time.Second,
		ticker,
		tp,
		slog.Default(),
		func() time.Time { return fixedNow },
	)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	ctx := context.Background()
	scanner.ScanTick(ctx, fixedNow)

	if q.Len() != 2 {
		t.Fatalf("queue len = %d, want 2", q.Len())
	}

	// Verify span created
	spans := spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "positions.scanner.tick" {
		t.Errorf("got span name %s, want positions.scanner.tick", spans[0].Name())
	}
}

func TestScannerHandlesRepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockstaleTripRepository(ctrl)

	mockRepo.EXPECT().
		FetchStaleTrips(gomock.Any(), gomock.Any(), 0).
		Return(nil, errors.New("db connection failure")).
		Times(1)

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	ticker := newFakeTicker()
	scanner, err := positions.NewScanner(mockRepo, q, 60*time.Second, ticker, nil, slog.Default(), nil)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	// ScanTick should not panic and should handle error cleanly
	scanner.ScanTick(context.Background(), time.Now())

	if q.Len() != 0 {
		t.Errorf("queue len = %d, want 0", q.Len())
	}
}

func TestScannerStartStopWithTicker(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := NewMockstaleTripRepository(ctrl)

	mockRepo.EXPECT().
		FetchStaleTrips(gomock.Any(), gomock.Any(), 0).
		Return([]triprepo.StaleTrip{}, nil).
		AnyTimes()

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	ticker := newFakeTicker()
	scanner, err := positions.NewScanner(mockRepo, q, 60*time.Second, ticker, nil, slog.Default(), nil)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scanner.Start(ctx)

	// Trigger tick via fake ticker
	ticker.Tick(time.Now())

	// Give a moment for tick to be handled
	time.Sleep(10 * time.Millisecond)

	scanner.Stop()
}
