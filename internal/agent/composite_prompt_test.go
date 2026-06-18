package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

type compositeRecallStub struct {
	hits []biz.CompositeRecallHit
}

func (s compositeRecallStub) RecallComposite(_ context.Context, _ biz.CompositeRecallQuery) ([]biz.CompositeRecallHit, error) {
	return s.hits, nil
}

func TestCompositeMemoryCue_FormatsFusedBlock(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Session A: fixed bug"},
		{Layer: "L3", Line: "Prefers Go"},
	}}, biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, nil)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "[L2]") || !strings.Contains(cue, "[L3]") {
		t.Fatalf("missing layer tags: %s", cue)
	}
}

// TestCompositeMemoryCue_MergesProactiveHits verifies that proactive recall
// hits are merged with RecallComposite results, deduplicated by line, and
// ranked by score (P3-11).
func TestCompositeMemoryCue_MergesProactiveHits(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Session A: fixed bug", Score: 0.5},
		{Layer: "L3", Line: "Prefers Go", Score: 0.8},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "Prefers Go", Score: 0.9},      // duplicate of recall hit
		{Layer: "L3", Line: "Lives in London", Score: 0.7}, // unique proactive hit
	}
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: recallHits},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, proactiveHits)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "fixed bug") {
		t.Fatalf("missing recall hit: %s", cue)
	}
	if !strings.Contains(cue, "Prefers Go") {
		t.Fatalf("missing deduplicated hit: %s", cue)
	}
	if !strings.Contains(cue, "Lives in London") {
		t.Fatalf("missing proactive hit: %s", cue)
	}
	// Verify deduplication: "Prefers Go" should appear only once
	if strings.Count(cue, "Prefers Go") != 1 {
		t.Fatalf("expected 'Prefers Go' to appear once (deduplicated), got %d: %s", strings.Count(cue, "Prefers Go"), cue)
	}
}

// TestCompositeMemoryCue_ProactiveOnly verifies that proactive hits are
// rendered even when RecallComposite returns no hits.
func TestCompositeMemoryCue_ProactiveOnly(t *testing.T) {
	policy := biz.ResolveMemoryRuntimePolicy(&biz.AgentRuntimeSettings{
		MemoryEnabled: true, L2RecallEnabled: true, L3Enabled: true, L0InjectL3: true,
	})
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "Lives in London", Score: 0.7},
	}
	cue := CompositeMemoryCue(context.Background(), compositeRecallStub{hits: nil},
		biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0, proactiveHits)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "Lives in London") {
		t.Fatalf("missing proactive hit: %s", cue)
	}
}

// TestMergeCompositeHits_Deduplication verifies that mergeCompositeHits
// deduplicates by line (case-insensitive) and respects the limit.
func TestMergeCompositeHits_Deduplication(t *testing.T) {
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "Fixed bug", Score: 0.5},
		{Layer: "L3", Line: "Prefers Go", Score: 0.8},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "prefers go", Score: 0.9}, // case-insensitive duplicate
		{Layer: "L3", Line: "Lives in London", Score: 0.7},
	}
	merged := mergeCompositeHits(recallHits, proactiveHits, 10)
	if len(merged) != 3 {
		t.Fatalf("expected 3 deduplicated hits, got %d: %+v", len(merged), merged)
	}
	// Higher score should come first; duplicate "prefers go" is dropped, keeping "Prefers Go" (0.8)
	if merged[0].Line != "Prefers Go" || merged[0].Score != 0.8 {
		t.Fatalf("expected 'Prefers Go' (0.8) first, got %+v", merged[0])
	}
}

// TestMergeCompositeHits_Limit verifies that mergeCompositeHits respects the
// limit parameter after deduplication and sorting.
func TestMergeCompositeHits_Limit(t *testing.T) {
	recallHits := []biz.CompositeRecallHit{
		{Layer: "L2", Line: "A", Score: 0.3},
		{Layer: "L2", Line: "B", Score: 0.5},
	}
	proactiveHits := []biz.CompositeRecallHit{
		{Layer: "L3", Line: "C", Score: 0.9},
		{Layer: "L3", Line: "D", Score: 0.7},
	}
	merged := mergeCompositeHits(recallHits, proactiveHits, 2)
	if len(merged) != 2 {
		t.Fatalf("expected 2 hits after limit, got %d: %+v", len(merged), merged)
	}
	// Top 2 by score: C (0.9) and D (0.7)
	if merged[0].Line != "C" || merged[1].Line != "D" {
		t.Fatalf("expected [C, D], got [%s, %s]", merged[0].Line, merged[1].Line)
	}
}
