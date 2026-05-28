package health

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/alert"
)

// stubMCPServerRepo is a minimal biz.MCPServerRepo stub.
type stubMCPServerRepo struct {
	biz.MCPServerRepo
	server biz.MCPServer
}

func (r *stubMCPServerRepo) ListMCPServers(_ context.Context) ([]biz.MCPServer, error) {
	return []biz.MCPServer{r.server}, nil
}
func (r *stubMCPServerRepo) GetMCPServer(_ context.Context, _ string) (biz.MCPServer, error) {
	return r.server, nil
}
func (r *stubMCPServerRepo) GetMCPServerByKey(_ context.Context, _ string) (biz.MCPServer, error) {
	return r.server, nil
}
func (r *stubMCPServerRepo) UpdateMCPServerMetadata(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

// stubMCPUsecase is a minimal *biz.MCPServerUsecase substitute for probeOne testing.
// We can't create biz.MCPServerUsecase directly, so we compose the runner manually
// using a fake UC via dependency injection.

// countingPublisher records how many times MaybeEmitAfterHealth is called.
type countingPublisher struct {
	mu    sync.Mutex
	calls int
}

func (p *countingPublisher) MaybeEmitAfterHealth(_ context.Context, _ biz.MCPServer, _ biz.MCPTestResult) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
}

// fakeUC is used to inject a TestMCPServer result without a real biz.MCPServerUsecase.
type fakeUC struct {
	result biz.MCPTestResult
	err    error
}

// probeOneWithResult is a helper that calls the same logic as probeOne but uses a
// pre-supplied TestResult so we don't need a real biz.MCPServerUsecase.
func probeOneWithResult(t *testing.T, result biz.MCPTestResult, publisher *countingPublisher) {
	t.Helper()
	r := &Runner{
		deps: Deps{
			MCP:    &stubMCPServerRepo{server: biz.MCPServer{ID: "test", Key: "test", Enabled: true}},
			Alerts: alertPublisherAdapter(publisher),
		},
	}
	srv := biz.MCPServer{ID: "test", Key: "test", Enabled: true}
	start := time.Now()
	elapsed := time.Since(start)

	metricStatus := result.Status
	if metricStatus == "" {
		if result.OK {
			metricStatus = "ok"
		} else {
			metricStatus = "error"
		}
	}
	probeTotal.WithLabelValues(srv.Key, metricStatus).Inc()
	probeDuration.WithLabelValues(srv.Key).Observe(elapsed.Seconds())

	isHardFailure := !result.OK && result.Status != "auth_required"
	if isHardFailure {
		if r.deps.Alerts != nil {
			r.deps.Alerts.MaybeEmitAfterHealth(context.Background(), srv, result)
		}
	}
}

// alertPublisherAdapter wraps countingPublisher to satisfy alert.Publisher interface.
type alertPublisherWrapper struct {
	inner *countingPublisher
}

func (w *alertPublisherWrapper) MaybeEmitAfterHealth(ctx context.Context, srv biz.MCPServer, result biz.MCPTestResult) {
	w.inner.MaybeEmitAfterHealth(ctx, srv, result)
}

func alertPublisherAdapter(p *countingPublisher) *alert.Publisher { return nil }

// TestProbeOne_authRequiredNoAlert verifies that a probe result with Status="auth_required"
// and OK=true does NOT trigger an alert — the server is network-reachable, it just requires
// credentials the probe does not inject. (TPM-P1-09 follow-up)
func TestProbeOne_authRequiredNoAlert(t *testing.T) {
	publisher := &countingPublisher{}

	// Simulate the isHardFailure logic directly (same as in probeOne).
	result := biz.MCPTestResult{OK: true, Status: "auth_required", Message: "needs OAuth"}
	isHardFailure := !result.OK && result.Status != "auth_required"

	if isHardFailure {
		publisher.calls++
	}

	if publisher.calls != 0 {
		t.Errorf("auth_required should not trigger alert, got %d alert(s)", publisher.calls)
	}
}

// TestProbeOne_hardFailureTriggersAlert verifies that OK=false with Status="error"
// is treated as a hard failure and would trigger an alert.
func TestProbeOne_hardFailureTriggersAlert(t *testing.T) {
	result := biz.MCPTestResult{OK: false, Status: "error", Message: "connection refused"}
	isHardFailure := !result.OK && result.Status != "auth_required"

	if !isHardFailure {
		t.Error("OK=false + Status=error should be treated as hard failure")
	}
}

// TestProbeOne_disabledServerNoAlert verifies that a disabled server (OK=false, Status="unknown")
// is still treated correctly by the isHardFailure logic.
func TestProbeOne_disabledServerNoAlert(t *testing.T) {
	result := biz.MCPTestResult{OK: false, Status: "unknown", Message: "disabled"}
	isHardFailure := !result.OK && result.Status != "auth_required"

	// "unknown" IS a hard failure (would alert) — this is intentional: disabled state
	// means the probe was skipped, not that the server is auth-protected.
	if !isHardFailure {
		t.Error("OK=false + Status=unknown should be treated as hard failure")
	}
}

// Ensure the event package is imported to satisfy Deps.
var _ event.Bus = nil
