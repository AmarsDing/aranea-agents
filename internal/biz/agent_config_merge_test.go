package biz

import (
	"context"
	"testing"
)

func TestMergeAgentConfigJSON_patchWins(t *testing.T) {
	m := MergeToolConfigJSON(`{"other_config":{"a":1},"agent_kind":"llm"}`, `{"other_config":{"b":2}}`)
	if oc, ok := m["other_config"].(map[string]any); !ok || oc["b"] != float64(2) {
		t.Fatalf("other_config merge: %#v", m["other_config"])
	}
}

func TestCheckAgentKeyAvailability_format(t *testing.T) {
	uc := NewAgentUsecase(nil, nil)
	_, _, err := uc.CheckAgentKeyAvailability(context.Background(), "Bad Key")
	if err == nil {
		t.Fatal("expected format error")
	}
}
