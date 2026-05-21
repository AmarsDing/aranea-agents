package telemetry

import (
	"context"
	"testing"
)

func TestInitNoopWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown := Init("aranea-test", "test")
	if shutdown == nil {
		t.Fatal("expected shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown: %v", err)
	}
}
