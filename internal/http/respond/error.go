package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/MichaelRayven/go-course/internal/http/domainerror"
)

func Error(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	err error,
) {
	status := Problem(w, r, err)
	if status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", "error", err)
	}
}

// Problem writes err as application/problem+json without logging it and
// returns the status. For callers that have already logged the error.
func Problem(w http.ResponseWriter, r *http.Request, err error) int {
	status, body := domainerror.Problem(err)
	body.Instance = &r.URL.Path

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)

	return status
}
