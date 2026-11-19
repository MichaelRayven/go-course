package create_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/trip/create"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
)

const (
	contentTypeJSON    = "application/json"
	codeInvalidRequest = "invalid_request"
)

var (
	userID   = uuid.UUID{15: 1}
	driverID = uuid.UUID{15: 2}
	tripID   = uuid.UUID{15: 0xaa}
)

var validBody = fmt.Sprintf(`{
	"user_id":     %q,
	"driver_id":   %q,
	"start_point": {"latitude": 10, "longitude": 20},
	"end_point":   {"latitude": 30, "longitude": 40},
	"price":       100
}`, userID, driverID)

var missingFieldsBody = fmt.Sprintf(`{
    "user_id": %q
}`, userID)

var extraFieldsBody = fmt.Sprintf(`{
	"user_id":     %q,
	"driver_id":   %q,
	"start_point": {"latitude": 10, "longitude": 20},
	"end_point":   {"latitude": 30, "longitude": 40},
	"price":       100,
	"discount":    5
}`, userID, driverID)

func wantInput() createuc.Input {
	return createuc.Input{
		UserID:     userID,
		DriverID:   driverID,
		StartPoint: trip.Coordinates{Latitude: 10, Longitude: 20},
		EndPoint:   trip.Coordinates{Latitude: 30, Longitude: 40},
		Price:      100,
	}
}

func createdTrip() *trip.Trip {
	in := wantInput()
	return &trip.Trip{
		ID:         tripID,
		UserID:     in.UserID,
		DriverID:   in.DriverID,
		StartPoint: in.StartPoint,
		EndPoint:   in.EndPoint,
		Price:      in.Price,
		Status:     trip.StatusActive,
		StartedAt:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func setup(t *testing.T) (*create.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))
	return create.New(uc, slog.New(slog.DiscardHandler)), uc
}

func serve(h *create.Handler, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.CreateTrip(rec, req, api.CreateTripParams{})
	return rec
}

func TestCreateTrip(t *testing.T) {
	h, uc := setup(t)
	created := createdTrip()
	uc.EXPECT().Execute(gomock.Any(), wantInput()).Return(created, nil)

	rec := serve(h, validBody, contentTypeJSON)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/trips/"+created.ID.String() {
		t.Errorf("Location = %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, contentTypeJSON) {
		t.Errorf("Content-Type = %q", ct)
	}

	var got api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Id != created.ID || got.Status != api.Active ||
		got.StartPoint != (api.Coordinates{Latitude: 10, Longitude: 20}) ||
		got.EndPoint != (api.Coordinates{Latitude: 30, Longitude: 40}) {
		t.Errorf("body = %+v", got)
	}
}

func TestCreateTripErrors(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		setup       func(uc *Mockusecase) // nil: the usecase must not be called
		wantStatus  int
		wantCode    string
	}{
		{
			name:        "malformed JSON",
			body:        "{",
			contentType: contentTypeJSON,
			wantStatus:  http.StatusBadRequest,
			wantCode:    codeInvalidRequest,
		},
		{
			name:        "missing required fields",
			body:        missingFieldsBody,
			contentType: contentTypeJSON,
			wantStatus:  http.StatusBadRequest,
			wantCode:    codeInvalidRequest,
		},
		{
			name:        "unknown field",
			body:        extraFieldsBody,
			contentType: contentTypeJSON,
			wantStatus:  http.StatusBadRequest,
			wantCode:    codeInvalidRequest,
		},
		{
			name:        "wrong content type",
			body:        validBody,
			contentType: "text/plain",
			wantStatus:  http.StatusBadRequest,
			wantCode:    codeInvalidRequest,
		},
		{
			name:        "domain validation error",
			body:        validBody,
			contentType: contentTypeJSON,
			setup: func(uc *Mockusecase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(nil, &trip.ValidationError{Field: "Price", Reason: "cannot be negative"})
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:        "driver busy",
			body:        validBody,
			contentType: contentTypeJSON,
			setup: func(uc *Mockusecase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("save trip: %w", trip.ErrDriverBusy))
			},
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name:        "internal error hides details",
			body:        validBody,
			contentType: contentTypeJSON,
			setup: func(uc *Mockusecase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("pq: secret details"))
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, uc := setup(t)
			if tt.setup != nil {
				tt.setup(uc)
			}

			rec := serve(h, tt.body, tt.contentType)

			assertProblem(t, rec, tt.wantStatus, tt.wantCode)
		})
	}
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, wantStatus, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	if strings.Contains(rec.Body.String(), "pq") || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("internal details leaked: %s", rec.Body)
	}

	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Code != wantCode {
		t.Errorf("code = %q, want %q", p.Code, wantCode)
	}
}
