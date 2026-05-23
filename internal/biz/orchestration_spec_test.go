package biz

import (
	"encoding/json"
	"testing"
)

func TestSpecV2RoundTrip(t *testing.T) {
	raw := `{"version":2,"mode":"parallel","runtime_engine":"graph","custom_field":42,"members":[{"agent_id":"a1","role":"worker","enabled":true,"sort_order":10}],"failure_policy":{"default":"retry_then_block","parallel_fail":"continue"}}`
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	if spec.RuntimeEngine != "graph" {
		t.Fatalf("runtime_engine=%q", spec.RuntimeEngine)
	}
	merged, err := MergeOrchestrationSpecIntoDefinition(raw, spec)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(merged), &body); err != nil {
		t.Fatal(err)
	}
	if body["custom_field"] != float64(42) {
		t.Fatalf("custom_field lost: %v", body["custom_field"])
	}
}

func TestEnsureGraphRuntimeDefault(t *testing.T) {
	out := EnsureGraphRuntimeDefault(`{"version":1,"mode":"sequential","members":[]}`)
	spec, err := ParseOrchestrationSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	if spec.RuntimeEngine != "graph" {
		t.Fatalf("expected graph default, got %q", spec.RuntimeEngine)
	}
}

func TestDefaultOrchestrationSpec(t *testing.T) {
	spec := DefaultOrchestrationSpec()
	if spec.RuntimeEngine != "graph" || spec.Version != OrchestrationSpecVersion {
		t.Fatalf("default spec=%+v", spec)
	}
}
