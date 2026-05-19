package biz

import "testing"

func TestParseHookConfig_andMatch(t *testing.T) {
	cfg, err := ParseHookConfig(`{
		"callback_point": "before_tool",
		"condition": {"agent_id": "ag-1", "tool_name": "search"},
		"action": {"type": "block", "message": "denied"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CallbackPoint != "before_tool" {
		t.Fatalf("point=%q", cfg.CallbackPoint)
	}
	if !HookAppliesToAgent(cfg.Condition, "ag-1", "key-1") {
		t.Fatal("expected agent id match")
	}
	if HookAppliesToAgent(cfg.Condition, "x", "key-1") {
		t.Fatal("expected no match for other agent id")
	}
	cond := HookCondition{AgentID: "key-1"}
	if !HookAppliesToAgent(cond, "x", "key-1") {
		t.Fatal("expected agent key match")
	}
	if !HookAppliesToTool(cfg.Condition, "search") {
		t.Fatal("expected tool match")
	}
	if HookAppliesToTool(cfg.Condition, "other") {
		t.Fatal("expected tool mismatch")
	}
}

func TestNormalizeCallbackPoint(t *testing.T) {
	if got := NormalizeCallbackPoint("BeforeModel"); got != "before_model" {
		t.Fatalf("got %q", got)
	}
}
