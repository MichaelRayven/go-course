package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/MichaelRayven/go-course/internal/http/middleware"
)

func TestMetricsLabelsUseRouteTemplate(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	route := tripRouteFunc

	mw, err := middleware.Metrics(mp, route)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet,
		"/api/v1/trips/00000000-0000-0000-0000-000000000001", http.NoBody))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	hist := findHistogram(t, rm, "http.server.request.duration")
	if len(hist.DataPoints) != 1 {
		t.Fatalf("data points = %d, want 1", len(hist.DataPoints))
	}
	attrs := hist.DataPoints[0].Attributes
	assertAttr(t, attrs, "http.route", tripRoute)
	assertAttr(t, attrs, "http.request.method", "GET")
	if v, ok := attrs.Value("http.response.status_code"); !ok || v.AsInt64() != http.StatusNotFound {
		t.Errorf("http.response.status_code = %v, want 404", v)
	}
}

func findHistogram(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				h, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("%s is %T, want histogram", name, m.Data)
				}
				return h
			}
		}
	}
	t.Fatalf("metric %s not recorded", name)
	return metricdata.Histogram[float64]{}
}

func assertAttr(t *testing.T, set attribute.Set, key, want string) {
	t.Helper()
	if v, ok := set.Value(attribute.Key(key)); !ok || v.AsString() != want {
		t.Errorf("%s = %q, want %q", key, v.AsString(), want)
	}
}
