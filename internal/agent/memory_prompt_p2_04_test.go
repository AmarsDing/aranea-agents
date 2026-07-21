package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// --- P2-04: Unified prompt budget tests ---

func TestJoinCuesWithBudget_NoTruncationWhenUnderLimit(t *testing.T) {
	r := &MemoryCueResult{
		L1Cue:     "## L1 working memory\nTask: test",
		RecallCue: "## L3 semantic memory\n- fact one",
	}
	got := r.JoinCuesWithBudget(1000)
	if strings.Contains(got, "truncated") {
		t.Fatalf("expected no truncation, got: %q", got)
	}
	if !strings.Contains(got, "L1 working memory") || !strings.Contains(got, "L3 semantic memory") {
		t.Fatalf("expected both cues present, got: %q", got)
	}
}

func TestJoinCuesWithBudget_TruncatesWhenOverLimit(t *testing.T) {
	r := &MemoryCueResult{
		L1Cue:     strings.Repeat("a", 200),
		RecallCue: strings.Repeat("b", 200),
	}
	got := r.JoinCuesWithBudget(100)
	if !strings.Contains(got, "truncated by prompt budget") {
		t.Fatalf("expected truncation marker, got length=%d", len(got))
	}
	// Truncated output should be shorter than the original.
	if len(got) >= 400 {
		t.Fatalf("expected truncated output < 400 chars, got %d", len(got))
	}
}

func TestJoinCuesWithBudget_ZeroBudgetMeansUnlimited(t *testing.T) {
	r := &MemoryCueResult{
		L1Cue:     strings.Repeat("x", 5000),
		RecallCue: strings.Repeat("y", 5000),
	}
	got := r.JoinCuesWithBudget(0)
	if strings.Contains(got, "truncated") {
		t.Fatalf("expected no truncation with zero budget, got: %q", got)
	}
	if len(got) < 10000 {
		t.Fatalf("expected full content with zero budget, got %d chars", len(got))
	}
}

func TestJoinCuesWithBudget_MultiByteSafe(t *testing.T) {
	// Use multi-byte (Chinese) characters to verify rune-safe truncation.
	r := &MemoryCueResult{
		L1Cue: strings.Repeat("你", 100),
	}
	got := r.JoinCuesWithBudget(50)
	if !strings.Contains(got, "truncated by prompt budget") {
		t.Fatalf("expected truncation marker, got length=%d", len(got))
	}
}

// --- P2-04: L3 provenance tests ---

func TestL3MemoryCue_ProvenanceIncludedByDefault(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{
		[]byte(`{"id":"fact-abc-12345","statement":"User likes tea","source_session_id":"sess-xyz","confidence":0.85,"version":3}`),
	}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
		L3RecallTopK:  5,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5, nil, nil)

	if got == "" {
		t.Fatal("expected non-empty cue")
	}
	if !strings.Contains(got, "User likes tea") {
		t.Errorf("expected statement in cue, got: %q", got)
	}
	// P2-04: provenance should be included by default.
	if !strings.Contains(got, "id:fact-abc") {
		t.Errorf("expected short fact ID in provenance, got: %q", got)
	}
	if !strings.Contains(got, "src:sess-xyz") {
		t.Errorf("expected source session in provenance, got: %q", got)
	}
	if !strings.Contains(got, "conf:0.85") {
		t.Errorf("expected confidence in provenance, got: %q", got)
	}
	if !strings.Contains(got, "v3") {
		t.Errorf("expected version in provenance, got: %q", got)
	}
}

func TestL3MemoryCue_ProvenanceDisabledWhenPolicyOff(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{
		[]byte(`{"id":"fact-abc","statement":"User likes tea","source_session_id":"sess-xyz","confidence":0.85,"version":3}`),
	}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
		L3RecallTopK:  5,
	})
	// Explicitly disable provenance.
	policy.L3InjectProvenance = false
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5, nil, nil)

	if got == "" {
		t.Fatal("expected non-empty cue")
	}
	if !strings.Contains(got, "User likes tea") {
		t.Errorf("expected statement in cue, got: %q", got)
	}
	// P2-04: provenance should NOT be included when disabled.
	if strings.Contains(got, "id:fact-abc") {
		t.Errorf("did not expect fact ID when provenance disabled, got: %q", got)
	}
	if strings.Contains(got, "conf:") {
		t.Errorf("did not expect confidence when provenance disabled, got: %q", got)
	}
}

