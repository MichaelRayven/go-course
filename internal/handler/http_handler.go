package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/repository"
	"github.com/MichaelRayven/go-course/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const maxRequestBodySize = 1 << 20 // 1MiB

type createTripRequest struct {
	UserID     *uuid.UUID          `json:"user_id"`
	DriverID   *uuid.UUID          `json:"driver_id"`
	StartPoint *coordinatesRequest `json:"start_point"`
	EndPoint   *coordinatesRequest `json:"end_point"`
	Price      *int64              `json:"price"`
}

type coordinatesRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type HTTPHandler struct {
	api.Unimplemented
	db           Pinger
	tripService  *service.TripService
	queryTimeout time.Duration
	logger       *slog.Logger
}

var _ api.ServerInterface = (*HTTPHandler)(nil)

func NewHTTPHandler(db Pinger, tripService *service.TripService, queryTimeout time.Duration, logger *slog.Logger) *HTTPHandler {
	return &HTTPHandler{
		Unimplemented: api.Unimplemented{},
		db:            db,
		tripService:   tripService,
		queryTimeout:  queryTimeout,
		logger:        logger,
	}
}

func (h *HTTPHandler) Routes() http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(h.logRequests)
	router.Use(middleware.Recoverer)

	return api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			writeInvalidRequest(w, r, "Request parameters are invalid")
		},
	})
}

func (h *HTTPHandler) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		responseWriter := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(responseWriter, r)

		h.logger.InfoContext(r.Context(), "HTTP request",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", responseWriter.Status(),
			"bytes", responseWriter.BytesWritten(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

func (h *HTTPHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *HTTPHandler) Ready(w http.ResponseWriter, r *http.Request) {
	pingCtx, cancelPing := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancelPing()

	if err := h.db.Ping(pingCtx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *HTTPHandler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.GetByID(r.Context(), tripId)
	if errors.Is(err, repository.ErrTripNotFound) {
		writeProblem(w, r, http.StatusNotFound,
			"https://tripgo.example/problems/trip-not-found",
			"trip_not_found",
			"Trip not found",
			"Trip with the specified ID was not found",
		)
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "get trip",
			"trip_id", tripId,
			"error", err,
		)

		writeProblem(w, r, http.StatusInternalServerError,
			"https://tripgo.example/problems/internal-error",
			"internal_error",
			"Internal server error",
			"An unexpected error occurred",
		)
		return
	}

	writeJSON(w, http.StatusOK, trip)
}

func (h *HTTPHandler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.tripService.Finish(r.Context(), tripId)
	if errors.Is(err, repository.ErrTripNotFound) {
		writeProblem(w, r, http.StatusNotFound,
			"https://tripgo.example/problems/trip-not-found",
			"trip_not_found", "Trip not found", "Trip with the specified ID was not found")
		return
	} else if errors.Is(err, repository.ErrTripCompleted) {
		writeProblem(w, r, http.StatusConflict,
			"https://tripgo.example/problems/trip-completed",
			"trip_completed", "Trip completed", "Trip with the specified ID is already completed")
		return
	} else if err != nil {
		h.logger.ErrorContext(r.Context(), "finish trip",
			"trip_id", tripId,
			"error", err,
		)

		writeProblem(w, r, http.StatusInternalServerError,
			"https://tripgo.example/problems/internal-error",
			"internal_error",
			"Internal server error",
			"An unexpected error occurred",
		)
		return
	}

	h.logger.InfoContext(r.Context(), "trip finished", "trip_id", trip.Id)
	writeJSON(w, http.StatusOK, trip)
}

func (h *HTTPHandler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	var request createTripRequest

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeInvalidRequest(w, r, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeInvalidRequest(w, r, "Request body must be a valid JSON object")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeInvalidRequest(w, r, "Request body must contain exactly one JSON object")
		return
	}

	if request.UserID == nil || request.DriverID == nil || request.StartPoint == nil || request.EndPoint == nil || request.Price == nil {
		writeInvalidRequest(w, r, "All required fields must be provided")
		return
	}
	if *request.UserID == uuid.Nil || *request.DriverID == uuid.Nil {
		writeInvalidRequest(w, r, "User ID and driver ID must be non-empty UUIDs")
		return
	}
	if request.StartPoint.Latitude == nil || request.StartPoint.Longitude == nil || request.EndPoint.Latitude == nil || request.EndPoint.Longitude == nil {
		writeInvalidRequest(w, r, "Latitude and longitude must be provided for both points")
		return
	}

	if *request.EndPoint.Latitude < -90 || *request.EndPoint.Latitude > 90 || *request.StartPoint.Latitude < -90 || *request.StartPoint.Latitude > 90 {
		writeInvalidRequest(w, r, "Latitude must be between -90 and 90 degrees")
		return
	}
	if *request.EndPoint.Longitude < -180 || *request.EndPoint.Longitude > 180 || *request.StartPoint.Longitude < -180 || *request.StartPoint.Longitude > 180 {
		writeInvalidRequest(w, r, "Longitude must be between -180 and 180 degrees")
		return
	}
	if *request.Price < 0 {
		writeInvalidRequest(w, r, "Price must not be negative")
		return
	}

	trip, err := h.tripService.Create(r.Context(), api.TripData{
		UserId:   *request.UserID,
		DriverId: *request.DriverID,
		StartPoint: api.Coordinates{
			Latitude:  *request.StartPoint.Latitude,
			Longitude: *request.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  *request.EndPoint.Latitude,
			Longitude: *request.EndPoint.Longitude,
		},
		Price: *request.Price,
	})
	if errors.Is(err, repository.ErrDriverBusy) {
		writeProblem(w, r, http.StatusConflict,
			"https://tripgo.example/problems/driver-busy",
			"driver_busy",
			"Driver busy",
			"Driver already has an active trip",
		)
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "create trip", "error", err)
		writeProblem(w, r, http.StatusInternalServerError,
			"https://tripgo.example/problems/internal-error",
			"internal_error",
			"Internal Server Error",
			"Internal server error",
		)
		return
	}

	h.logger.InfoContext(r.Context(), "trip created", "trip_id", trip.Id)
	w.Header().Set("Location", "/api/v1/trips/"+trip.Id.String())
	writeJSON(w, http.StatusCreated, trip)
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, r, http.StatusBadRequest,
		"https://tripgo.example/problems/invalid-request",
		"invalid_request",
		"Invalid request",
		detail,
	)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	problemType string,
	code string,
	title string,
	detail string,
) {
	instance := r.URL.Path

	problem := api.Problem{
		Type:     problemType,
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &instance,
		Code:     code,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
