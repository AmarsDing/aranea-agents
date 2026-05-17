// Package server – EP-OBS-02: OTel tracer-provider initialisation.
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, an OTLP/HTTP exporter is configured
// and the global tracer provider is installed so that all kratos tracing.Server
// middleware spans are exported to the configured backend (Jaeger, OTEL Collector…).
// When the variable is absent the noop tracer is used and no overhead is incurred.
package server

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracerProvider sets up the global OTel tracer provider.
// It returns a shutdown function that must be deferred by the caller to flush
// pending spans before process exit.
//
// The function is a no-op (returns a nil-safe shutdown) when
// OTEL_EXPORTER_OTLP_ENDPOINT is not set, keeping the zero-config path free of
// any network overhead.
func InitTracerProvider(serviceName, serviceVersion string) (shutdown func(context.Context) error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		slog.Debug("OTel: OTEL_EXPORTER_OTLP_ENDPOINT not set; using noop tracer")
		return func(context.Context) error { return nil }
	}

	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		slog.Error("OTel: failed to create OTLP exporter", "error", err)
		return func(context.Context) error { return nil }
	}

	res, _ := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OTel: tracer provider initialised", "endpoint", endpoint, "service", serviceName)
	return tp.Shutdown
}
