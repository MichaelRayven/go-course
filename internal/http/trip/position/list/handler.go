package list

import (
	"log/slog"
	"net/http"

	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/respond"
)

type Handler struct {
	uc     usecase
	logger *slog.Logger
}

func New(uc usecase, logger *slog.Logger) *Handler {
	return &Handler{
		uc:     uc,
		logger: logger,
	}
}

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	positions, err := h.uc.Execute(r.Context(), tripID)
	if err != nil {
		respond.Error(w, r, h.logger, err)
		return
	}

	respond.JSON(w, http.StatusOK, toAPI(positions))
}
