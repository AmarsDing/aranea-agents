package deliverable

import (
	"context"
	"encoding/json"
	"strings"
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

// ---------------------------------------------------------------------------
// C3: topic namespace (shared blackboard)
// ---------------------------------------------------------------------------

// ctxWithDeliverableState builds an invocation context whose session carries
// the given deliverable map (nil → no state set).
func ctxWithDeliverableState(t *testing.T, state map[string]any) context.Context {
	t.Helper()
	sess := session.NewSession("test-app", "test-user", "test-session")
	if state != nil {
		b, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		sess.SetState(biz.DeliverableStateKey, b)
	}
	inv := agent.NewInvocation()
	inv.Session = sess
	return agent.NewInvocationContext(context.Background(), inv)
}

// Legacy semantics: no topic → the written map is exactly the input data.
func TestSetDeliverableTool_Call_NoTopic_LegacyOverwrite(t *testing.T) {
	ctx := ctxWithDeliverableState(t, map[string]any{"old": "stays?"})
	tl := NewSetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"data":{"new":"value"}}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	if o.Topic != "" {
		t.Fatalf("expected empty topic, got %q", o.Topic)
	}
	if _, hasOld := o.Data["old"]; hasOld {
		t.Fatalf("legacy overwrite must not merge existing state, got %#v", o.Data)
	}
	if o.Data["new"] != "value" || len(o.Data) != 1 {
		t.Fatalf("unexpected data: %#v", o.Data)
	}
}

// Topic merge into an empty state.
func TestSetDeliverableTool_Call_Topic_EmptyState(t *testing.T) {
	ctx := ctxWithDeliverableState(t, nil)
	tl := NewSetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"data":{"a":1},"topic":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	if o.Topic != "research" {
		t.Fatalf("topic=%q want research", o.Topic)
	}
	sub, ok := o.Data["research"].(map[string]any)
	if !ok || sub["a"] != float64(1) {
		t.Fatalf("expected deliverable[research].a=1, got %#v", o.Data)
	}
}

// Topic merge preserves other existing topics and reserved keys.
func TestSetDeliverableTool_Call_Topic_PreservesOthers(t *testing.T) {
	ctx := ctxWithDeliverableState(t, map[string]any{
		"summary":   "old summary",
		"cognition": map[string]any{"assumptions": []any{"a1"}},
		"draft":     map[string]any{"v": 1},
	})
	tl := NewSetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"data":{"x":true},"topic":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	if o.Data["summary"] != "old summary" {
		t.Fatalf("existing summary must survive topic write, got %#v", o.Data)
	}
	if _, ok := o.Data["cognition"]; !ok {
		t.Fatalf("existing cognition must survive topic write, got %#v", o.Data)
	}
	if _, ok := o.Data["draft"]; !ok {
		t.Fatalf("existing topic draft must survive, got %#v", o.Data)
	}
	if _, ok := o.Data["research"].(map[string]any); !ok {
		t.Fatalf("research topic missing, got %#v", o.Data)
	}
}

// Writing the same topic twice replaces that topic only.
func TestSetDeliverableTool_Call_Topic_OverwriteSameTopic(t *testing.T) {
	ctx := ctxWithDeliverableState(t, map[string]any{
		"research": map[string]any{"old": true},
		"draft":    map[string]any{"keep": true},
	})
	tl := NewSetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"data":{"new":true},"topic":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	sub := o.Data["research"].(map[string]any)
	if _, hasOld := sub["old"]; hasOld {
		t.Fatalf("same-topic write must replace the topic, got %#v", sub)
	}
	if sub["new"] != true {
		t.Fatalf("same-topic write lost new data, got %#v", sub)
	}
	if _, ok := o.Data["draft"]; !ok {
		t.Fatalf("other topic must survive, got %#v", o.Data)
	}
}

// Invalid topic names are rejected with an LLM-actionable error.
func TestSetDeliverableTool_Call_TopicValidation(t *testing.T) {
	tl := NewSetDeliverableTool()
	cases := map[string]string{
		"uppercase":     `{"data":{},"topic":"Research"}`,
		"leading_dash":  `{"data":{},"topic":"-bad"}`,
		"space":         `{"data":{},"topic":"has space"}`,
		"too_long":      `{"data":{},"topic":"` + strings.Repeat("a", 65) + `"}`,
		"reserved_sum":  `{"data":{},"topic":"summary"}`,
		"reserved_cogn": `{"data":{},"topic":"cognition"}`,
	}
	for name, args := range cases {
		if _, err := tl.Call(context.Background(), []byte(args)); err == nil {
			t.Fatalf("%s: expected topic validation error", name)
		}
	}
	// Valid slugs pass.
	for _, topic := range []string{"research", "draft_v1", "a-1", "x"} {
		args := []byte(`{"data":{"k":1},"topic":"` + topic + `"}`)
		if _, err := tl.Call(context.Background(), args); err != nil {
			t.Fatalf("topic %q should be valid: %v", topic, err)
		}
	}
}

