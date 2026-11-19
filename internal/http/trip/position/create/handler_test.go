package create_test

import (
	"encoding/json"
	"errors"
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
	"github.com/MichaelRayven/go-course/internal/http/trip/position/create"
)

var (
	tripID = uuid.UUID{15: 0xaa}
	fixed  = time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
)

func setup(t *testing.T) (*create.Handler, *Mockusecase) {
	t.Helper()
	ctrl := gomock.NewController(t)
	uc := NewMockusecase(ctrl)
	return create.New(uc, slog.New(slog.DiscardHandler)), uc
}

func serve(h *create.Handler, body string, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.CreateTripPosition(rec, req, tripID)
	return rec
}

func validJSON() string {
	return `{"coordinates":{"latitude":59.9350,"longitude":30.3350},"recorded_at":"2026-08-24T12:00:05Z"}`
}

func TestCreateTripPositionSuccess(t *testing.T) {
	h, uc := setup(t)

	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(&trip.Position{
		ID:          42,
		TripID:      tripID,
		Coordinates: trip.Coordinates{Latitude: 59.9350, Longitude: 30.3350},
		RecordedAt:  fixed,
		CreatedAt:   fixed,
	}, nil)

	rec := serve(h, validJSON(), "application/json")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var res api.TripPosition
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.Id != 42 || res.TripId != tripID {
		t.Errorf("unexpected body: %+v", res)
	}
	if res.Coordinates.Latitude != 59.9350 || res.Coordinates.Longitude != 30.3350 {
		t.Errorf("unexpected coordinates: %+v", res.Coordinates)
	}
}

func TestCreateTripPositionInvalidContentType(t *testing.T) {
	h, _ := setup(t)
	rec := serve(h, validJSON(), "text/plain")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTripPositionInvalidJSON(t *testing.T) {
	h, _ := setup(t)
	rec := serve(h, `{"coordinates":`, "application/json")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTripPositionExtraJSON(t *testing.T) {
	h, _ := setup(t)
	rec := serve(h, validJSON()+` {}`, "application/json")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTripPositionValidationError(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, &trip.ValidationError{
		Field:  "Coordinates.Latitude",
		Reason: "must be between -90 and 90 degrees",
	})

	rec := serve(h, validJSON(), "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTripPositionTripNotFound(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, trip.ErrTripNotFound)

	rec := serve(h, validJSON(), "application/json")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateTripPositionTripCompleted(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, trip.ErrTripCompleted)

	rec := serve(h, validJSON(), "application/json")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestCreateTripPositionInternalError(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	rec := serve(h, validJSON(), "application/json")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
