package turntrace

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestBridgeRecordToolCallEndConcurrent(t *testing.T) {
	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "test.turn", SessionID: "s1", RunID: "r1"})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bridge.RecordToolCallEnd("call_1", "search", nil)
		}()
	}
	wg.Wait()
	bridge.Finish(nil)
	bridge.Finish(nil)
}

func TestBridgeFinishUsesExecutionStatus(t *testing.T) {
	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainGraph, SpanName: "graph.execute", SessionID: "s", RunID: "e1"})
	bridge.Finish(context.Canceled)
}

// setupTestTracerProvider installs an in-memory span exporter for the test,
// returning the exporter and a cleanup func that restores the prior TracerProvider.
func setupTestTracerProvider(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exp),
	)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return exp, func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	}
}

func TestBridge_StartPhase_EndPhase(t *testing.T) {
	exp, cleanup := setupTestTracerProvider(t)
	defer cleanup()

	ctx := context.Background()
	ctx, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "test.turn", SessionID: "s1", RunID: "r1"})

	// Start each phase and verify a span is returned.
	planCtx, planSpan := bridge.StartPhase(ctx, PhasePlan)
	if planSpan == nil {
		t.Fatal("StartPhase(PhasePlan) returned nil span")
	}
	if planCtx == nil {
		t.Fatal("StartPhase(PhasePlan) returned nil ctx")
	}

	_, allocSpan := bridge.StartPhase(planCtx, PhaseAlloc)
	if allocSpan == nil {
		t.Fatal("StartPhase(PhaseAlloc) returned nil span")
	}

	_, orchSpan := bridge.StartPhase(ctx, PhaseOrch)
	if orchSpan == nil {
		t.Fatal("StartPhase(PhaseOrch) returned nil span")
	}

	// End phases — should not panic and should end spans.
	bridge.EndPhase(PhaseOrch, nil)
	bridge.EndPhase(PhaseAlloc, nil)
	bridge.EndPhase(PhasePlan, nil)

	bridge.Finish(nil)

	// WithSyncer exports spans synchronously when they end, so any span
	// present in the exporter has already been ended.
	spans := exp.GetSpans()
	names := spanNames(spans)
	if !contains(names, PhasePlan) {
		t.Errorf("expected phase span %q in recorded spans %v", PhasePlan, names)
	}
	if !contains(names, PhaseAlloc) {
		t.Errorf("expected phase span %q in recorded spans %v", PhaseAlloc, names)
	}
	if !contains(names, PhaseOrch) {
		t.Errorf("expected phase span %q in recorded spans %v", PhaseOrch, names)
	}
}

func TestBridge_StartPhase_NilBridge(t *testing.T) {
	var nilBridge *Bridge
	ctx := context.Background()

	// Must not panic on nil Bridge.
	outCtx, span := nilBridge.StartPhase(ctx, PhasePlan)
	if span != nil {
		t.Errorf("nil Bridge StartPhase should return nil span, got %v", span)
	}
	if outCtx == nil {
		t.Error("nil Bridge StartPhase should return non-nil ctx")
	}

	// EndPhase on nil Bridge must not panic.
	nilBridge.EndPhase(PhasePlan, nil)
}

func TestBridge_EndPhase_NotStarted(t *testing.T) {
	exp, cleanup := setupTestTracerProvider(t)
	defer cleanup()

	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "test.turn", SessionID: "s1", RunID: "r1"})

	// EndPhase on a phase that was never started — must not panic.
	bridge.EndPhase(PhasePlan, nil)
	bridge.EndPhase(PhaseAlloc, nil)
	bridge.EndPhase(PhaseOrch, nil)

	bridge.Finish(nil)

	// No phase spans should have been recorded.
	spans := exp.GetSpans()
	for _, s := range spans {
		if contains([]string{PhasePlan, PhaseAlloc, PhaseOrch}, s.Name) {
			t.Errorf("unexpected phase span %q recorded for non-started phase", s.Name)
		}
	}
}

func TestBridge_Finish_ClosesPhaseSpans(t *testing.T) {
	exp, cleanup := setupTestTracerProvider(t)
	defer cleanup()

	ctx := context.Background()
	_, bridge, _ := Start(ctx, Config{Domain: DomainChat, SpanName: "test.turn", SessionID: "s1", RunID: "r1"})

	// Start phases but do NOT end them explicitly.
	bridge.StartPhase(ctx, PhasePlan)
	bridge.StartPhase(ctx, PhaseAlloc)
	bridge.StartPhase(ctx, PhaseOrch)

	// Finish should close all open phase spans without panic.
	bridge.Finish(nil)

	// WithSyncer exports spans synchronously when they end, so any span
	// present in the exporter has already been ended.
	spans := exp.GetSpans()
	endedCount := 0
	for _, s := range spans {
		if contains([]string{PhasePlan, PhaseAlloc, PhaseOrch}, s.Name) {
			endedCount++
		}
	}
	if endedCount != 3 {
		t.Errorf("expected 3 ended phase spans, got %d", endedCount)
	}
}

// spanNames extracts the Name field from recorded spans.
func spanNames(spans []tracetest.SpanStub) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name)
	}
	return out
}

// contains reports whether the slice contains the string.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
