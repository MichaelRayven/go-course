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
	status, body := domainerror.Problem(err)
	body.Instance = &r.URL.Path

	if status >= http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed", "error", err)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
