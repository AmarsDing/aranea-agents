package deliverable

import (
	"aranea-agents/internal/biz"
	"encoding/json"
	"testing"
)

// RuntimeState values may arrive as raw JSON bytes (session-seeded state).
func TestGetDeliverableTool_Call_RuntimeStateRawBytes(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"k": "v-bytes"})
	ctx := ctxWithRuntimeState(map[string]any{biz.DeliverableStateKey: raw})
	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"key":"k"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found || o.Data["k"] != "v-bytes" {
		t.Fatalf("raw-bytes RuntimeState must decode, got %#v", o)
	}
}

// Read-your-writes: a set followed by a get within the same node invocation
// must observe the written value, even before the graph channel merge.
func TestDeliverableTool_ReadYourWrites_SameNode(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{})
	set := NewSetDeliverableTool()
	if _, err := set.Call(ctx, []byte(`{"data":{"research_note":"ALPHA-12345"}}`)); err != nil {
		t.Fatal(err)
	}
	get := NewGetDeliverableTool()
	out, err := get.Call(ctx, []byte(`{"key":"research_note"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found || o.Data["research_note"] != "ALPHA-12345" {
		t.Fatalf("same-node get must observe prior set, got %#v", o)
	}
}

// Two topic writes in the same node must coexist (second merge reads the
// first write via RuntimeState).
func TestDeliverableTool_ReadYourWrites_TopicMergeSameNode(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{})
	set := NewSetDeliverableTool()
	if _, err := set.Call(ctx, []byte(`{"data":{"a":1},"topic":"research"}`)); err != nil {
		t.Fatal(err)
	}
	out, err := set.Call(ctx, []byte(`{"data":{"b":2},"topic":"draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	if _, ok := o.Data["research"].(map[string]any); !ok {
		t.Fatalf("first topic lost in same-node merge: %#v", o.Data)
	}
	if _, ok := o.Data["draft"].(map[string]any); !ok {
		t.Fatalf("second topic missing: %#v", o.Data)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
