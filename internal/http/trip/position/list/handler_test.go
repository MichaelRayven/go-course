package list_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/trip/position/list"
)

var tripID = uuid.UUID{15: 0xaa}

func setup(t *testing.T) (*list.Handler, *Mockusecase) {
	t.Helper()
	ctrl := gomock.NewController(t)
	uc := NewMockusecase(ctrl)
	return list.New(uc, slog.New(slog.DiscardHandler)), uc
}

func serve(h *list.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", http.NoBody)
	rec := httptest.NewRecorder()
	h.ListTripPositions(rec, req, tripID)
	return rec
}

func TestListTripPositionsSuccess(t *testing.T) {
	h, uc := setup(t)

	positions := []trip.Position{
		{
			ID:          1,
			TripID:      tripID,
			Coordinates: trip.Coordinates{Latitude: 59.935, Longitude: 30.335},
			RecordedAt:  time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:          2,
			TripID:      tripID,
			Coordinates: trip.Coordinates{Latitude: 59.936, Longitude: 30.336},
			RecordedAt:  time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC),
		},
	}

	uc.EXPECT().Execute(gomock.Any(), tripID).Return(positions, nil)

	rec := serve(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var res api.TripPositionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(res.Positions))
	}
	if res.Positions[0].Id != 1 || res.Positions[1].Id != 2 {
		t.Errorf("unexpected positions response: %+v", res.Positions)
	}
}

func TestListTripPositionsEmpty(t *testing.T) {
	h, uc := setup(t)

	uc.EXPECT().Execute(gomock.Any(), tripID).Return([]trip.Position{}, nil)

	rec := serve(h)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var res api.TripPositionsResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(res.Positions))
	}
}

func TestListTripPositionsTripNotFound(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)

	rec := serve(h)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListTripPositionsInternalError(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, errors.New("db error"))

	rec := serve(h)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
