package finish_test

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
	"github.com/MichaelRayven/go-course/internal/http/trip/finish"
)

var (
	tripID     = uuid.UUID{15: 0xaa}
	finishedAt = time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
)

func setup(t *testing.T) (*finish.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))
	return finish.New(uc, slog.New(slog.DiscardHandler)), uc
}

func serve(h *finish.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/finish", http.NoBody)
	rec := httptest.NewRecorder()
	h.FinishTrip(rec, req, tripID)
	return rec
}

func TestFinishTrip(t *testing.T) {
	h, uc := setup(t)
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(&trip.Trip{
		ID:         tripID,
		UserID:     uuid.UUID{15: 1},
		DriverID:   uuid.UUID{15: 2},
		StartPoint: trip.Coordinates{Latitude: 10, Longitude: 20},
		EndPoint:   trip.Coordinates{Latitude: 30, Longitude: 40},
		Price:      100,
		Status:     trip.StatusCompleted,
		StartedAt:  time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		FinishedAt: &finishedAt,
	}, nil)

	rec := serve(h)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	var got api.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Id != tripID || got.Status != api.Completed ||
		got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) ||
		got.StartPoint != (api.Coordinates{Latitude: 10, Longitude: 20}) ||
		got.EndPoint != (api.Coordinates{Latitude: 30, Longitude: 40}) {
		t.Errorf("body = %+v", got)
	}
}

func TestFinishTripErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"already completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"internal error", errors.New("pq: secret details"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, uc := setup(t)
			uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, tt.err)

			rec := serve(h)

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
