package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/MichaelRayven/go-course/internal/config"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HTTPHandler struct {
	api.Unimplemented
	db     Pinger
	cfg    config.Config
	logger *log.Logger
}

var _ api.ServerInterface = (*HTTPHandler)(nil)

func NewHTTPHandler(db Pinger, cfg config.Config, logger *log.Logger) *HTTPHandler {
	return &HTTPHandler{
		Unimplemented: api.Unimplemented{},
		db:            db,
		logger:        logger,
		cfg:           cfg,
	}
}

func (h *HTTPHandler) Routes() http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	return api.HandlerFromMux(h, router)
}

func (h *HTTPHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *HTTPHandler) Ready(w http.ResponseWriter, r *http.Request) {
	pingCtx, cancelPing := context.WithTimeout(r.Context(), h.cfg.Database.QueryTimeout)
	defer cancelPing()

	if err := h.db.Ping(pingCtx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
