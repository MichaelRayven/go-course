package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MichaelRayven/go-course/internal/http/middleware"
)

func TestLoggingWritesOneRecordPerRequest(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantLevel string
	}{
		{"success is info", http.StatusCreated, "INFO"},
		{"client error is info", http.StatusConflict, "INFO"},
		{"server error is error", http.StatusInternalServerError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			route := tripRouteFunc

			h := middleware.Logging(logger, route)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"secret":"body"}`))
			}))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost,
				"/api/v1/trips/00000000-0000-0000-0000-000000000001", http.NoBody))

			lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
			if len(lines) != 1 {
				t.Fatalf("got %d log records, want 1:\n%s", len(lines), buf.String())
			}
			var rec map[string]any
			if err := json.Unmarshal(lines[0], &rec); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if rec["level"] != tt.wantLevel {
				t.Errorf("level = %v, want %s", rec["level"], tt.wantLevel)
			}
			if rec["msg"] != "request handled" || rec["method"] != "POST" {
				t.Errorf("record = %v", rec)
			}
			if rec["route"] != tripRoute {
				t.Errorf("route = %v, want the template", rec["route"])
			}
			if rec["status"] != float64(tt.status) {
				t.Errorf("status = %v, want %d", rec["status"], tt.status)
			}
			if _, ok := rec["duration_ms"]; !ok {
				t.Error("duration_ms missing")
			}
			if bytes.Contains(buf.Bytes(), []byte("secret")) {
				t.Error("response body was logged")
			}
		})
	}
}
