package agent

import (
	"context"
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
	if got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5); got != "" {
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
	got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5)
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
	got := L2MemoryCue(context.Background(), l2, ag, policy, "sess-1", "", 0)
	if got == "" || !containsAll(got, "L2 episodic memory", "Auto-memory consolidation") {
		t.Fatalf("unexpected cue: %q", got)
	}
}

func TestL1MemoryCue_FormatsTaskSummary(t *testing.T) {
	got := formatL1TaskOnly(map[string]any{
		"task_title": "Build API",
		"task_goal":  "Ship v1",
	})
	if got == "" || !containsAll(got, "L1 working memory", "Build API", "Ship v1") {
		t.Fatalf("unexpected cue: %q", got)
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
