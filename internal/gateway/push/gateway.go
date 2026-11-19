package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

const (
	meterScope = "github.com/MichaelRayven/go-course/internal/gateway/push"
	tracerName = "github.com/MichaelRayven/go-course/internal/gateway/push"
)

// HTTPError is returned when Push Service responds with a non-202 status code.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("push service returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("push service returned status %d: %s", e.StatusCode, e.Body)
}

type pushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data,omitempty"`
}

type Gateway struct {
	client     *http.Client
	endpoint   string
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
	requests   metric.Int64Counter
	duration   metric.Float64Histogram
}

func New(
	client *http.Client,
	baseURL string,
	tp trace.TracerProvider,
	mp metric.MeterProvider,
	propagator propagation.TextMapPropagator,
) (*Gateway, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if tp == nil {
		tp = nooptrace.NewTracerProvider()
	}
	if mp == nil {
		mp = noopmetric.NewMeterProvider()
	}
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	meter := mp.Meter(meterScope)

	requests, err := meter.Int64Counter(
		"tripgo.position_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of location push requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create requests counter: %w", err)
	}

	duration, err := meter.Float64Histogram(
		"tripgo.position_requests.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of location push HTTP requests."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create duration histogram: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/push"

	return &Gateway{
		client:     client,
		endpoint:   endpoint,
		tracer:     tp.Tracer(tracerName),
		propagator: propagator,
		requests:   requests,
		duration:   duration,
	}, nil
}

func (g *Gateway) SendPositionRequest(ctx context.Context, recipientID, tripID uuid.UUID) error {
	ctx, span := g.tracer.Start(ctx, "push.send_request",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPRequestMethodKey.String(http.MethodPost),
			semconv.URLFull(g.endpoint),
		),
	)
	defer span.End()

	reqBody := pushRequest{
		RequestID:   uuid.NewString(),
		RecipientID: recipientID,
		Kind:        "REQUEST_POSITION",
		Data: map[string]string{
			"trip_id": tripID.String(),
		},
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal push request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(raw))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create push request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	g.propagator.Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	start := time.Now()
	resp, err := g.client.Do(httpReq)
	duration := time.Since(start).Seconds()

	if err != nil {
		result := "error"
		if errors.Is(err, context.Canceled) {
			result = "cancelled"
		}
		g.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
		g.duration.Record(ctx, duration, metric.WithAttributes(attribute.String("result", result)))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("send push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		httpErr := &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(snippet)),
		}
		g.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "error")))
		g.duration.Record(ctx, duration, metric.WithAttributes(attribute.String("result", "error")))
		span.RecordError(httpErr)
		span.SetStatus(codes.Error, httpErr.Error())
		return httpErr
	}

	_, _ = io.Copy(io.Discard, resp.Body)

	g.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "success")))
	g.duration.Record(ctx, duration, metric.WithAttributes(attribute.String("result", "success")))
	span.SetStatus(codes.Ok, "")
	return nil
}

func (g *Gateway) Close() error {
	g.client.CloseIdleConnections()
	return nil
}
