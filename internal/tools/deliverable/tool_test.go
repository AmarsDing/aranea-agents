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

// 保留 key 值锚定（2026-08-15 评审修复 6）：summary/cognition 的保留 key
// 在三个包各有字面量镜像（本包私有常量、biz.deliverableReservedKeySummary、
// graph/adapter.deliverableReservedSummaryKey）。此处钉住本侧值；跨包一致
// 性由 graph/adapter 的行为锚定测试（set_deliverable 必须拒绝以该 key 作
// topic）与 biz 的值锚定测试共同保证——任何一侧漂移都会亮红。
func TestReservedKeys_ValuesPinned(t *testing.T) {
	if reservedKeySummary != "summary" {
		t.Fatalf("reservedKeySummary drifted: %q", reservedKeySummary)
	}
	if reservedKeyCognition != "cognition" {
		t.Fatalf("reservedKeyCognition drifted: %q", reservedKeyCognition)
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
		"leading_dash":  `{"data":{},"topic":"-bad"}`,
		"space":         `{"data":{},"topic":"has space"}`,
		"cjk_space":     `{"data":{},"topic":"根因 报告"}`,
		"punctuation":   `{"data":{},"topic":"rca.report"}`,
		"slash":         `{"data":{},"topic":"a/b"}`,
		"too_long":      `{"data":{},"topic":"` + strings.Repeat("报告", 33) + `"}`,
		"reserved_sum":  `{"data":{},"topic":"summary"}`,
		"reserved_cogn": `{"data":{},"topic":"cognition"}`,
	}
	for name, args := range cases {
		if _, err := tl.Call(context.Background(), []byte(args)); err == nil {
			t.Fatalf("%s: expected topic validation error", name)
		}
	}
	// Valid topics pass: ASCII slugs, mixed case, and Unicode contract names
	// (TS9-BUG-4: the planner authored the Chinese contract name "恢复执行报告",
	// which must round-trip through the MDC 1:1 topic mapping).
	for _, topic := range []string{"research", "draft_v1", "a-1", "x", "Research", "root-cause-report", "恢复执行报告", "根因报告v2"} {
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

// P2b（2026-08-21）：topic 参数归一化——写读两侧同规则（trim+NFC+全角折叠），
// LLM 转写契约名时的码点差异（全角 Ａ/－、padding 空白）不再让写键与读键错位。
func TestDeliverableTopicNormalization_WriteReadRoundTrip(t *testing.T) {
	set := NewSetDeliverableTool()
	// 全角字母/连字符 + 首尾空白写入 → 归一化为半角键 "poem-a"。
	out, err := set.Call(ctxWithDeliverableState(t, nil), []byte(`{"data":{"text":"x"},"topic":" ｐｏｅｍ－ａ "}`))
	if err != nil {
		t.Fatal(err)
	}
	merged := out.(setDeliverableOutput).Data
	if _, ok := merged["poem-a"]; !ok {
		t.Fatalf("write key not normalized: %#v", merged)
	}

	// 读取侧同规则归一：全角/空白读法命中半角写键。
	get := NewGetDeliverableTool()
	ctx := ctxWithDeliverableState(t, map[string]any{"poem-a": map[string]any{"text": "x"}})
	gOut, err := get.Call(ctx, []byte(`{"topic":" ｐｏｅｍ－ａ "}`))
	if err != nil {
		t.Fatal(err)
	}
	o := gOut.(getDeliverableOutput)
	if !o.Found || o.Topic != "poem-a" || o.Data["text"] != "x" {
		t.Fatalf("normalized topic read failed: %#v", o)
	}
}

// P2b：归一化不做大小写折叠——大小写差异是不同 topic（防契约声明的
// case 变体条目被静默合并）。
func TestDeliverableTopicNormalization_CaseSensitive(t *testing.T) {
	get := NewGetDeliverableTool()
	ctx := ctxWithDeliverableState(t, map[string]any{"Poem-A": map[string]any{"v": 1}})
	out, err := get.Call(ctx, []byte(`{"topic":"poem-a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.(getDeliverableOutput).Found {
		t.Fatal("case-distinct topic must NOT match after normalization")
	}
}

// ---------------------------------------------------------------------------
// Graph RuntimeState read path (member-to-member handoff inside a graph run)
// ---------------------------------------------------------------------------

// ctxWithRuntimeState builds an invocation context carrying the given graph
// runtime state (node-start snapshot), with no session attached.
func ctxWithRuntimeState(state map[string]any) context.Context {
	inv := agent.NewInvocation()
	if state != nil {
		inv.RunOptions.RuntimeState = state
	}
	return agent.NewInvocationContext(context.Background(), inv)
}

// get_deliverable must read the graph runtime state (node-start snapshot
// containing upstream members' writes) even when the session has no state.
func TestGetDeliverableTool_Call_RuntimeState(t *testing.T) {
	ctx := ctxWithRuntimeState(map[string]any{
		biz.DeliverableStateKey: map[string]any{"secret": "402"},
	})
	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"key":"secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found {
		t.Fatalf("expected Found=true from RuntimeState, got %#v", o)
	}
	if o.Data["secret"] != "402" {
		t.Fatalf("unexpected data: %#v", o.Data)
	}
}

// RuntimeState takes precedence over session state when both are present:
// inside a graph run the node-start snapshot is the authoritative view.
func TestGetDeliverableTool_Call_RuntimeStatePrecedence(t *testing.T) {
	sess := session.NewSession("test-app", "test-user", "test-session")
	b, _ := json.Marshal(map[string]any{"k": "from-session"})
	sess.SetState(biz.DeliverableStateKey, b)
	inv := agent.NewInvocation()
	inv.Session = sess
	inv.RunOptions.RuntimeState = map[string]any{
		biz.DeliverableStateKey: map[string]any{"k": "from-graph"},
	}
	ctx := agent.NewInvocationContext(context.Background(), inv)

	tl := NewGetDeliverableTool()
	out, err := tl.Call(ctx, []byte(`{"key":"k"}`))
	if err != nil {
		t.Fatal(err)
	}
	o := out.(getDeliverableOutput)
	if !o.Found || o.Data["k"] != "from-graph" {
		t.Fatalf("RuntimeState must win over session, got %#v", o)
	}
}

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
