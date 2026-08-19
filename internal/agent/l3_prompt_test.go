package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

type memoryL2RecallerMock struct {
	rows [][]byte
}

func (m *memoryL2RecallerMock) RecallEpisodes(_ context.Context, _ biz.L2RecallQuery) ([][]byte, error) {
	return m.rows, nil
}

type memoryL3RecallerMock struct {
	rows      [][]byte
	fusedRows [][]byte
}

func (m *memoryL3RecallerMock) RecallFacts(_ context.Context, _ biz.L3RecallQuery) ([][]byte, error) {
	return m.rows, nil
}

func (m *memoryL3RecallerMock) RecallFactsFused(_ context.Context, _ biz.L3FusedRecallQuery) ([][]byte, error) {
	if len(m.fusedRows) > 0 {
		return m.fusedRows, nil
	}
	return m.rows, nil
}

func TestL3MemoryCue_DisabledWhenInjectOff(t *testing.T) {
	l3 := &memoryL3RecallerMock{rows: [][]byte{[]byte(`{"statement":"I prefer tea"}`)}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    false,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: false}}
	if got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5, nil, nil); got != "" {
		t.Fatalf("expected empty cue, got %q", got)
	}
}

func TestL3MemoryCue_FormatsFacts(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{[]byte(`{"statement":"I prefer tea"}`)}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
		L3RecallTopK:  5,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5, nil, nil)
	if got == "" || !containsAll(got, "L3 semantic memory", "I prefer tea") {
		t.Fatalf("unexpected cue: %q", got)
	}
}

func TestL2MemoryCue_FormatsEpisodes(t *testing.T) {
	l2 := &memoryL2RecallerMock{rows: [][]byte{[]byte(`{"title":"Auto-memory consolidation","outcome_summary":"learned preference"}`)}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled:   true,
		L2RecallEnabled: true,
		L2RecallMax:     2,
	})
	ag := biz.Agent{ID: "ag1"}
	got := L2MemoryCue(context.Background(), l2, ag, policy, "sess-1", "", 0, nil)
	if got == "" || !containsAll(got, "L2 episodic memory", "Auto-memory consolidation") {
		t.Fatalf("unexpected cue: %q", got)
	}
}

// TestL2MemoryCue_UsesIndependentBudget verifies the L2 recall block packs
// lines against policy.L2RecallBudgetTokens — not the L3 budget (2026-08-20
// token-cost review: L2 previously borrowed L3RecallBudgetTokens).
func TestL2MemoryCue_UsesIndependentBudget(t *testing.T) {
	mk := func(i int) []byte {
		return []byte(fmt.Sprintf(`{"title":"episode-%d","outcome_summary":"%s"}`, i, strings.Repeat("事", 150)))
	}
	l2 := &memoryL2RecallerMock{rows: [][]byte{mk(1), mk(2), mk(3)}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled:   true,
		L2RecallEnabled: true,
		L2RecallMax:     3,
		// L2 预算 200 tokens：header(~47) + 两条 gist(~64+64) 放入，第三条溢出。
		L2RecallBudgetTokens: 200,
		// L3 预算充足——若实现仍复用它，三条会全部注入。
		L3RecallBudgetTokens: 1600,
	})
	ag := biz.Agent{ID: "ag1"}
	got := L2MemoryCue(context.Background(), l2, ag, policy, "sess-1", "", 0, nil)
	if !strings.Contains(got, "episode-1") {
		t.Fatalf("episode-1 should be kept, got %q", got)
	}
	if strings.Contains(got, "episode-3") {
		t.Fatalf("L2 budget 200 should truncate before episode-3, got %q", got)
	}
}

func TestL1MemoryCue_FormatsTaskSummary(t *testing.T) {
	got := formatL1TaskOnlyResult(map[string]any{
		"task_title": "Build API",
		"task_goal":  "Ship v1",
	})
	if got == nil || got.Cue == "" || !containsAll(got.Cue, "L1 working memory", "Build API", "Ship v1") {
		t.Fatalf("unexpected cue: %+v", got)
	}
}

func TestL3MemoryCue_CrossLayerDedup(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{
		[]byte(`{"statement":"User prefers dark mode"}`),
		[]byte(`{"statement":"User likes tea"}`),
		[]byte(`{"statement":"System uses Go"}`),
	}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
		L3RecallTopK:  10,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	rt := biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}

	// L1 field "User prefers dark mode" should filter out the matching L3 fact
	l1Values := []string{"User prefers dark mode"}
	got := L3MemoryCue(context.Background(), l3, ag, policy, rt, "", 10, l1Values, nil)
	if got == "" {
		t.Fatal("expected non-empty cue")
	}
	if strings.Contains(got, "User prefers dark mode") {
		t.Error("L3 cue should not contain deduped fact 'User prefers dark mode'")
	}
	if !strings.Contains(got, "User likes tea") {
		t.Error("L3 cue should contain non-deduped fact 'User likes tea'")
	}
	if !strings.Contains(got, "System uses Go") {
		t.Error("L3 cue should contain non-deduped fact 'System uses Go'")
	}
}

func TestL3MemoryCue_CrossLayerDedup_CaseInsensitive(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{
		[]byte(`{"statement":"USER PREFERS DARK MODE"}`),
	}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L3Enabled: true, L0InjectL3: true, L3RecallTopK: 10,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	rt := biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}

	l1Values := []string{"user prefers dark mode"}
	got := L3MemoryCue(context.Background(), l3, ag, policy, rt, "", 10, l1Values, nil)
	if got != "" {
		t.Errorf("expected empty cue after case-insensitive dedup, got %q", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
