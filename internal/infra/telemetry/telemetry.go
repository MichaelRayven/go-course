package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const exportTimeout = 2 * time.Second

type Config struct {
	ServiceName string
}

type Telemetry struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator

	shutdown []func(context.Context) error
}

// New builds the providers. The exporter endpoint, protocol and sampler come
// from the standard OTEL_* variables, which the SDK reads itself.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Telemetry, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("telemetry export failed", "error", err)
	}))

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(version()),
		),
		resource.WithFromEnv(), // OTEL_RESOURCE_ATTRIBUTES: service.namespace, deployment.environment
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithTimeout(exportTimeout))
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
		// No WithSampler: the SDK applies OTEL_TRACES_SAMPLER.
	)

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithTimeout(exportTimeout))
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create metric exporter: %w", err),
			tracerProvider.Shutdown(ctx),
		)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)

	return &Telemetry{
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Propagator:     propagation.TraceContext{},
		shutdown:       []func(context.Context) error{tracerProvider.Shutdown, meterProvider.Shutdown},
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	errs := make([]error, 0, len(t.shutdown))
	for _, fn := range t.shutdown {
		errs = append(errs, fn(ctx))
	}
	return errors.Join(errs...)
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
