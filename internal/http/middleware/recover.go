package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/MichaelRayven/go-course/internal/http/respond"
)

var errPanic = errors.New("panic recovered")

// Recover turns a panic in a handler into 500 internal_error, logged once with
// the stack trace. The process keeps serving other requests.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// net/http uses this panic to abort a response on purpose.
				if rec == http.ErrAbortHandler { //nolint:errorlint // compared by identity, as net/http does
					panic(rec)
				}

				logger.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				respond.Problem(w, r, errPanic)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
