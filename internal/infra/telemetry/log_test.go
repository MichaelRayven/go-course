package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/MichaelRayven/go-course/internal/infra/telemetry"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	return rec
}

func TestLoggerAddsTraceAndSpanID(t *testing.T) {
	var buf bytes.Buffer
	logger := telemetry.NewLogger(&buf, "info", "trip-service")
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(context.Background(), "op")
	defer span.End()

	logger.With("trip_id", "42").InfoContext(ctx, "trip created")

	rec := decode(t, &buf)
	sc := span.SpanContext()
	if rec["trace_id"] != sc.TraceID().String() {
		t.Errorf("trace_id = %v, want %s", rec["trace_id"], sc.TraceID())
	}
	if rec["span_id"] != sc.SpanID().String() {
		t.Errorf("span_id = %v, want %s", rec["span_id"], sc.SpanID())
	}
	if rec["service"] != "trip-service" || rec["trip_id"] != "42" {
		t.Errorf("record = %v, want service and trip_id kept", rec)
	}
}

func TestLoggerWithoutSpanHasNoTraceID(t *testing.T) {
	var buf bytes.Buffer
	telemetry.NewLogger(&buf, "info", "trip-service").InfoContext(context.Background(), "started")

	if _, ok := decode(t, &buf)["trace_id"]; ok {
		t.Error("trace_id present without a span")
	}
}

func TestLoggerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	telemetry.NewLogger(&buf, "warn", "trip-service").InfoContext(context.Background(), "hidden")

	if buf.Len() != 0 {
		t.Errorf("info record written at warn level: %s", buf.String())
	}
}
