package team

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestParseDefinition_ModelCascade(t *testing.T) {
	raw := `{"version":2,"mode":"coordinator","model_cascade":{"member_provider":"openai","member_model":"gpt-4o-mini"}}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.ModelCascade == nil {
		t.Fatal("model_cascade must be parsed")
	}
	if def.ModelCascade.MemberProvider != "openai" || def.ModelCascade.MemberModel != "gpt-4o-mini" {
		t.Fatalf("cascade = %+v, want openai/gpt-4o-mini", def.ModelCascade)
	}
}

func TestParseDefinition_ModelCascadeAbsent(t *testing.T) {
	def, err := ParseDefinition(`{"version":2,"mode":"sequential"}`)
	if err != nil {
		t.Fatal(err)
	}
	if def.ModelCascade != nil {
		t.Fatalf("absent model_cascade must stay nil, got %+v", def.ModelCascade)
	}
}

func cascadeTestLookup(agents map[string]biz.Agent) func(context.Context, string) (biz.Agent, error) {
	return func(_ context.Context, id string) (biz.Agent, error) {
		if ag, ok := agents[id]; ok {
			return ag, nil
		}
		return biz.Agent{}, errors.New("not found")
	}
}

func TestCascadeLeaderAgentKeys_SynthesizerAndAnchor(t *testing.T) {
	def := Definition{
		SynthesizerAgentID:  "synth-id",
		IntentAnchorAgentID: "anchor-id",
	}
	lookup := cascadeTestLookup(map[string]biz.Agent{
		"synth-id":  {AgentKey: "synth-key"},
		"anchor-id": {AgentKey: "anchor-key"},
	})
	got := cascadeLeaderAgentKeys(context.Background(), def, lookup, loggateway.NewNoop())
	if len(got) != 2 || got[0] != "synth-key" || got[1] != "anchor-key" {
		t.Fatalf("leader keys = %v, want [synth-key anchor-key]", got)
	}
}

func TestCascadeLeaderAgentKeys_SynthesizerFromRole(t *testing.T) {
	def := Definition{
		Members: []MemberDef{
			{AgentID: "w-id", Role: "worker", SortOrder: 1},
			{AgentID: "s-id", Role: "synthesizer", SortOrder: 2},
		},
	}
	lookup := cascadeTestLookup(map[string]biz.Agent{
		"s-id": {AgentKey: "synth-key"},
	})
	got := cascadeLeaderAgentKeys(context.Background(), def, lookup, loggateway.NewNoop())
	if len(got) != 1 || got[0] != "synth-key" {
		t.Fatalf("leader keys = %v, want [synth-key] (role fallback)", got)
	}
}

func TestCascadeLeaderAgentKeys_DedupAndSkipFailures(t *testing.T) {
	def := Definition{
		SynthesizerAgentID:  "same-id",
		IntentAnchorAgentID: "same-id", // same agent in both roles → dedup
	}
	lookup := cascadeTestLookup(map[string]biz.Agent{
		"same-id": {AgentKey: "shared-key"},
	})
	got := cascadeLeaderAgentKeys(context.Background(), def, lookup, loggateway.NewNoop())
	if len(got) != 1 || got[0] != "shared-key" {
		t.Fatalf("leader keys = %v, want [shared-key] deduped", got)
	}

	// lookup failure → skip with warn, never fail the run
	def2 := Definition{SynthesizerAgentID: "missing-id"}
	got2 := cascadeLeaderAgentKeys(context.Background(), def2, cascadeTestLookup(nil), loggateway.NewNoop())
	if len(got2) != 0 {
		t.Fatalf("leader keys = %v, want empty on lookup failure", got2)
	}
}

func TestCascadeLeaderAgentKeys_None(t *testing.T) {
	got := cascadeLeaderAgentKeys(context.Background(), Definition{}, cascadeTestLookup(nil), loggateway.NewNoop())
	if len(got) != 0 {
		t.Fatalf("leader keys = %v, want empty", got)
	}
}
