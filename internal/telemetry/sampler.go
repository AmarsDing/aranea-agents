package telemetry

import (
	"os"
	"strconv"
	"strings"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// buildSampler reads OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG.
// Defaults to always-on (SDK default) when unset.
func buildSampler() sdktrace.Sampler {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER")))
	argStr := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	ratio := parseRatio(argStr)

	switch name {
	case "", "always_on", "parentbased_always_on":
		if name == "parentbased_always_on" {
			return sdktrace.ParentBased(sdktrace.AlwaysSample())
		}
		return sdktrace.AlwaysSample()
	case "always_off", "parentbased_always_off":
		if name == "parentbased_always_off" {
			return sdktrace.ParentBased(sdktrace.NeverSample())
		}
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	default:
		return sdktrace.AlwaysSample()
	}
}

func parseRatio(raw string) float64 {
	if raw == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 1.0
	}
	if v > 1 {
		return 1.0
	}
	return v
}
