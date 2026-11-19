package domainerror_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/http/domainerror"
)

func priceErr() error {
	return &trip.ValidationError{Field: "Price", Reason: "cannot be negative"}
}

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantType   string
	}{
		{
			name:       "validation error",
			err:        priceErr(),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantType:   "https://tripgo.example/problems/invalid-request",
		},
		{
			name:       "wrapped validation error",
			err:        fmt.Errorf("create trip: %w", priceErr()),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantType:   "https://tripgo.example/problems/invalid-request",
		},
		{
			name:       "trip not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			wantType:   "https://tripgo.example/problems/trip-not-found",
		},
		{
			name:       "wrapped trip not found",
			err:        fmt.Errorf("get trip: %w", trip.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			wantType:   "https://tripgo.example/problems/trip-not-found",
		},
		{
			name:       "trip completed",
			err:        fmt.Errorf("finish trip: %w", trip.ErrTripCompleted),
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			wantType:   "https://tripgo.example/problems/trip-completed",
		},
		{
			name:       "driver busy",
			err:        fmt.Errorf("save trip: %w", trip.ErrDriverBusy),
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			wantType:   "https://tripgo.example/problems/driver-busy",
		},
		{
			name:       "finish before start is an internal error",
			err:        trip.ErrFinishBeforeStart,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantType:   "https://tripgo.example/problems/internal-error",
		},
		{
			name:       "unknown error",
			err:        errors.New("pq: connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantType:   "https://tripgo.example/problems/internal-error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := domainerror.Problem(tt.err)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if int(body.Status) != tt.wantStatus {
				t.Errorf("body.Status = %d, want %d", body.Status, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Errorf("body.Code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Type != tt.wantType {
				t.Errorf("body.Type = %q, want %q", body.Type, tt.wantType)
			}
			if body.Title == "" {
				t.Error("body.Title is empty")
			}
			if body.Detail == nil {
				t.Fatal("body.Detail is nil")
			}
		})
	}
}

// Internal error details must never reach the client.
func TestProblemHidesInternalDetails(t *testing.T) {
	secret := "pq: password authentication failed for user tripgo"

	_, body := domainerror.Problem(fmt.Errorf("fetch trip: %w", errors.New(secret)))

	if body.Detail == nil {
		t.Fatal("body.Detail is nil")
	}
	for _, field := range []string{*body.Detail, body.Title} {
		if strings.Contains(field, secret) || strings.Contains(field, "fetch trip") {
			t.Errorf("internal error leaked into response: %q", field)
		}
	}
}

// Validation errors are written for the client, so their text is shown.
func TestProblemShowsValidationDetail(t *testing.T) {
	_, body := domainerror.Problem(priceErr())

	if body.Detail == nil || *body.Detail != "Price: cannot be negative" {
		t.Errorf("Detail = %v, want %q", body.Detail, "Price: cannot be negative")
	}
}
