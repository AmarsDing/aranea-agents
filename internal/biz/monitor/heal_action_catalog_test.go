package monitor_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// --- mocks for CatalogHealActionHandler ---

type mockProviderProber struct {
	calls atomic.Int32
	errs  []error // per-call errors; nil beyond len
}

func (m *mockProviderProber) RunHealthChecks(_ context.Context) error {
	i := int(m.calls.Add(1)) - 1
	if i < len(m.errs) {
		return m.errs[i]
	}
	return nil
}

type mockMCPRefresher struct {
	calls  atomic.Int32
	probed int
	err    error
}

func (m *mockMCPRefresher) RefreshEnabledHealth(_ context.Context, _ int) (int, error) {
	m.calls.Add(1)
	return m.probed, m.err
}

// --- tests ---

func TestCatalogHealActionHandler_Retry_CallsProber(t *testing.T) {
	prober := &mockProviderProber{}
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop()).BindRetry(prober)

	action := monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 1}}
	if err := h.HandleFixAction(context.Background(), action, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prober.calls.Load() != 1 {
		t.Errorf("prober calls = %d, want 1 (success on first attempt)", prober.calls.Load())
	}
}

func TestCatalogHealActionHandler_Retry_RetriesOnError(t *testing.T) {
	prober := &mockProviderProber{errs: []error{errors.New("boom"), nil}}
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop()).BindRetry(prober)

	action := monitor.FixAction{Type: "retry", MaxAttempts: 2, Params: map[string]any{"backoff_ms": 1}}
	if err := h.HandleFixAction(context.Background(), action, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prober.calls.Load() != 2 {
		t.Errorf("prober calls = %d, want 2 (first failed, second succeeded)", prober.calls.Load())
	}
}

func TestCatalogHealActionHandler_Retry_ExhaustsAttempts(t *testing.T) {
	prober := &mockProviderProber{errs: []error{errors.New("boom"), errors.New("boom"), errors.New("boom")}}
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop()).BindRetry(prober)

	action := monitor.FixAction{Type: "retry", MaxAttempts: 3, Params: map[string]any{"backoff_ms": 1}}
	if err := h.HandleFixAction(context.Background(), action, nil); err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if prober.calls.Load() != 3 {
		t.Errorf("prober calls = %d, want 3", prober.calls.Load())
	}
}

func TestCatalogHealActionHandler_Retry_NoProberBound(t *testing.T) {
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop())

	action := monitor.FixAction{Type: "retry", MaxAttempts: 1}
	if err := h.HandleFixAction(context.Background(), action, nil); err == nil {
		t.Fatal("expected error when no prober is bound")
	}
}

func TestCatalogHealActionHandler_Reconnect_CallsRefresher(t *testing.T) {
	refresher := &mockMCPRefresher{probed: 5}
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop()).BindReconnect(refresher)

	action := monitor.FixAction{Type: "reconnect", MaxAttempts: 3}
	if err := h.HandleFixAction(context.Background(), action, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refresher.calls.Load() != 1 {
		t.Errorf("refresher calls = %d, want 1", refresher.calls.Load())
	}
}

func TestCatalogHealActionHandler_Reconnect_NoRefresherBound(t *testing.T) {
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop())

	action := monitor.FixAction{Type: "reconnect", MaxAttempts: 1}
	if err := h.HandleFixAction(context.Background(), action, nil); err == nil {
		t.Fatal("expected error when no refresher is bound")
	}
}

func TestCatalogHealActionHandler_RecordOnlyActions(t *testing.T) {
	prober := &mockProviderProber{}
	refresher := &mockMCPRefresher{}
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop()).BindRetry(prober).BindReconnect(refresher)

	for _, typ := range []string{"fallback", "log_only"} {
		action := monitor.FixAction{Type: typ, MaxAttempts: 1}
		if err := h.HandleFixAction(context.Background(), action, nil); err != nil {
			t.Fatalf("%s: unexpected error: %v", typ, err)
		}
	}
	if prober.calls.Load() != 0 || refresher.calls.Load() != 0 {
		t.Error("record-only actions must not invoke any executor")
	}
}

func TestCatalogHealActionHandler_UnknownAction_Fails(t *testing.T) {
	h := monitor.NewCatalogHealActionHandler(loggateway.NewNoop())

	action := monitor.FixAction{Type: "explode", MaxAttempts: 1}
	if err := h.HandleFixAction(context.Background(), action, nil); err == nil {
		t.Fatal("expected error for unknown action type")
	}
}

func TestCatalogHealActionHandler_NilSafe(t *testing.T) {
	var h *monitor.CatalogHealActionHandler
	if err := h.HandleFixAction(context.Background(), monitor.FixAction{Type: "retry"}, nil); err == nil {
		t.Fatal("expected error on nil receiver")
	}
}
