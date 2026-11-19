package health

import (
	"context"
	"net/http"
	"time"

	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/respond"
)

type Handler struct {
	db      pinger
	timeout time.Duration
}

func New(db pinger, timeout time.Duration) *Handler {
	return &Handler{
		db:      db,
		timeout: timeout,
	}
}

// Health reports that the process is alive. It never touches dependencies.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

// Ready reports whether the service can serve traffic, i.e. the database answers.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		respond.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
