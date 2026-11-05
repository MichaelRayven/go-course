package middleware

import (
	"fmt"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const meterScope = "github.com/MichaelRayven/go-course/internal/http/middleware"

func Metrics(mp metric.MeterProvider, route RouteFunc) (func(http.Handler) http.Handler, error) {
	meter := mp.Meter(meterScope)

	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create request duration histogram: %w", err)
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of in-flight HTTP server requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create active requests counter: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			start := time.Now()

			attrs := []attribute.KeyValue{semconv.HTTPRequestMethodKey.String(r.Method)}
			if pattern := route(r); pattern != "" {
				attrs = append(attrs, semconv.HTTPRoute(pattern))
			}
			activeAttrs := metric.WithAttributes(attrs...)

			active.Add(ctx, 1, activeAttrs)
			defer active.Add(ctx, -1, activeAttrs)

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
				append(attrs, semconv.HTTPResponseStatusCode(status(ww)))...,
			))
		})
	}, nil
}

// status treats a handler that never called WriteHeader as 200, like net/http does.
func status(ww chimw.WrapResponseWriter) int {
	if s := ww.Status(); s != 0 {
		return s
	}
	return http.StatusOK
}
