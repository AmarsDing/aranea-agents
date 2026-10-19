package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// stubAdmissionAgentsUC implements the P3-1 fast-path settings reader on top of
// the narrow TeamAgentLookup stub, counting both entry points so tests can
// prove the resolver never falls back to the expensive hydrated Get.
type stubAdmissionAgentsUC struct {
	stubTeamAgentLookup
	settings      biz.AgentRuntimeSettings
	err           error
	settingsCalls int
	getCalls      int
}

func (s *stubAdmissionAgentsUC) GetRuntimeSettingsForAdmission(context.Context, string) (biz.AgentRuntimeSettings, error) {
	s.settingsCalls++
	return s.settings, s.err
}

func (s *stubAdmissionAgentsUC) Get(context.Context, string) (biz.Agent, error) {
	s.getCalls++
	return biz.Agent{}, apierror.NotFound(apierror.DomainAgent, "not found")
}

// P3-1: threshold comes from the settings-only read (1 query); the hydrated
// Get (5 queries) must NOT be consulted.
func TestResolveContextAdmissionThreshold_SettingsFastPath(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	stub := &stubAdmissionAgentsUC{settings: biz.AgentRuntimeSettings{L0SummaryThreshold: 0.42}}
	orch.core.TD.ReadDeps.AgentsUC = stub

	got := orch.resolveContextAdmissionThresholdForSession(context.Background(), biz.Session{AgentID: "a1"}, biz.TurnEntryPoint(""))
	if got != 0.42 {
		t.Fatalf("threshold=%v want 0.42", got)
	}
	if stub.settingsCalls != 1 {
		t.Fatalf("settings reads=%d want 1", stub.settingsCalls)
	}
	if stub.getCalls != 0 {
		t.Fatalf("hydrated Get must not run on the fast path, got %d calls", stub.getCalls)
	}
}

// Settings read error -> default threshold, no hydrated fallback (a broken
// settings store must not trigger a 5-query hydrate per turn either).
func TestResolveContextAdmissionThreshold_SettingsErrorFallsToDefault(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	stub := &stubAdmissionAgentsUC{err: apierror.Internal(apierror.DomainAgent, "store down")}
	orch.core.TD.ReadDeps.AgentsUC = stub

	got := orch.resolveContextAdmissionThresholdForSession(context.Background(), biz.Session{AgentID: "a1"}, biz.TurnEntryPoint(""))
	if got != biz.DefaultContextAdmissionThreshold {
		t.Fatalf("threshold=%v want default %v", got, biz.DefaultContextAdmissionThreshold)
	}
	if stub.getCalls != 0 {
		t.Fatalf("hydrated Get must not run on settings error, got %d calls", stub.getCalls)
	}
}

// Unset (<=0) L0SummaryThreshold -> caller default, matching the hydrated
// read's raw-value semantics (no withSettingDefaults fill-in on this path).
func TestResolveContextAdmissionThreshold_SettingsUnsetFallsToDefault(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	stub := &stubAdmissionAgentsUC{settings: biz.AgentRuntimeSettings{L0SummaryThreshold: 0}}
	orch.core.TD.ReadDeps.AgentsUC = stub

	got := orch.resolveContextAdmissionThresholdForSession(context.Background(), biz.Session{AgentID: "a1"}, biz.TurnEntryPoint(""))
	if got != biz.DefaultContextAdmissionThreshold {
		t.Fatalf("threshold=%v want default %v", got, biz.DefaultContextAdmissionThreshold)
	}
}

// Narrow stub WITHOUT the fast-path method (legacy tests) -> legacy hydrated
// resolution runs (type-assert fails, falls back to Get).
func TestResolveContextAdmissionThreshold_NarrowStubLegacyFallback(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	orch.core.TD.ReadDeps.AgentsUC = stubTeamAgentLookup{}

	got := orch.resolveContextAdmissionThresholdForSession(context.Background(), biz.Session{AgentID: "a1"}, biz.TurnEntryPoint(""))
	// stubTeamAgentLookup.Get returns NotFound -> default threshold.
	if got != biz.DefaultContextAdmissionThreshold {
		t.Fatalf("threshold=%v want default %v", got, biz.DefaultContextAdmissionThreshold)
	}
}

func TestResolveContextAdmissionThreshold_EmptyAgentID(t *testing.T) {
	orch := newSubmitAwaitReplyTestOrch(nil)
	stub := &stubAdmissionAgentsUC{settings: biz.AgentRuntimeSettings{L0SummaryThreshold: 0.42}}
	orch.core.TD.ReadDeps.AgentsUC = stub

	got := orch.resolveContextAdmissionThresholdForSession(context.Background(), biz.Session{}, biz.TurnEntryPoint(""))
	if got != biz.DefaultContextAdmissionThreshold {
		t.Fatalf("threshold=%v want default %v", got, biz.DefaultContextAdmissionThreshold)
	}
	if stub.settingsCalls != 0 || stub.getCalls != 0 {
		t.Fatalf("empty agent id must not read settings or hydrate (settings=%d get=%d)", stub.settingsCalls, stub.getCalls)
	}
}
