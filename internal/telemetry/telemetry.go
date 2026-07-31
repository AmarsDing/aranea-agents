// Package telemetry initializes OpenTelemetry tracer and meter providers for the admin process.
// Prometheus business metrics live in internal/metrics; this package owns OTLP export only.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"aranea-agents/pkg/loggateway"

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

// InitStatus classifies the outcome of Init for delayed flow-log emission.
// Init runs before the monitor event bus exists (pre-wire), so the startup
// orchestration layer (cmd/admin/app.go) reads LastInitResult and emits the
// system.telemetry.* flow logs once the bus is available.
type InitStatus string

const (
	InitStatusUnset InitStatus = ""
	InitStatusOK    InitStatus = "ok"
	InitStatusNoop  InitStatus = "noop"
	InitStatusError InitStatus = "error"
)

// InitResult captures the Init outcome for delayed reporting. ErrMessage is
// the sanitized error text (no credentials — OTLP endpoints carry no secrets).
type InitResult struct {
	Status     InitStatus
	Endpoint   string
	Protocol   string
	ErrMessage string
}

var (
	lastInitMu     sync.RWMutex
	lastInitResult InitResult
)

// LastInitResult returns the outcome of the most recent Init call.
func LastInitResult() InitResult {
	lastInitMu.RLock()
	defer lastInitMu.RUnlock()
	return lastInitResult
}

func recordInitResult(res InitResult) {
	lastInitMu.Lock()
	defer lastInitMu.Unlock()
	lastInitResult = res
}

// Init configures OTLP tracer and meter providers from environment variables.
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset, providers remain noop and shutdown is a no-op.
func Init(serviceName, serviceVersion string, lg loggateway.Logger) func(context.Context) error {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		recordInitResult(InitResult{Status: InitStatusNoop})
		lg.Debug("OTEL_EXPORTER_OTLP_ENDPOINT 未配置，使用 noop 提供者", loggateway.StepID("telemetry.noop"))
		return func(context.Context) error { return nil }
	}

	protocol := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
	if protocol == "" {
		protocol = otelProtocolHTTP
	}

	ctx := context.Background()
	tpShutdown, err := initTracerProvider(ctx, serviceName, serviceVersion, endpoint, protocol)
	if err != nil {
		recordInitResult(InitResult{Status: InitStatusError, Endpoint: endpoint, Protocol: protocol, ErrMessage: err.Error()})
		lg.Error("OTel Tracer 初始化失败", loggateway.StepID("telemetry.error"), loggateway.Str("protocol", protocol), loggateway.Err(err))
		return func(context.Context) error { return nil }
	}

	if err := initMeterProvider(ctx, serviceName, serviceVersion, endpoint, protocol); err != nil {
		lg.Warn("OTel Meter 初始化失败，指标使用 noop", loggateway.StepID("telemetry.error"), loggateway.Err(err))
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	recordInitResult(InitResult{Status: InitStatusOK, Endpoint: endpoint, Protocol: protocol})
	lg.Info("遥测已初始化",
		loggateway.StepID("telemetry.init"),
		loggateway.Str("endpoint", endpoint),
		loggateway.Str("protocol", protocol),
		loggateway.Str("service", serviceName),
	)
	return func(ctx context.Context) error {
		return shutdownAll(ctx, tpShutdown)
	}
}

func initTracerProvider(ctx context.Context, serviceName, serviceVersion, endpoint, protocol string) (func(context.Context) error, error) {
	switch protocol {
	case otelProtocolGRPC:
		return initGRPCTracerProvider(ctx, endpoint, serviceName, serviceVersion)
	default:
		res, err := newServiceResource(serviceName, serviceVersion)
		if err != nil {
			return nil, err
		}
		return initHTTPTracerProvider(ctx, res, endpoint)
	}
}

func newServiceResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
}

// initHTTPTracerProvider configures OTLP/HTTP trace export with env-driven sampling.
// Note: gRPC export (initGRPCTracerProvider) uses trpc-agent-go defaults (always sample).
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
		sdktrace.WithSampler(buildSampler()),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func initGRPCTracerProvider(ctx context.Context, endpoint, serviceName, serviceVersion string) (func(context.Context) error, error) {
	clean, err := atrace.Start(ctx,
		atrace.WithEndpoint(endpoint),
		atrace.WithProtocol(otelProtocolGRPC),
		atrace.WithServiceName(serviceName),
		atrace.WithServiceVersion(serviceVersion),
	)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error { return clean() }, nil
}

func initMeterProvider(ctx context.Context, serviceName, serviceVersion, endpoint, protocol string) error {
	mp, err := ametric.NewMeterProvider(ctx,
		ametric.WithEndpoint(endpoint),
		ametric.WithProtocol(protocol),
		ametric.WithServiceName(serviceName),
		ametric.WithServiceVersion(serviceVersion),
	)
	if err != nil {
		return err
	}
	return ametric.InitMeterProvider(mp)
}

func shutdownAll(ctx context.Context, tpShutdown func(context.Context) error) error {
	var errs []error
	if tpShutdown != nil {
		if err := tpShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
	}
	if mp := ametric.GetMeterProvider(); mp != nil {
		if sdp, ok := mp.(interface{ Shutdown(context.Context) error }); ok {
			if err := sdp.Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
