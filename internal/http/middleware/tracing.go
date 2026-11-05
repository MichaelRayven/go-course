package middleware

import (
	"net/http"
	"strconv"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// RouteFunc returns the matched route template, e.g. /api/v1/trips/{tripId},
// or "" when no route matches.
type RouteFunc func(r *http.Request) string

func Tracing(tp trace.TracerProvider, propagator propagation.TextMapPropagator, route RouteFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		annotate := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.SpanFromContext(r.Context())
			if pattern := route(r); pattern != "" {
				span.SetAttributes(semconv.HTTPRoute(pattern))
			}

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			if status := ww.Status(); status >= http.StatusInternalServerError {
				span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(status)))
				span.SetStatus(codes.Error, http.StatusText(status))
			}
		})

		return otelhttp.NewHandler(annotate, "http.server",
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithPropagators(propagator),
			// HTTP metrics come from the Metrics middleware, with route labels.
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				if pattern := route(r); pattern != "" {
					return r.Method + " " + pattern
				}
				return r.Method
			}),
		)
	}
}
