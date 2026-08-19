package biz

import "testing"

// TestResolveMemoryRuntimePolicy_L2RecallBudgetTokensIndependent verifies the
// L2 episodic recall block has its own token budget knob, independent from
// L3RecallBudgetTokens (2026-08-20 token-cost review: L2 previously borrowed
// the L3 budget field, making the two blocks impossible to tune separately).
func TestResolveMemoryRuntimePolicy_L2RecallBudgetTokensIndependent(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled:        true,
		L2RecallEnabled:      true,
		L2RecallBudgetTokens: 400,
	})
	if p.L2RecallBudgetTokens != 400 {
		t.Fatalf("L2RecallBudgetTokens should pass through, got %d", p.L2RecallBudgetTokens)
	}
	// Unset → standard tier default.
	p2 := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled:   true,
		L2RecallEnabled: true,
	})
	if p2.L2RecallBudgetTokens != MemoryRecallBudgetStandard {
		t.Fatalf("L2RecallBudgetTokens default should be %d, got %d", MemoryRecallBudgetStandard, p2.L2RecallBudgetTokens)
	}
}

// TestResolveMemoryRuntimePolicy_L3InjectProvenanceConfigurable verifies
// provenance injection is driven by settings instead of being hardcoded on
// (2026-08-20 token-cost review: the old code set it true unconditionally,
// despite the comment claiming opt-out was possible).
func TestResolveMemoryRuntimePolicy_L3InjectProvenanceConfigurable(t *testing.T) {
	base := func(provenance bool) *AgentRuntimeSettings {
		return &AgentRuntimeSettings{
			MemoryEnabled:      true,
			L3Enabled:          true,
			L0InjectL3:         true,
			L3InjectProvenance: provenance,
		}
	}
	if !ResolveMemoryRuntimePolicy(base(true)).L3InjectProvenance {
		t.Fatal("L3InjectProvenance=true should pass through")
	}
	if ResolveMemoryRuntimePolicy(base(false)).L3InjectProvenance {
		t.Fatal("L3InjectProvenance=false should disable provenance injection")
	}
}

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

// TestResolveMemoryRuntimePolicy_DefaultRecallScopesIncludeTeam verifies the C5
// default: when L3RecallScopesJSON is empty, the fallback scopes include "team"
// so team-scope L3 facts are recallable by default. Without a TeamID in the
// runtime context, L3ScopeTargets simply skips the team scope (no-op).
func TestResolveMemoryRuntimePolicy_DefaultRecallScopesIncludeTeam(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
	})
	want := []string{"agent", "team"}
	if len(p.L3RecallScopes) != len(want) {
		t.Fatalf("default L3RecallScopes=%v want %v", p.L3RecallScopes, want)
	}
	for i, s := range want {
		if p.L3RecallScopes[i] != s {
			t.Fatalf("default L3RecallScopes=%v want %v", p.L3RecallScopes, want)
		}
	}
}

func TestL3ScopeTargets_TeamScopeSkippedWithoutTeamID(t *testing.T) {
	targets := L3ScopeTargets(MemoryRuntimeContext{AgentID: "ag1"}, []string{"agent", "team"})
	for _, tg := range targets {
		if tg.ScopeType == "team" {
			t.Fatalf("team scope should be skipped when TeamID is empty, got %v", targets)
		}
	}
	if len(targets) != 1 || targets[0].ScopeType != "agent" {
		t.Fatalf("expected only agent scope, got %v", targets)
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

// TestResolveMemoryRuntimePolicy_LinkEvolutionAndEpisodeConsolidation verifies
// that Phase 6A-03 T9 and 6A-06 T8 policy fields are set correctly.
func TestResolveMemoryRuntimePolicy_LinkEvolutionAndEpisodeConsolidation(t *testing.T) {
	// Memory enabled, L3 enabled → link evolution should be on.
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
	})
	if !p.LinkEvolutionEnabled {
		t.Errorf("expected LinkEvolutionEnabled=true when L3 enabled")
	}
	if !p.EpisodeConsolidationEnabled {
		t.Errorf("expected EpisodeConsolidationEnabled=true when memory enabled")
	}
	if p.EpisodeMinImportance != 0.3 {
		t.Errorf("expected EpisodeMinImportance=0.3, got %v", p.EpisodeMinImportance)
	}

	// Memory enabled, L3 disabled → link evolution should be off.
	p2 := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     false,
	})
	if p2.LinkEvolutionEnabled {
		t.Errorf("expected LinkEvolutionEnabled=false when L3 disabled")
	}
	// Episode consolidation should still be on (it's gated by MasterEnabled, not L3).
	if !p2.EpisodeConsolidationEnabled {
		t.Errorf("expected EpisodeConsolidationEnabled=true even when L3 disabled")
	}

	// Memory disabled → all off.
	p3 := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{
		MemoryEnabled: false,
		L3Enabled:     true, // ignored when master off
	})
	if p3.LinkEvolutionEnabled {
		t.Errorf("expected LinkEvolutionEnabled=false when memory disabled")
	}
	if p3.EpisodeConsolidationEnabled {
		t.Errorf("expected EpisodeConsolidationEnabled=false when memory disabled")
	}
}
