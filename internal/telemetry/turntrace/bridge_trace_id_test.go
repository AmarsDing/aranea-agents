package turntrace

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"

	"aranea-agents/internal/event"
)

// installNoopProvider forces the global TracerProvider to noop (production
// default when OTEL_EXPORTER_OTLP_ENDPOINT is unset) and restores the prior
// provider on cleanup.
func installNoopProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
}

func TestEnsureTraceID_GeneratesAndIsIdempotent(t *testing.T) {
	ctx, id1 := EnsureTraceID(context.Background())
	if id1 == "" {
		t.Fatal("EnsureTraceID must generate an id")
	}
	_, id2 := EnsureTraceID(ctx)
	if id2 != id1 {
		t.Fatalf("EnsureTraceID not idempotent: %q vs %q", id1, id2)
	}
}

func TestStart_NoopProvider_UnifiesTraceID(t *testing.T) {
	installNoopProvider(t)

	ctx, want := EnsureTraceID(context.Background())
	ctx, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "chat.turn", SessionID: "s1", RunID: "r1"})
	defer bridge.Finish(nil)

	if got := bridge.TraceID(); got != want {
		t.Fatalf("Bridge.TraceID() = %q, want unified %q", got, want)
	}
	// Downstream emitters must observe the same trace id from ctx.
	tc := event.NewTraceContext(ctx, event.TraceOpts{SessionID: "s1"})
	if tc.TraceID != want {
		t.Fatalf("NewTraceContext after Start = %q, want %q", tc.TraceID, want)
	}
}

func TestStart_NoopProvider_GeneratesWhenAbsent(t *testing.T) {
	installNoopProvider(t)

	ctx, bridge, _ := Start(context.Background(), Config{Domain: DomainTeam, SpanName: "team.run", SessionID: "s1", RunID: "r1"})
	defer bridge.Finish(nil)

	got := bridge.TraceID()
	if got == "" || got == "00000000000000000000000000000000" {
		t.Fatalf("Bridge.TraceID() must not be empty/zero under noop provider, got %q", got)
	}
	tc := event.NewTraceContext(ctx, event.TraceOpts{})
	if tc.TraceID != got {
		t.Fatalf("ctx trace id %q != bridge trace id %q", tc.TraceID, got)
	}
}

func TestStart_RealProvider_InheritsExplicitTraceID(t *testing.T) {
	_, cleanup := setupTestTracerProvider(t)
	defer cleanup()

	ctx, want := EnsureTraceID(context.Background())
	_, bridge, span := Start(ctx, Config{Domain: DomainChat, SpanName: "chat.turn", SessionID: "s1", RunID: "r1"})
	defer bridge.Finish(nil)

	if got := span.SpanContext().TraceID().String(); got != want {
		t.Fatalf("real provider span trace id = %q, want inherited %q", got, want)
	}
	if got := bridge.TraceID(); got != want {
		t.Fatalf("Bridge.TraceID() = %q, want %q", got, want)
	}
}

func TestEnsureTraceID_AdoptsExistingSpanTraceID(t *testing.T) {
	_, cleanup := setupTestTracerProvider(t)
	defer cleanup()

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "parent")
	defer span.End()
	want := span.SpanContext().TraceID().String()

	_, got := EnsureTraceID(ctx)
	if got != want {
		t.Fatalf("EnsureTraceID must adopt existing span trace id %q, got %q", want, got)
	}
}
