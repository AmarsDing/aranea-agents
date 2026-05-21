package telemetry

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestBuildSamplerDefaults(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")
	s := buildSampler()
	if _, ok := s.(sdktrace.Sampler); !ok {
		t.Fatal("expected sdktrace.Sampler")
	}
}

func TestBuildSamplerParentBasedRatio(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	s := buildSampler()
	if s == nil {
		t.Fatal("expected sampler")
	}
}

func TestParseRatio(t *testing.T) {
	if parseRatio("0.5") != 0.5 {
		t.Fatal("expected 0.5")
	}
	if parseRatio("bad") != 1.0 {
		t.Fatal("expected default 1.0 for bad input")
	}
	if parseRatio("2") != 1.0 {
		t.Fatal("expected clamp at 1.0")
	}
}
