package telemetry

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

	ametric "trpc.group/trpc-go/trpc-agent-go/telemetry/metric"
	atrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
)

const (
	otelProtocolHTTP = "http"
	otelProtocolGRPC = "grpc"
)

func Init(serviceName, serviceVersion string) (shutdown func(context.Context) error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		slog.Debug("OTel: OTEL_EXPORTER_OTLP_ENDPOINT not set; using noop providers")
		return func(context.Context) error { return nil }
	}

	protocol := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
	if protocol == "" {
		protocol = otelProtocolHTTP
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		slog.Error("OTel: failed to create resource", "error", err)
		return func(context.Context) error { return nil }
	}

	var tpShutdown func(context.Context) error
	switch protocol {
	case otelProtocolGRPC:
		tpShutdown, err = initGRPCTracerProvider(context.Background(), res, endpoint)
	default:
		tpShutdown, err = initHTTPTracerProvider(context.Background(), res, endpoint)
	}
	if err != nil {
		slog.Error("OTel: failed to init tracer provider", "protocol", protocol, "error", err)
		return func(context.Context) error { return nil }
	}

	if err := initMeterProvider(context.Background(), serviceName, serviceVersion, endpoint, protocol); err != nil {
		slog.Warn("OTel: meter provider init failed; metrics will use noop", "error", err)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OTel: telemetry initialised", "endpoint", endpoint, "protocol", protocol, "service", serviceName)
	return func(ctx context.Context) error {
		var errs []error
		if err := tpShutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if mp := ametric.GetMeterProvider(); mp != nil {
			if sdp, ok := mp.(interface{ Shutdown(context.Context) error }); ok {
				if err := sdp.Shutdown(ctx); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}
}

func initHTTPTracerProvider(ctx context.Context, res *resource.Resource, endpoint string) (func(context.Context) error, error) {
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func initGRPCTracerProvider(ctx context.Context, res *resource.Resource, endpoint string) (func(context.Context) error, error) {
	clean, err := atrace.Start(ctx,
		atrace.WithEndpoint(endpoint),
		atrace.WithProtocol(otelProtocolGRPC),
		atrace.WithServiceName(strings.TrimPrefix(res.SchemaURL(), "")),
	)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { return clean() }, nil
}

func initMeterProvider(ctx context.Context, serviceName, serviceVersion, endpoint, protocol string) error {
	opts := []ametric.Option{
		ametric.WithEndpoint(endpoint),
		ametric.WithProtocol(protocol),
		ametric.WithServiceName(serviceName),
		ametric.WithServiceVersion(serviceVersion),
	}
	mp, err := ametric.NewMeterProvider(ctx, opts...)
	if err != nil {
		return err
	}
	return ametric.InitMeterProvider(mp)
}
