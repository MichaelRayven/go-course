package health_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/http/health"
)

func TestHealthNeverTouchesDatabase(t *testing.T) {
	db := NewMockpinger(gomock.NewController(t)) // no EXPECT: Ping must not be called
	h := health.New(db, time.Second)

	rec := httptest.NewRecorder()
	h.Health(rec, httptest.NewRequest(http.MethodGet, "/health", http.NoBody))

	assertHealth(t, rec, http.StatusOK, api.Ok)
}

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   api.HealthResponseStatus
	}{
		{"database answers", nil, http.StatusOK, api.Ok},
		{"database unavailable", errors.New("connection refused"), http.StatusServiceUnavailable, api.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := NewMockpinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).Return(tt.pingErr)
			h := health.New(db, time.Second)

			rec := httptest.NewRecorder()
			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", http.NoBody))

			assertHealth(t, rec, tt.wantStatus, tt.wantBody)
		})
	}
}

func assertHealth(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, want api.HealthResponseStatus) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rec.Code, wantStatus)
	}

	var got api.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Status != want {
		t.Errorf("status = %q, want %q", got.Status, want)
	}
}
