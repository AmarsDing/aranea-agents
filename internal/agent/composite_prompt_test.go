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
	}}, biz.Agent{ID: "a1"}, policy, biz.MemoryRuntimeContext{}, "sess", "bug", 0)
	if !strings.Contains(cue, "L2+L3 memory") {
		t.Fatalf("missing header: %s", cue)
	}
	if !strings.Contains(cue, "[L2]") || !strings.Contains(cue, "[L3]") {
		t.Fatalf("missing layer tags: %s", cue)
	}
}
