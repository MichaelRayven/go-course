package domainerror

import (
	"errors"
	"net/http"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
)

const typePrefix = "https://tripgo.example/problems/"

func Problem(err error) (status int, body api.Problem) {
	var vErr *trip.ValidationError
	if errors.As(err, &vErr) {
		return http.StatusBadRequest, api.Problem{
			Status: http.StatusBadRequest,
			Title:  "Invalid Request",
			Code:   "invalid_request",
			Detail: new(vErr.Error()),
			Type:   typePrefix + "invalid-request",
		}
	}

	if errors.Is(err, trip.ErrTripNotFound) {
		return http.StatusNotFound, api.Problem{
			Status: http.StatusNotFound,
			Title:  "Trip Not Found",
			Code:   "trip_not_found",
			Detail: new("Trip with the specified ID was not found"),
			Type:   typePrefix + "trip-not-found",
		}
	}

	if errors.Is(err, trip.ErrTripCompleted) {
		return http.StatusConflict, api.Problem{
			Status: http.StatusConflict,
			Title:  "Trip Completed",
			Code:   "trip_completed",
			Detail: new("Trip already completed"),
			Type:   typePrefix + "trip-completed",
		}
	}

	if errors.Is(err, trip.ErrDriverBusy) {
		return http.StatusConflict, api.Problem{
			Status: http.StatusConflict,
			Title:  "Driver Busy",
			Code:   "driver_busy",
			Detail: new("Driver already has an active trip"),
			Type:   typePrefix + "driver-busy",
		}
	}

	// trip.ErrFinishBeforeStarted is explicitly ignored
	// since it is a server-side error, it should return code 500

	return http.StatusInternalServerError, api.Problem{
		Status: http.StatusInternalServerError,
		Title:  "Internal Server Error",
		Code:   "internal_error",
		Detail: new("Internal server error"),
		Type:   typePrefix + "internal-error",
	}
}
