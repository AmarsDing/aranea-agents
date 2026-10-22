package sandbox

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// countMetric reads a CounterVec child value for assertion.
func countMetric(t *testing.T, cv *prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	return testutil.ToFloat64(cv.WithLabelValues(labels...))
}

// labelSpyEngine wraps fakeEngine and implements the optional LabelUpdater
// capability so Renew's deadline-label sync can be observed.
type labelSpyEngine struct {
	*fakeEngine
	mu        sync.Mutex
	updates   []map[string]string
	updateErr error
}

func (s *labelSpyEngine) UpdateLabels(_ context.Context, h Handle, labels map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	s.updates = append(s.updates, cp)
	return s.updateErr
}

func (s *labelSpyEngine) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updates)
}

func (s *labelSpyEngine) lastUpdate() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.updates) == 0 {
		return nil
	}
	return s.updates[len(s.updates)-1]
}

// Renew must push the new deadline back to the engine labels when the engine
// supports LabelUpdater (ADR-82-4 label 真相源同步).
func TestRenewSyncsDeadlineLabel(t *testing.T) {
	spy := &labelSpyEngine{fakeEngine: newFakeEngine()}
	m := newTestManager(t, spy, func(c *Config) {
		c.TTL.Default = time.Minute
		c.TTL.Max = 2 * time.Minute
	})
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lease.Renew(context.Background(), 10*time.Minute); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if got := spy.updateCount(); got != 1 {
		t.Fatalf("UpdateLabels calls=%d, want 1", got)
	}
	e, _ := m.registry.get(lease.SandboxID())
	want := strconv.FormatInt(e.view.Deadline.Unix(), 10)
	if got := spy.lastUpdate()[LabelDeadline]; got != want {
		t.Fatalf("deadline label=%q, want %q", got, want)
	}
}

// Engine-side label sync failure must NOT break the lease (registry view
// remains authoritative); it is counted + warn-logged only.
func TestRenewLabelSyncFailureNonBlocking(t *testing.T) {
	spy := &labelSpyEngine{
		fakeEngine: newFakeEngine(),
		updateErr:  errors.New("daemon label write exploded"),
	}
	m := newTestManager(t, spy, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lease.Renew(context.Background(), time.Minute); err != nil {
		t.Fatalf("Renew must stay non-blocking on label sync failure, got %v", err)
	}
	// Lease stays live and the registry deadline still moved.
	e, ok := m.registry.get(lease.SandboxID())
	if !ok || e.entryState != StateLeased {
		t.Fatalf("lease lost after label sync failure: %+v ok=%v", e, ok)
	}
}

// Engines WITHOUT the LabelUpdater capability (DockerEngine today — docker
// labels are immutable post-create) must renew silently with zero calls.
func TestRenewWithoutLabelUpdaterCapability(t *testing.T) {
	eng := newFakeEngine() // does not implement LabelUpdater
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lease.Renew(context.Background(), time.Minute); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	// No capability assertion possible beyond "no panic / no error" — assert
	// the renew took effect in the registry.
	e, _ := m.registry.get(lease.SandboxID())
	if e.view.Deadline.IsZero() {
		t.Fatal("registry deadline not updated")
	}
}

// Exec propagates stdout truncation metric; stderr truncation increments the
// per-profile counter.
func TestLeaseExecStderrTruncatedMetric(t *testing.T) {
	eng := newFakeEngine()
	eng.execFunc = func(h Handle, spec ExecSpec) (ExecResult, error) {
		return ExecResult{Stderr: "x\n[stderr truncated at 4 bytes]", StderrTruncated: true, Enforcement: EnforcementFull}, nil
	}
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	res, err := lease.Exec(context.Background(), ExecSpec{Argv: []string{"true"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.StderrTruncated {
		t.Fatal("StderrTruncated flag lost")
	}
	if got := countMetric(t, stderrTruncatedTotal, lease.Profile()); got < 1 {
		t.Fatalf("stderrTruncatedTotal{%s}=%v, want ≥1", lease.Profile(), got)
	}
}
