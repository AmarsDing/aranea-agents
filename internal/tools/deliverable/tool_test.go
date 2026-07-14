package deliverable

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

func TestSetDeliverableTool_Declaration(t *testing.T) {
	tl := NewSetDeliverableTool()
	decl := tl.Declaration()
	if decl.Name != "set_deliverable" {
		t.Fatalf("name=%q want set_deliverable", decl.Name)
	}
	if decl.InputSchema == nil || decl.InputSchema.Type != "object" {
		t.Fatalf("input schema missing or not object: %#v", decl.InputSchema)
	}
	if decl.OutputSchema == nil {
		t.Fatal("output schema should be non-nil")
	}
}

func TestSetDeliverableTool_StateDelta(t *testing.T) {
	tl := NewSetDeliverableTool()
	// Simulate a Call result JSON
	resultJSON, err := json.Marshal(setDeliverableOutput{
		Written: true,
		Data: map[string]any{
			"summary": "task completed",
			"file":    "output.txt",
		},
		Keys: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	delta := tl.StateDelta("call-1", nil, resultJSON)
	if delta == nil {
		t.Fatal("expected non-nil delta")
	}
	b, ok := delta[biz.DeliverableStateKey]
	if !ok {
		t.Fatalf("expected delta to contain %q, got keys: %v", biz.DeliverableStateKey, deltaKeys(delta))
	}
	var stored map[string]any
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatalf("delta value not valid JSON map: %v", err)
	}
	if stored["summary"] != "task completed" || stored["file"] != "output.txt" {
		t.Fatalf("unexpected stored data: %#v", stored)
	}
}

func TestSetDeliverableTool_StateDelta_EmptyResult(t *testing.T) {
	tl := NewSetDeliverableTool()
	// Empty toolCallID → nil delta
	if delta := tl.StateDelta("", nil, []byte(`{}`)); delta != nil {
		t.Fatalf("expected nil delta for empty toolCallID, got: %#v", delta)
	}
	// Invalid JSON → nil delta
	if delta := tl.StateDelta("call-1", nil, []byte("not-json")); delta != nil {
		t.Fatalf("expected nil delta for invalid JSON, got: %#v", delta)
	}
	// Written=false → nil delta
	resultJSON, _ := json.Marshal(setDeliverableOutput{Written: false})
	if delta := tl.StateDelta("call-1", nil, resultJSON); delta != nil {
		t.Fatalf("expected nil delta when Written=false, got: %#v", delta)
	}
}

func TestSetDeliverableTool_Call(t *testing.T) {
	tl := NewSetDeliverableTool()
	input, _ := json.Marshal(map[string]any{
		"data": map[string]any{"key": "value"},
		"note": "test note",
	})
	out, err := tl.Call(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(setDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if !o.Written {
		t.Fatal("expected Written=true")
	}
	if o.Keys != 1 {
		t.Fatalf("expected Keys=1, got %d", o.Keys)
	}
	if o.Note != "test note" {
		t.Fatalf("expected Note='test note', got %q", o.Note)
	}
}

func TestGetDeliverableTool_Declaration(t *testing.T) {
	tl := NewGetDeliverableTool()
	decl := tl.Declaration()
	if decl.Name != "get_deliverable" {
		t.Fatalf("name=%q want get_deliverable", decl.Name)
	}
	if decl.InputSchema == nil || decl.InputSchema.Type != "object" {
		t.Fatalf("input schema missing or not object: %#v", decl.InputSchema)
	}
	if decl.OutputSchema == nil {
		t.Fatal("output schema should be non-nil")
	}
}

func TestGetDeliverableTool_Call_NoInvocation(t *testing.T) {
	tl := NewGetDeliverableTool()
	// No invocation in context → Found=false, no error
	out, err := tl.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(getDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if o.Found {
		t.Fatal("expected Found=false when no invocation in context")
	}
}

func TestGetDeliverableTool_Call_WithSession(t *testing.T) {
	// Build a session with deliverable state
	sess := session.NewSession("test-app", "test-user", "test-session")
	deliverableData, _ := json.Marshal(map[string]any{
		"report":  "analysis done",
		"metrics": []any{1, 2, 3},
	})
	sess.SetState(biz.DeliverableStateKey, deliverableData)

	// Build an invocation with the session
	inv := agent.NewInvocation()
	inv.Session = sess
	ctx := agent.NewInvocationContext(context.Background(), inv)

	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(getDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if !o.Found {
		t.Fatal("expected Found=true")
	}
	if o.Data["report"] != "analysis done" {
		t.Fatalf("unexpected report: %#v", o.Data["report"])
	}
}

func TestGetDeliverableTool_Call_EmptySession(t *testing.T) {
	sess := session.NewSession("test-app", "test-user", "test-session")
	inv := agent.NewInvocation()
	inv.Session = sess
	ctx := agent.NewInvocationContext(context.Background(), inv)

	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(getDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if o.Found {
		t.Fatal("expected Found=false when deliverable not set")
	}
}

func deltaKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
