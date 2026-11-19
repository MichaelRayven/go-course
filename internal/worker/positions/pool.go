package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type Pool struct {
	workersCount    int
	pushTimeout     time.Duration
	queue           *Queue
	pushGateway     pushGateway
	logger          *slog.Logger
	tracer          trace.Tracer
	requestsCounter metric.Int64Counter
	activeWorkers   metric.Int64UpDownCounter

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewPool(
	workersCount int,
	pushTimeout time.Duration,
	queue *Queue,
	gateway pushGateway,
	tp trace.TracerProvider,
	mp metric.MeterProvider,
	logger *slog.Logger,
) (*Pool, error) {
	if workersCount <= 0 {
		return nil, fmt.Errorf("workers count must be greater than zero, got %d", workersCount)
	}
	if pushTimeout <= 0 {
		return nil, fmt.Errorf("push timeout must be greater than zero, got %v", pushTimeout)
	}
	if queue == nil {
		return nil, fmt.Errorf("queue must not be nil")
	}
	if gateway == nil {
		return nil, fmt.Errorf("push gateway must not be nil")
	}
	if tp == nil {
		tp = nooptrace.NewTracerProvider()
	}
	if mp == nil {
		mp = noopmetric.NewMeterProvider()
	}
	if logger == nil {
		logger = slog.Default()
	}

	meter := mp.Meter(meterScope)

	requestsCounter, err := meter.Int64Counter(
		"tripgo.position_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of location push requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create position_requests counter: %w", err)
	}

	activeWorkers, err := meter.Int64UpDownCounter(
		"tripgo.position_requests.workers.active",
		metric.WithUnit("1"),
		metric.WithDescription("Currently active positions workers."),
	)
	if err != nil {
		return nil, fmt.Errorf("create active workers counter: %w", err)
	}

	return &Pool{
		workersCount:    workersCount,
		pushTimeout:     pushTimeout,
		queue:           queue,
		pushGateway:     gateway,
		logger:          logger,
		tracer:          tp.Tracer(tracerName),
		requestsCounter: requestsCounter,
		activeWorkers:   activeWorkers,
	}, nil
}

func (p *Pool) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		p.ctx, p.cancel = context.WithCancel(ctx)

		for i := 0; i < p.workersCount; i++ {
			p.wg.Add(1)
			go p.worker(p.ctx, i)
		}
	})
}

func (p *Pool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.queue.Close()
	})

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if p.cancel != nil {
			p.cancel()
		}
		<-done
		return ctx.Err()
	}
}

func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.queue.Jobs():
			if !ok {
				return
			}
			p.queue.DecSize(ctx)
			p.processJob(ctx, job)
		}
	}
}

func (p *Pool) processJob(ctx context.Context, job Job) {
	if p.activeWorkers != nil {
		p.activeWorkers.Add(ctx, 1)
		defer p.activeWorkers.Add(ctx, -1)
	}

	// 1. Worker span linked to ScanSpanCtx
	var spanOpts []trace.SpanStartOption
	spanOpts = append(spanOpts, trace.WithSpanKind(trace.SpanKindInternal))
	if job.ScanSpanCtx.IsValid() {
		spanOpts = append(spanOpts, trace.WithLinks(trace.Link{SpanContext: job.ScanSpanCtx}))
	}

	jobCtx, span := p.tracer.Start(ctx, "positions.worker.process", spanOpts...)
	defer span.End()

	// 2. Check if job was cancelled while waiting in queue and register in-flight cancel
	reqCtx, cancel := context.WithTimeout(jobCtx, p.pushTimeout)
	defer cancel()

	if !p.queue.StartJob(job.TripID, cancel) {
		if p.requestsCounter != nil {
			p.requestsCounter.Add(jobCtx, 1, metric.WithAttributes(attribute.String("result", "cancelled")))
		}
		p.logger.InfoContext(jobCtx, "stale trip push cancelled due to new position", "trip_id", job.TripID)
		return
	}
	defer p.queue.FinishJob(job.TripID)

	// 3. Dispatch to Push Service with timeout
	if err := p.pushGateway.SendPositionRequest(reqCtx, job.DriverID, job.TripID); err != nil {
		p.logger.WarnContext(jobCtx, "failed to send position request",
			"trip_id", job.TripID,
			"driver_id", job.DriverID,
			"error", err,
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	span.SetStatus(codes.Ok, "")
}
