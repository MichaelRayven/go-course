package positions

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const (
	meterScope = "github.com/MichaelRayven/go-course/internal/worker/positions"
	tracerName = "github.com/MichaelRayven/go-course/internal/worker/positions"
)

type EnqueueResult string

const (
	EnqueueResultQueued    EnqueueResult = "queued"
	EnqueueResultDuplicate EnqueueResult = "duplicate"
	EnqueueResultDropped   EnqueueResult = "dropped"
)

type Job struct {
	TripID      uuid.UUID
	DriverID    uuid.UUID
	ScanSpanCtx trace.SpanContext
}

type Queue struct {
	mu              sync.Mutex
	ch              chan Job
	inFlight        map[uuid.UUID]context.CancelFunc
	closed          bool
	staleChecks     metric.Int64Counter
	queueSize       metric.Int64UpDownCounter
	requestsCounter metric.Int64Counter
}

func NewQueue(capacity int, mp metric.MeterProvider) (*Queue, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("queue capacity must be greater than zero, got %d", capacity)
	}

	if mp == nil {
		mp = noopmetric.NewMeterProvider()
	}

	meter := mp.Meter(meterScope)

	staleChecks, err := meter.Int64Counter(
		"tripgo.stale_checks",
		metric.WithUnit("1"),
		metric.WithDescription("Result of stale trip checks."),
	)
	if err != nil {
		return nil, fmt.Errorf("create stale_checks counter: %w", err)
	}

	queueSize, err := meter.Int64UpDownCounter(
		"tripgo.position_requests.queue.size",
		metric.WithUnit("1"),
		metric.WithDescription("Current items in positions queue."),
	)
	if err != nil {
		return nil, fmt.Errorf("create queue size counter: %w", err)
	}

	requestsCounter, err := meter.Int64Counter(
		"tripgo.position_requests",
		metric.WithUnit("1"),
		metric.WithDescription("Number of location push requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create position_requests counter: %w", err)
	}

	return &Queue{
		ch:              make(chan Job, capacity),
		inFlight:        make(map[uuid.UUID]context.CancelFunc),
		staleChecks:     staleChecks,
		queueSize:       queueSize,
		requestsCounter: requestsCounter,
	}, nil
}

func (q *Queue) Enqueue(ctx context.Context, job Job) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		q.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultDropped))))
		q.requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultDropped))))
		return EnqueueResultDropped
	}

	if _, exists := q.inFlight[job.TripID]; exists {
		q.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultDuplicate))))
		return EnqueueResultDuplicate
	}

	select {
	case q.ch <- job:
		q.inFlight[job.TripID] = nil
		q.queueSize.Add(ctx, 1)
		q.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultQueued))))
		return EnqueueResultQueued
	default:
		q.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultDropped))))
		q.requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", string(EnqueueResultDropped))))
		return EnqueueResultDropped
	}
}

// Cancel removes tripID from inFlight registry and cancels any running request when a fresh coordinate arrives.
func (q *Queue) Cancel(tripID uuid.UUID) {
	q.mu.Lock()
	cancel, exists := q.inFlight[tripID]
	if exists {
		delete(q.inFlight, tripID)
	}
	q.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// StartJob checks if tripID is still in-flight and registers its cancellation callback.
// Returns true if valid, false if it was cancelled while in queue.
func (q *Queue) StartJob(tripID uuid.UUID, cancel context.CancelFunc) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.inFlight[tripID]; !exists {
		return false
	}
	q.inFlight[tripID] = cancel
	return true
}

// FinishJob removes tripID from inFlight registry after processing completes.
func (q *Queue) FinishJob(tripID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.inFlight, tripID)
}

// DecSize decrements the queue size metric when a job is picked by a worker.
func (q *Queue) DecSize(ctx context.Context) {
	q.queueSize.Add(ctx, -1)
}

// Jobs returns the read-only channel of jobs.
func (q *Queue) Jobs() <-chan Job {
	return q.ch
}

// Closes the queue channel and marks the queue as closed.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}
	q.closed = true
	close(q.ch)
}

// InFlightCount returns the current number of in-flight trips (useful for testing).
func (q *Queue) InFlightCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.inFlight)
}

// Len returns the number of buffered items currently in the channel.
func (q *Queue) Len() int {
	return len(q.ch)
}
