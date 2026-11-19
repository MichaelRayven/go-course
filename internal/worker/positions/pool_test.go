package positions_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	positions "github.com/MichaelRayven/go-course/internal/worker/positions"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestQueueEnqueueAndDeduplication(t *testing.T) {
	t.Parallel()

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))

	q, err := positions.NewQueue(10, mp)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	tripID1 := uuid.New()
	driverID1 := uuid.New()
	tripID2 := uuid.New()
	driverID2 := uuid.New()

	ctx := context.Background()

	// 1. First enqueue -> Queued
	res := q.Enqueue(ctx, positions.Job{TripID: tripID1, DriverID: driverID1})
	if res != positions.EnqueueResultQueued {
		t.Errorf("got %v, want %v", res, positions.EnqueueResultQueued)
	}

	// 2. Duplicate enqueue -> Duplicate
	res = q.Enqueue(ctx, positions.Job{TripID: tripID1, DriverID: driverID1})
	if res != positions.EnqueueResultDuplicate {
		t.Errorf("got %v, want %v", res, positions.EnqueueResultDuplicate)
	}

	// 3. Different trip -> Queued
	res = q.Enqueue(ctx, positions.Job{TripID: tripID2, DriverID: driverID2})
	if res != positions.EnqueueResultQueued {
		t.Errorf("got %v, want %v", res, positions.EnqueueResultQueued)
	}

	if q.Len() != 2 {
		t.Errorf("queue len = %d, want 2", q.Len())
	}
	if q.InFlightCount() != 2 {
		t.Errorf("in-flight count = %d, want 2", q.InFlightCount())
	}

	// Verify stale_checks metrics
	var rm metricdata.ResourceMetrics
	if err := metricReader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	counts := make(map[string]int64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "tripgo.stale_checks" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if attr.Key == "result" {
								counts[attr.Value.AsString()] += dp.Value
							}
						}
					}
				}
			}
		}
	}

	if counts["queued"] != 2 {
		t.Errorf("stale_checks{result=queued} = %d, want 2", counts["queued"])
	}
	if counts["duplicate"] != 1 {
		t.Errorf("stale_checks{result=duplicate} = %d, want 1", counts["duplicate"])
	}
}

func TestQueueOverflow(t *testing.T) {
	t.Parallel()

	q, err := positions.NewQueue(2, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	ctx := context.Background()

	// Fill queue capacity
	if res := q.Enqueue(ctx, positions.Job{TripID: uuid.New(), DriverID: uuid.New()}); res != positions.EnqueueResultQueued {
		t.Errorf("item 1: got %v, want %v", res, positions.EnqueueResultQueued)
	}
	if res := q.Enqueue(ctx, positions.Job{TripID: uuid.New(), DriverID: uuid.New()}); res != positions.EnqueueResultQueued {
		t.Errorf("item 2: got %v, want %v", res, positions.EnqueueResultQueued)
	}

	// 3rd item should drop without blocking
	if res := q.Enqueue(ctx, positions.Job{TripID: uuid.New(), DriverID: uuid.New()}); res != positions.EnqueueResultDropped {
		t.Errorf("item 3: got %v, want %v", res, positions.EnqueueResultDropped)
	}
}

func TestPoolCancellationBeforePickup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)
	// Gateway should NEVER be called for cancelled trip!
	mockGateway.EXPECT().SendPositionRequest(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))

	q, err := positions.NewQueue(10, mp)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(2, 500*time.Millisecond, q, mockGateway, nil, mp, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	tripID := uuid.New()
	driverID := uuid.New()

	ctx := context.Background()
	q.Enqueue(ctx, positions.Job{TripID: tripID, DriverID: driverID})

	// Coordinate arrives while waiting in queue:
	q.Cancel(tripID)

	pool.Start(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := pool.Stop(stopCtx); err != nil {
		t.Fatalf("pool.Stop: %v", err)
	}

	// Check cancellation metric
	var rm metricdata.ResourceMetrics
	if err := metricReader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}

	var cancelledCount int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "tripgo.position_requests" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if attr.Key == "result" && attr.Value.AsString() == "cancelled" {
								cancelledCount += dp.Value
							}
						}
					}
				}
			}
		}
	}

	if cancelledCount != 1 {
		t.Errorf("tripgo.position_requests{result=cancelled} = %d, want 1", cancelledCount)
	}
}

