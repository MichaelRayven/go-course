package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTicker struct {
	t *time.Ticker
}

func (r *realTicker) Chan() <-chan time.Time {
	return r.t.C
}

func (r *realTicker) Stop() {
	r.t.Stop()
}

func NewRealTicker(d time.Duration) Ticker {
	return &realTicker{t: time.NewTicker(d)}
}

type Scanner struct {
	repo       staleTripRepository
	queue      *Queue
	staleAfter time.Duration
	ticker     Ticker
	logger     *slog.Logger
	tracer     trace.Tracer
	now        func() time.Time

	inProgress atomic.Bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	startOnce  sync.Once
	stopOnce   sync.Once
}

func NewScanner(
	repo staleTripRepository,
	queue *Queue,
	staleAfter time.Duration,
	ticker Ticker,
	tp trace.TracerProvider,
	logger *slog.Logger,
	now func() time.Time,
) (*Scanner, error) {
	if repo == nil {
		return nil, fmt.Errorf("stale trip repository must not be nil")
	}
	if queue == nil {
		return nil, fmt.Errorf("queue must not be nil")
	}
	if staleAfter <= 0 {
		return nil, fmt.Errorf("stale after duration must be greater than zero, got %v", staleAfter)
	}
	if ticker == nil {
		return nil, fmt.Errorf("ticker must not be nil")
	}
	if tp == nil {
		tp = nooptrace.NewTracerProvider()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &Scanner{
		repo:       repo,
		queue:      queue,
		staleAfter: staleAfter,
		ticker:     ticker,
		logger:     logger,
		tracer:     tp.Tracer(tracerName),
		now:        now,
	}, nil
}

func (s *Scanner) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		scanCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel

		s.wg.Add(1)
		go s.run(scanCtx)
	})
}

func (s *Scanner) Stop() {
	s.stopOnce.Do(func() {
		if s.ticker != nil {
			s.ticker.Stop()
		}
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *Scanner) run(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-s.ticker.Chan():
			if !ok {
				return
			}
			s.ScanTick(ctx, t)
		}
	}
}

// ScanTick executes a single scan cycle.
func (s *Scanner) ScanTick(ctx context.Context, tickTime time.Time) {
	if !s.inProgress.CompareAndSwap(false, true) {
		s.logger.WarnContext(ctx, "scanner tick skipped: previous tick still in progress")
		return
	}
	defer s.inProgress.Store(false)

	threshold := tickTime.Add(-s.staleAfter)

	ctx, span := s.tracer.Start(ctx, "positions.scanner.tick",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("threshold", threshold.Format(time.RFC3339)),
		),
	)
	defer span.End()

	trips, err := s.repo.FetchStaleTrips(ctx, threshold, 0)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to fetch stale trips", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	if len(trips) == 0 {
		span.SetStatus(codes.Ok, "")
		return
	}

	scanSpanCtx := span.SpanContext()

	var queued, duplicate, dropped int
	for _, tr := range trips {
		job := Job{
			TripID:      tr.ID,
			DriverID:    tr.DriverID,
			ScanSpanCtx: scanSpanCtx,
		}
		res := s.queue.Enqueue(ctx, job)
		switch res {
		case EnqueueResultQueued:
			queued++
		case EnqueueResultDuplicate:
			duplicate++
		case EnqueueResultDropped:
			dropped++
			s.logger.WarnContext(ctx, "positions queue is full, dropping stale trip check",
				"trip_id", tr.ID,
				"driver_id", tr.DriverID,
			)
		}
	}

	s.logger.InfoContext(ctx, "stale trips scan completed",
		"found", len(trips),
		"queued", queued,
		"duplicate", duplicate,
		"dropped", dropped,
	)
	span.SetStatus(codes.Ok, "")
}
