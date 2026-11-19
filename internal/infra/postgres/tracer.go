package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/MichaelRayven/go-course/internal/infra/postgres"

// collectionPattern finds the table a statement works on. Our queries are
// built by squirrel and always use one of these forms.
var collectionPattern = regexp.MustCompile(`(?i)\b(?:from|into|update)\s+"?([a-z_][a-z0-9_]*)`)

type QueryTracer struct {
	spans    *otelpgx.Tracer
	duration metric.Float64Histogram
}

var _ pgx.QueryTracer = (*QueryTracer)(nil)

func NewQueryTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*QueryTracer, error) {
	duration, err := mp.Meter(instrumentationScope).Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create db duration histogram: %w", err)
	}

	return &QueryTracer{
		spans: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tp),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		duration: duration,
	}, nil
}

type queryKey struct{}

type queryInfo struct {
	start time.Time
	attrs []attribute.KeyValue
}

func (t *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.spans.TraceQueryStart(ctx, conn, data)

	attrs := []attribute.KeyValue{semconv.DBOperationName(otelpgx.SQLOperationName(ctx, data.SQL))}
	if collection := collectionName(data.SQL); collection != "" {
		attrs = append(attrs, semconv.DBCollectionName(collection))
	}
	trace.SpanFromContext(ctx).SetAttributes(attrs...)

	return context.WithValue(ctx, queryKey{}, queryInfo{start: time.Now(), attrs: attrs})
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	info, _ := ctx.Value(queryKey{}).(queryInfo)
	attrs := append([]attribute.KeyValue{}, info.attrs...)

	if data.Err != nil {
		errType := semconv.ErrorTypeKey.String(errorType(data.Err))
		attrs = append(attrs, errType)
		trace.SpanFromContext(ctx).SetAttributes(errType)
	}

	// Ends the span and marks it codes.Error when data.Err is set.
	t.spans.TraceQueryEnd(ctx, conn, data)

	if !info.start.IsZero() {
		t.duration.Record(ctx, time.Since(info.start).Seconds(), metric.WithAttributes(attrs...))
	}
}

func collectionName(sql string) string {
	if m := collectionPattern.FindStringSubmatch(sql); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

func errorType(err error) string {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return pgErr.Code
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "_OTHER"
	}
}
