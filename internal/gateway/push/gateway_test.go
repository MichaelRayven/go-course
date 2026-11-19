package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/MichaelRayven/go-course/internal/gateway/push"
)

var (
	testRecipientID = uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
	testTripID      = uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
)

type expectedPushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

func TestSendPositionRequestSuccess(t *testing.T) {
	t.Parallel()

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	propagator := propagation.TraceContext{}

	var receivedBody expectedPushRequest
	var receivedTraceparent string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got method %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/push" {
			t.Errorf("got path %s, want /api/v1/push", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("got Content-Type %s, want application/json", ct)
		}

		receivedTraceparent = r.Header.Get("traceparent")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedBody); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"msg-123","accepted_at":"2026-08-24T12:00:00Z"}`))
	}))
	defer ts.Close()

	gw, err := push.New(ts.Client(), ts.URL, tp, mp, propagator)
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}
	defer func() { _ = gw.Close() }()

	ctx := context.Background()
	if err := gw.SendPositionRequest(ctx, testRecipientID, testTripID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody.RequestID == "" {
		t.Error("expected non-empty request_id")
	}
	if receivedBody.RecipientID != testRecipientID {
		t.Errorf("got recipient_id %v, want %v", receivedBody.RecipientID, testRecipientID)
	}
	if receivedBody.Kind != "REQUEST_POSITION" {
		t.Errorf("got kind %s, want REQUEST_POSITION", receivedBody.Kind)
	}
	if receivedBody.Data["trip_id"] != testTripID.String() {
		t.Errorf("got data.trip_id %s, want %s", receivedBody.Data["trip_id"], testTripID.String())
	}
	if receivedTraceparent == "" {
		t.Error("expected traceparent header to be injected")
	}

	// Verify span recorded
	spans := spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}
	if spans[0].Name() != "push.send_request" {
		t.Errorf("got span name %s, want push.send_request", spans[0].Name())
	}

	// Verify metrics recorded
	var rm metricdata.ResourceMetrics
	if err := metricReader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	foundSuccess := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "tripgo.position_requests" {
				if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
					for _, dp := range sum.DataPoints {
						for _, attr := range dp.Attributes.ToSlice() {
							if attr.Key == "result" && attr.Value.AsString() == "success" {
								foundSuccess = true
							}
						}
					}
				}
			}
		}
	}
	if !foundSuccess {
		t.Error("metric tripgo.position_requests{result=success} not recorded")
	}
}

func TestSendPositionRequestHTTPErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			body:       `{"code":"invalid_request","message":"bad field"}`,
		},
		{
			name:       "429 Too Many Requests",
			statusCode: http.StatusTooManyRequests,
			body:       `{"code":"rate_limit_exceeded"}`,
		},
		{
			name:       "503 Service Unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"code":"service_unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			gw, err := push.New(ts.Client(), ts.URL, nil, nil, nil)
			if err != nil {
				t.Fatalf("push.New: %v", err)
			}
			defer func() { _ = gw.Close() }()

			err = gw.SendPositionRequest(context.Background(), testRecipientID, testTripID)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var httpErr *push.HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected *push.HTTPError, got %T: %v", err, err)
			}
			if httpErr.StatusCode != tt.statusCode {
				t.Errorf("got StatusCode %d, want %d", httpErr.StatusCode, tt.statusCode)
			}
			if !strings.Contains(httpErr.Body, tt.body) {
				t.Errorf("got Body %q, want to contain %q", httpErr.Body, tt.body)
			}
		})
	}
}

func TestSendPositionRequestTransportError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	// Close immediately to simulate network/transport failure
	ts.Close()

	gw, err := push.New(ts.Client(), ts.URL, nil, nil, nil)
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}

	err = gw.SendPositionRequest(context.Background(), testRecipientID, testTripID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var httpErr *push.HTTPError
	if errors.As(err, &httpErr) {
		t.Errorf("transport error should not be *push.HTTPError, got: %v", httpErr)
	}
}

func TestSendPositionRequestContextCancelled(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	gw, err := push.New(ts.Client(), ts.URL, nil, nil, nil)
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}
	defer func() { _ = gw.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = gw.SendPositionRequest(ctx, testRecipientID, testTripID)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestSendPositionRequestTimeout(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	client := ts.Client()
	client.Timeout = 20 * time.Millisecond

	gw, err := push.New(client, ts.URL, nil, nil, nil)
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}
	defer func() { _ = gw.Close() }()

	err = gw.SendPositionRequest(context.Background(), testRecipientID, testTripID)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
