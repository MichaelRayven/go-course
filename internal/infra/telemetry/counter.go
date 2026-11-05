package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

const meterScope = "github.com/MichaelRayven/go-course"

// Counter adapts an OpenTelemetry counter to the one-method interface the
// usecases declare, so they stay free of telemetry types.
type Counter struct {
	counter metric.Int64Counter
}

func NewCounter(mp metric.MeterProvider, name, description string) (*Counter, error) {
	c, err := mp.Meter(meterScope).Int64Counter(name, metric.WithUnit("{trip}"), metric.WithDescription(description))
	if err != nil {
		return nil, fmt.Errorf("create counter %s: %w", name, err)
	}
	return &Counter{counter: c}, nil
}

func (c *Counter) Inc(ctx context.Context) {
	c.counter.Add(ctx, 1)
}
