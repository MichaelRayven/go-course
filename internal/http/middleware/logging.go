package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Logging writes one record per request once the response is done. It runs
// inside Tracing, so the record carries the request's trace_id. Bodies are
// never logged. 4xx is a client mistake, not a service failure, so only 5xx
// is logged as Error.
func Logging(logger *slog.Logger, route RouteFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			code := status(ww)
			level := slog.LevelInfo
			if code >= http.StatusInternalServerError {
				level = slog.LevelError
			}

			logger.LogAttrs(r.Context(), level, "request handled",
				slog.String("method", r.Method),
				slog.String("route", route(r)),
				slog.Int("status", code),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}
