package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// runRepairHook executes the repair hook and returns the effective arguments
// afterwards: ModifiedArguments when the hook rewrote them (P1-3 — the only
// channel the framework writes back), else the original passthrough args.
func runRepairHook(t *testing.T, toolName string, raw []byte) []byte {
	t.Helper()
	hook := newToolArgsRepairBeforeHook(nil)
	bt := &trpctool.BeforeToolArgs{ToolName: toolName, Arguments: raw}
	res, err := hook.HandleBeforeTool(context.Background(), bt)
	if err != nil || res == nil {
		t.Fatalf("hook failed: res=%v err=%v", res, err)
	}
	if res.ModifiedArguments != nil {
		return res.ModifiedArguments
	}
	return bt.Arguments
}

func mustValidObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("repaired args are not a valid JSON object: %v\nraw: %s", err, raw)
	}
	return m
}

func TestToolArgsRepair_validPassthrough(t *testing.T) {
	in := []byte(`{"a":1,"b":"x"}`)
	out := runRepairHook(t, "set_deliverable", in)
	if string(out) != string(in) {
		t.Fatalf("valid args must pass through unchanged, got: %s", out)
	}
}

func TestToolArgsRepair_emptyAndNil(t *testing.T) {
	if out := runRepairHook(t, "read_file", nil); out != nil {
		t.Fatalf("nil args must remain nil, got: %s", out)
	}
	if out := runRepairHook(t, "read_file", []byte{}); len(out) != 0 {
		t.Fatalf("empty args must remain empty, got: %s", out)
	}
}

// 2026-07-25 22:33 incident: model emitted a complete JSON object followed by
// one extra '}' — killed both teams via deliverable gate.
func TestToolArgsRepair_trailingExtraBrace(t *testing.T) {
	in := []byte(`{"data":{"summary":"s"},"topic":"t"}}`)
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["topic"] != "t" {
		t.Fatalf("topic lost in repair: %v", m)
	}
	if data, ok := m["data"].(map[string]any); !ok || data["summary"] != "s" {
		t.Fatalf("nested data lost in repair: %v", m)
	}
}

func TestToolArgsRepair_trailingComma(t *testing.T) {
	in := []byte(`{"a":1},`)
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["a"] != json.Number("1") {
		t.Fatalf("unexpected content: %v", m)
	}
}

func TestToolArgsRepair_trailingGarbageText(t *testing.T) {
	in := []byte(`{"a":1} leftover text from model`)
	out := runRepairHook(t, "set_deliverable", in)
	mustValidObject(t, out)
}

// 参数质量信号：修复成功必须在 ctx 标记 Repaired，供 AfterTool recorder
// 落库与计数（29-token WP：工具一次成功率度量闭环）。
func TestToolArgsRepair_MarksRepairedInContext(t *testing.T) {
	hook := newToolArgsRepairBeforeHook(nil)
	bt := &trpctool.BeforeToolArgs{ToolName: "set_deliverable", Arguments: []byte(`{"a":1},`)}
	res, err := hook.HandleBeforeTool(context.Background(), bt)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	q := toolArgsQualityFromContext(res.Context)
	if !q.Repaired || q.Invalid {
		t.Fatalf("want Repaired=true Invalid=false, got %+v", q)
	}
}

// P1-3 / B-NEW：修复后的参数必须经 BeforeToolResult.ModifiedArguments 返回——
// 这是框架唯一写回 toolCall.Function.Arguments 的通道
// （internal/flow/processor/functioncall.go runBeforeToolCallbacks）。
// 仅原地改 args.Arguments 不会到达工具执行（2026-07-25 事故修复的执行路径
// 在此前一值为空，属潜伏 bug）。
func TestToolArgsRepair_RewriteReachesFramework(t *testing.T) {
	hook := newToolArgsRepairBeforeHook(nil)
	bt := &trpctool.BeforeToolArgs{ToolName: "set_deliverable", Arguments: []byte(`{"a":1},`)}
	res, err := hook.HandleBeforeTool(context.Background(), bt)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if res == nil || res.ModifiedArguments == nil {
		t.Fatal("repaired args must be returned via ModifiedArguments to reach tool execution")
	}
	if !json.Valid(res.ModifiedArguments) {
		t.Fatalf("ModifiedArguments not valid JSON: %s", res.ModifiedArguments)
	}
}

