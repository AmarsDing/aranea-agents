package biz

import "testing"

func TestResolveMemoryRuntimePolicy_MasterOff(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled: false,
		L3Enabled:     true,
		L0InjectL3:    true,
	})
	if p.MasterEnabled || p.InjectL3 || p.WriteL3Facts {
		t.Fatalf("expected all gates off when master disabled: %+v", p)
	}
}

func TestResolveMemoryRuntimePolicy_NilSettingsFailClosed(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(nil)
	if p.MasterEnabled || p.AnyInject() || p.AnyWrite() {
		t.Fatalf("nil settings should fail closed: %+v", p)
	}
}

func TestResolveMemoryRuntimePolicy_ReadWriteSymmetric(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled:    true,
		L3Enabled:        true,
		L0InjectL3:       true,
		L2EpisodeEnabled: true,
		L2RecallEnabled:  true,
		L4Enabled:        true,
		L0InjectL4:       false,
	})
	if !p.InjectL3 || !p.WriteL3Facts {
		t.Fatalf("L3 read/write should both be on: %+v", p)
	}
	if p.InjectL4 || !p.WriteL4Graph {
		t.Fatalf("L4 inject off but write on: %+v", p)
	}
	if !p.WriteL2Episode || !p.RecallL2 {
		t.Fatalf("L2 episode/write mismatch: %+v", p)
	}
}

func TestL3ScopeTargets_ResolvesTeamAndWorkspace(t *testing.T) {
	targets := L3ScopeTargets(MemoryRuntimeContext{
		AgentID: "ag1", UserID: "u1", TeamID: "team-9", Workspace: "ws-main",
	}, []string{"agent", "user", "team", "workspace"})
	if len(targets) < 4 {
		t.Fatalf("expected 4 scopes, got %v", targets)
	}
}

func TestEffectiveL3MinScore_PassiveSkipsFilter(t *testing.T) {
	p := MemoryRuntimePolicy{L3MinScoreQuery: 0.55, L3MinScorePassive: 0}
	if got := EffectiveL3MinScore(p, ""); got != 0 {
		t.Fatalf("passive min score should be 0, got %v", got)
	}
	if got := EffectiveL3MinScore(p, "hello"); got != 0.55 {
		t.Fatalf("query min score should apply, got %v", got)
	}
}
