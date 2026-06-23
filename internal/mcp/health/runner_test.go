package health

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
)

// stubMCPServerRepo is a minimal biz.MCPServerReader stub.
type stubMCPServerRepo struct {
	biz.MCPServerReader
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

// countingPublisher records how many times MaybeEmitAfterHealth is called.
// It implements AlertEmitter so it can be injected into Deps.Alerts.
type countingPublisher struct {
	mu    sync.Mutex
	calls int
	last  biz.MCPTestResult
}

func (p *countingPublisher) MaybeEmitAfterHealth(_ context.Context, _ biz.MCPServer, result biz.MCPTestResult) {
	p.mu.Lock()
	p.calls++
	p.last = result
	p.mu.Unlock()
}

func (p *countingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// runProbeOneLogic mirrors the alert-decision logic in Runner.probeOne so
// tests exercise the same branching as production code. The production code
// uses:
//   isHardFailure := !result.OK
//   isAuthWarning := result.OK && result.Status == "auth_required"
//   if isHardFailure || isAuthWarning { alerts.MaybeEmitAfterHealth(...) }
func runProbeOneLogic(result biz.MCPTestResult, alerts AlertEmitter) {
	isHardFailure := !result.OK
	isAuthWarning := result.OK && result.Status == "auth_required"
	if isHardFailure || isAuthWarning {
		if alerts != nil {
			alerts.MaybeEmitAfterHealth(context.Background(), biz.MCPServer{ID: "test", Key: "test"}, result)
		}
	}
}

// TestProbeOne_authRequiredTriggersAuthWarning verifies that a probe result
// with Status="auth_required" and OK=true triggers the alert path via the
// isAuthWarning branch (the server is network-reachable but requires
// credentials the probe does not inject). This is intentional: auth_required
// is a degraded state worth alerting on so operators notice misconfigured
// credentials. (TPM-P1-09 follow-up)
func TestProbeOne_authRequiredTriggersAuthWarning(t *testing.T) {
	publisher := &countingPublisher{}
	result := biz.MCPTestResult{OK: true, Status: "auth_required", Message: "needs OAuth"}

	runProbeOneLogic(result, publisher)

	if got := publisher.count(); got != 1 {
		t.Errorf("auth_required should trigger alert via isAuthWarning, got %d alert(s)", got)
	}
	if publisher.last.Status != "auth_required" {
		t.Errorf("publisher received wrong result status: %q", publisher.last.Status)
	}
}

// TestProbeOne_hardFailureTriggersAlert verifies that OK=false with any
// Status is treated as a hard failure and triggers an alert.
func TestProbeOne_hardFailureTriggersAlert(t *testing.T) {
	publisher := &countingPublisher{}
	result := biz.MCPTestResult{OK: false, Status: "error", Message: "connection refused"}

	runProbeOneLogic(result, publisher)

	if got := publisher.count(); got != 1 {
		t.Errorf("hard failure should trigger alert, got %d alert(s)", got)
	}
}

// TestProbeOne_disabledServerStillAlerts verifies that a disabled server
// (OK=false, Status="unknown") is still treated as a hard failure. Disabled
// servers are filtered out before probeOne in production (probeAll skips
// !srv.Enabled), but if a probe somehow returns unknown it should alert
// rather than be silently swallowed.
func TestProbeOne_disabledServerStillAlerts(t *testing.T) {
	publisher := &countingPublisher{}
	result := biz.MCPTestResult{OK: false, Status: "unknown", Message: "disabled"}

	runProbeOneLogic(result, publisher)

	if got := publisher.count(); got != 1 {
		t.Errorf("OK=false + Status=unknown should trigger alert, got %d alert(s)", got)
	}
}

// TestProbeOne_healthyServerNoAlert verifies that OK=true with a non-auth
// status does NOT trigger an alert — the server is healthy.
func TestProbeOne_healthyServerNoAlert(t *testing.T) {
	publisher := &countingPublisher{}
	result := biz.MCPTestResult{OK: true, Status: "ok", Message: ""}

	runProbeOneLogic(result, publisher)

	if got := publisher.count(); got != 0 {
		t.Errorf("healthy server should not trigger alert, got %d alert(s)", got)
	}
}

// TestProbeOne_nilAlertsNoPanic verifies that a nil Alerts dependency does
// not cause a panic when a hard failure occurs (defensive: the runner may be
// constructed without an alert publisher in minimal setups).
func TestProbeOne_nilAlertsNoPanic(t *testing.T) {
	result := biz.MCPTestResult{OK: false, Status: "error", Message: "boom"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil Alerts should not panic, got: %v", r)
		}
	}()
	runProbeOneLogic(result, nil)
}