// 不可修复的非法 JSON 必须标记 Invalid（原样透传给工具层报错）。
func TestToolArgsRepair_MarksInvalidWhenUnrepairable(t *testing.T) {
	hook := newToolArgsRepairBeforeHook(nil)
	bt := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: []byte(`{"path": `)}
	res, err := hook.HandleBeforeTool(context.Background(), bt)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	q := toolArgsQualityFromContext(res.Context)
	if q.Repaired || !q.Invalid {
		t.Fatalf("want Repaired=false Invalid=true, got %+v", q)
	}
}

func TestToolArgsRepair_ValidArgsNoMarkers(t *testing.T) {
	hook := newToolArgsRepairBeforeHook(nil)
	bt := &trpctool.BeforeToolArgs{ToolName: "read_file", Arguments: []byte(`{"path":"a"}`)}
	res, err := hook.HandleBeforeTool(context.Background(), bt)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	q := toolArgsQualityFromContext(res.Context)
	if q.Repaired || q.Invalid {
		t.Fatalf("valid args must not be marked, got %+v", q)
	}
}

// 2026-07-25 14:26 incident: raw newline inside a string literal.
func TestToolArgsRepair_rawNewlineInString(t *testing.T) {
	in := []byte("{\"summary\":\"line1\nline2\"}")
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["summary"] != "line1\nline2" {
		t.Fatalf("newline content mismatch: %q", m["summary"])
	}
}

func TestToolArgsRepair_rawTabAndCRInString(t *testing.T) {
	in := []byte("{\"s\":\"a\tb\rc\"}")
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["s"] != "a\tb\rc" {
		t.Fatalf("control char content mismatch: %q", m["s"])
	}
}

// Real-world payload shape from the 22:33 incident: nested data + topic +
// cognition, terminated by an extra '}'.
func TestToolArgsRepair_realWorldSetDeliverablePayload(t *testing.T) {
	in := []byte(`{"data":{"summary":"团队协作文档","contract_team_collab_doc":{"name":"团队与协作建议文档","sections":["团队角色分工","协作流程"]}},"topic":"team-collaboration-doc","cognition":{"assumptions":["团队规模 5-20 人"],"decisions":[{"choice":"Scrumban","rationale":"兼顾节奏与灵活"}],"open_questions":[],"rejected":[{"option":"纯 Scrum","reason":"Sprint 边界造成等待"}]}}}`)
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	cog, ok := m["cognition"].(map[string]any)
	if !ok {
		t.Fatalf("cognition lost: %v", m)
	}
	decisions, ok := cog["decisions"].([]any)
	if !ok || len(decisions) != 1 {
		t.Fatalf("decisions lost: %v", cog)
	}
	if d0, _ := decisions[0].(map[string]any); d0["choice"] != "Scrumban" {
		t.Fatalf("decision content mismatch: %v", d0)
	}
	if m["topic"] != "team-collaboration-doc" {
		t.Fatalf("topic mismatch: %v", m["topic"])
	}
}

// Combined corruption: raw newline inside a string AND a trailing extra brace
// requires the escape stage before the first-value decode stage.
func TestToolArgsRepair_newlinePlusTrailingBrace(t *testing.T) {
	in := []byte("{\"s\":\"a\nb\"}}")
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["s"] != "a\nb" {
		t.Fatalf("content mismatch: %q", m["s"])
	}
}

func TestToolArgsRepair_truncatedUnrepairable(t *testing.T) {
	in := []byte(`{"a":1`)
	out := runRepairHook(t, "set_deliverable", in)
	if string(out) != string(in) {
		t.Fatalf("truncated args must pass through unchanged, got: %s", out)
	}
}

func TestToolArgsRepair_plainGarbageUnrepairable(t *testing.T) {
	in := []byte("not-json")
	out := runRepairHook(t, "set_deliverable", in)
	if string(out) != string(in) {
		t.Fatalf("non-JSON args must pass through unchanged, got: %s", out)
	}
}

// Tool arguments must be JSON objects; never "repair" an array payload by
// truncating it into something the tool will misread.
func TestToolArgsRepair_nonObjectRejected(t *testing.T) {
	in := []byte(`[1,2,3]}}}`)
	out := runRepairHook(t, "set_deliverable", in)
	if string(out) != string(in) {
		t.Fatalf("array args must pass through unchanged, got: %s", out)
	}
}

// Large integers must survive the decode→re-marshal round trip (UseNumber).
func TestToolArgsRepair_largeNumberPreserved(t *testing.T) {
	in := []byte(`{"id":9007199254740993}}`)
	out := runRepairHook(t, "set_deliverable", in)
	m := mustValidObject(t, out)
	if m["id"] != json.Number("9007199254740993") {
		t.Fatalf("large int corrupted: %v (%T)", m["id"], m["id"])
	}
}
