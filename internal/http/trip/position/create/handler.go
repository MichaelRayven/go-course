package create

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/respond"
)

const maxRequestBodySize = 1 << 20 // 1MiB

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

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	req, err := decode(w, r)
	if err != nil {
		respond.Error(w, r, h.logger, err)
		return
	}

	pos, err := h.uc.Execute(r.Context(), req.toInput(tripID))
	if err != nil {
		respond.Error(w, r, h.logger, err)
		return
	}

	respond.JSON(w, http.StatusCreated, toAPI(*pos))
}

func decode(w http.ResponseWriter, r *http.Request) (request, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return request{}, &trip.ValidationError{Field: "Content-Type", Reason: "must be application/json"}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req request
	if err := decoder.Decode(&req); err != nil {
		return request{}, &trip.ValidationError{Field: "body", Reason: "must be a valid JSON object"}
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return request{}, &trip.ValidationError{Field: "body", Reason: "must contain exactly one JSON object"}
	}

	return req, nil
}