// C1: cognition lands under the reserved key, without mutating the input data.
func TestSetDeliverableTool_Call_CognitionReservedKey(t *testing.T) {
	tl := NewSetDeliverableTool()
	args := []byte(`{
		"data": {"report": "done"},
		"cognition": {
			"decisions": [{"choice": "A", "rationale": "cheaper", "confidence": 0.8}],
			"rejected": [{"option": "B", "reason": "slow"}],
			"assumptions": ["data frozen"],
			"open_questions": ["bias?"]
		}
	}`)
	out, err := tl.Call(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	cog, ok := o.Data["cognition"].(*biz.DeliverableCognition)
	if !ok {
		t.Fatalf("cognition reserved key missing or wrong type: %#v", o.Data["cognition"])
	}
	if len(cog.Decisions) != 1 || cog.Decisions[0].Choice != "A" || cog.Decisions[0].Confidence != 0.8 {
		t.Fatalf("unexpected decisions: %#v", cog.Decisions)
	}
	if len(cog.Rejected) != 1 || cog.Rejected[0].Option != "B" {
		t.Fatalf("unexpected rejected: %#v", cog.Rejected)
	}
	if len(cog.Assumptions) != 1 || len(cog.OpenQuestions) != 1 {
		t.Fatalf("unexpected assumptions/questions: %#v", cog)
	}
	if o.Data["report"] != "done" {
		t.Fatalf("business data lost: %#v", o.Data)
	}

	// The reserved key must survive StateDelta → JSON round-trip.
	delta := tl.StateDelta("call-1", nil, mustJSON(t, o))
	if delta == nil {
		t.Fatal("expected non-nil delta")
	}
	var stored map[string]any
	if err := json.Unmarshal(delta[biz.DeliverableStateKey], &stored); err != nil {
		t.Fatal(err)
	}
	cogMap, ok := stored["cognition"].(map[string]any)
	if !ok {
		t.Fatalf("stored cognition missing: %#v", stored)
	}
	if _, ok := cogMap["decisions"].([]any); !ok {
		t.Fatalf("stored cognition decisions missing: %#v", cogMap)
	}
}

// C1+C3: cognition + topic together — cognition goes to the reserved key,
// data under the topic.
func TestSetDeliverableTool_Call_CognitionWithTopic(t *testing.T) {
	ctx := ctxWithDeliverableState(t, map[string]any{"draft": map[string]any{"v": 1}})
	tl := NewSetDeliverableTool()
	args := []byte(`{
		"data": {"x": 1},
		"topic": "research",
		"cognition": {"assumptions": ["a1"]}
	}`)
	out, err := tl.Call(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	o := out.(setDeliverableOutput)
	if _, ok := o.Data["research"].(map[string]any); !ok {
		t.Fatalf("topic missing: %#v", o.Data)
	}
	if _, ok := o.Data["cognition"].(*biz.DeliverableCognition); !ok {
		t.Fatalf("cognition missing: %#v", o.Data)
	}
	if _, ok := o.Data["draft"]; !ok {
		t.Fatalf("existing topic must survive: %#v", o.Data)
	}
}

func TestGetDeliverableTool_Call_Topic(t *testing.T) {
	ctx := ctxWithDeliverableState(t, map[string]any{
		"research": map[string]any{"a": 1, "b": 2},
		"draft":    map[string]any{"v": 3},
	})
	tl := NewGetDeliverableTool()

	// Full topic sub-object.
	out, err := tl.Call(ctx, []byte(`{"topic":"research"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found || o.Topic != "research" || o.Data["a"] != float64(1) || o.Data["b"] != float64(2) {
		t.Fatalf("unexpected topic read: %#v", o)
	}

	// Key filter within the topic.
	out, err = tl.Call(ctx, []byte(`{"topic":"research","key":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	o = out.(getDeliverableOutput)
	if !o.Found || len(o.Data) != 1 || o.Data["b"] != float64(2) {
		t.Fatalf("unexpected topic+key read: %#v", o)
	}

	// Unknown topic → found=false, no error.
	out, err = tl.Call(ctx, []byte(`{"topic":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.(getDeliverableOutput).Found {
		t.Fatal("unknown topic must yield found=false")
	}

	// Topic pointing at a non-object value → found=false.
	ctx2 := ctxWithDeliverableState(t, map[string]any{"summary": "text"})
	out, err = tl.Call(ctx2, []byte(`{"topic":"summary"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.(getDeliverableOutput).Found {
		t.Fatal("non-object topic value must yield found=false")
	}

	// Unknown key within an existing topic → found=false.
	out, err = tl.Call(ctx, []byte(`{"topic":"research","key":"zzz"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.(getDeliverableOutput).Found {
		t.Fatal("unknown key within topic must yield found=false")
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