func TestL3MemoryCue_ProvenanceSkipsEmptyFactID(t *testing.T) {
	l3 := &memoryL3RecallerMock{fusedRows: [][]byte{
		[]byte(`{"statement":"User likes tea"}`), // no id field
	}}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true,
		L3Enabled:     true,
		L0InjectL3:    true,
		L3RecallTopK:  5,
	})
	ag := biz.Agent{ID: "ag1", Settings: &biz.AgentRuntimeSettings{MemoryEnabled: true, L3Enabled: true, L0InjectL3: true}}
	got := L3MemoryCue(context.Background(), l3, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1", UserID: "u1"}, "", 5, nil, nil)

	if got == "" {
		t.Fatal("expected non-empty cue")
	}
	if !strings.Contains(got, "User likes tea") {
		t.Errorf("expected statement in cue, got: %q", got)
	}
	// No provenance suffix when fact ID is empty.
	if strings.Contains(got, "id:") {
		t.Errorf("did not expect provenance when fact ID is empty, got: %q", got)
	}
}

func TestFormatL3Provenance_AllFields(t *testing.T) {
	got := formatL3Provenance("abcdef1234567890", "sess-xyz123", 0.85, 3)
	if !strings.Contains(got, "id:abcdef12") {
		t.Errorf("expected shortened ID (8 chars), got: %q", got)
	}
	if !strings.Contains(got, "src:sess-xyz") {
		t.Errorf("expected shortened source session, got: %q", got)
	}
	if !strings.Contains(got, "conf:0.85") {
		t.Errorf("expected confidence, got: %q", got)
	}
	if !strings.Contains(got, "v3") {
		t.Errorf("expected version, got: %q", got)
	}
}

func TestFormatL3Provenance_OnlyFactID(t *testing.T) {
	got := formatL3Provenance("abc123", "", 0, 0)
	if !strings.Contains(got, "id:abc123") {
		t.Errorf("expected fact ID, got: %q", got)
	}
	if strings.Contains(got, "src:") {
		t.Errorf("did not expect source session when empty, got: %q", got)
	}
	if strings.Contains(got, "conf:") {
		t.Errorf("did not expect confidence when zero, got: %q", got)
	}
	if strings.Contains(got, "v") {
		t.Errorf("did not expect version when zero, got: %q", got)
	}
}

// --- P2-04: Composite provenance tests ---

func TestCompositeMemoryCue_ProvenanceForL3Hits(t *testing.T) {
	hits := []biz.CompositeRecallHit{
		{
			Layer:         "L3",
			Line:          "User prefers dark mode",
			Score:         0.9,
			FactID:        "fact-abc123",
			SourceSession: "sess-1",
			Confidence:    0.8,
			Version:       2,
		},
		{
			Layer: "L2",
			Line:  "Previous chat summary",
			Score: 0.7,
		},
	}
	stub := compositeRecallStub{hits: hits}
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled:   true,
		L2RecallEnabled: true,
		L3Enabled:       true,
		L0InjectL3:      true,
	})
	ag := biz.Agent{ID: "ag1"}
	got := CompositeMemoryCue(context.Background(), stub, ag, policy, biz.MemoryRuntimeContext{AgentID: "ag1"}, "sess-1", "", 5, nil)
	if got == "" {
		t.Fatal("expected non-empty cue")
	}
	// L3 hit should have provenance.
	if !strings.Contains(got, "id:fact-abc") {
		t.Errorf("expected L3 fact ID provenance, got: %q", got)
	}
	if !strings.Contains(got, "src:sess-1") {
		t.Errorf("expected L3 source session provenance, got: %q", got)
	}
	// L2 hit should NOT have provenance (no FactID).
	lines := strings.Split(got, "\n")
	var l2Line string
	for _, l := range lines {
		if strings.Contains(l, "Previous chat summary") {
			l2Line = l
			break
		}
	}
	if l2Line == "" {
		t.Fatal("expected L2 line in cue")
	}
	if strings.Contains(l2Line, "id:") {
		t.Errorf("L2 hit should not have provenance, got: %q", l2Line)
	}
}

// compositeRecallStub is defined in composite_prompt_test.go — reusing it.