func TestPoolProcessesJobsConcurrently(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)

	const totalJobs = 6
	var processed atomic.Int64

	mockGateway.EXPECT().
		SendPositionRequest(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, recipientID, tripID uuid.UUID) error {
			time.Sleep(20 * time.Millisecond) // simulate work
			processed.Add(1)
			return nil
		}).
		Times(totalJobs)

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(3, 1*time.Second, q, mockGateway, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < totalJobs; i++ {
		q.Enqueue(ctx, positions.Job{
			TripID:   uuid.New(),
			DriverID: uuid.New(),
		})
	}

	pool.Start(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := pool.Stop(stopCtx); err != nil {
		t.Fatalf("pool.Stop: %v", err)
	}

	if got := processed.Load(); got != totalJobs {
		t.Errorf("processed %d jobs, want %d", got, totalJobs)
	}
	if q.InFlightCount() != 0 {
		t.Errorf("in-flight count = %d, want 0 after completion", q.InFlightCount())
	}
}

func TestPoolFaultTolerance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)

	// Even if gateway returns an error, worker should not crash and should continue
	mockGateway.EXPECT().
		SendPositionRequest(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("push service 503")).
		Times(1)

	mockGateway.EXPECT().
		SendPositionRequest(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(1, 500*time.Millisecond, q, mockGateway, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx := context.Background()
	q.Enqueue(ctx, positions.Job{TripID: uuid.New(), DriverID: uuid.New()})
	q.Enqueue(ctx, positions.Job{TripID: uuid.New(), DriverID: uuid.New()})

	pool.Start(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := pool.Stop(stopCtx); err != nil {
		t.Fatalf("pool.Stop: %v", err)
	}

	if q.InFlightCount() != 0 {
		t.Errorf("in-flight count = %d, want 0", q.InFlightCount())
	}
}

func TestPoolSpanLinks(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)
	mockGateway.EXPECT().SendPositionRequest(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	// Create a dummy scan span context
	tracer := tp.Tracer("scanner.test")
	scanCtx, scanSpan := tracer.Start(context.Background(), "scanner.tick")
	_ = scanCtx
	scanSpanCtx := scanSpan.SpanContext()
	scanSpan.End()

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(1, 500*time.Millisecond, q, mockGateway, tp, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	ctx := context.Background()
	q.Enqueue(ctx, positions.Job{
		TripID:      uuid.New(),
		DriverID:    uuid.New(),
		ScanSpanCtx: scanSpanCtx,
	})

	pool.Start(ctx)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := pool.Stop(stopCtx); err != nil {
		t.Fatalf("pool.Stop: %v", err)
	}

	spans := spanRecorder.Ended()
	var workerSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == "positions.worker.process" {
			workerSpan = s
			break
		}
	}

	if workerSpan == nil {
		t.Fatal("worker span not found")
	}

	links := workerSpan.Links()
	if len(links) != 1 {
		t.Fatalf("worker span expected 1 link, got %d", len(links))
	}
	if links[0].SpanContext.TraceID() != scanSpanCtx.TraceID() {
		t.Errorf("got link TraceID %v, want %v", links[0].SpanContext.TraceID(), scanSpanCtx.TraceID())
	}
}

func TestPoolCancelInFlightRequest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(1, 5*time.Second, q, mockGateway, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	tripID := uuid.New()
	driverID := uuid.New()

	started := make(chan struct{})
	cancelled := make(chan struct{})

	mockGateway.EXPECT().
		SendPositionRequest(gomock.Any(), driverID, tripID).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return ctx.Err()
		})

	ctx := context.Background()
	pool.Start(ctx)

	if res := q.Enqueue(ctx, positions.Job{TripID: tripID, DriverID: driverID}); res != positions.EnqueueResultQueued {
		t.Fatalf("expected queued, got %v", res)
	}

	<-started
	q.Cancel(tripID)

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("in-flight push request was not cancelled")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := pool.Stop(stopCtx); err != nil {
		t.Fatalf("pool.Stop: %v", err)
	}
}

func TestPoolStopCancelsSlowRequestOnTimeout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGateway := NewMockpushGateway(ctrl)

	q, err := positions.NewQueue(10, nil)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	pool, err := positions.NewPool(1, 5*time.Second, q, mockGateway, nil, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	tripID := uuid.New()
	driverID := uuid.New()
	started := make(chan struct{})

	mockGateway.EXPECT().
		SendPositionRequest(gomock.Any(), driverID, tripID).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})

	ctx := context.Background()
	pool.Start(ctx)

	if res := q.Enqueue(ctx, positions.Job{TripID: tripID, DriverID: driverID}); res != positions.EnqueueResultQueued {
		t.Fatalf("expected queued, got %v", res)
	}

	<-started

	stopCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	if err := pool.Stop(stopCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}
