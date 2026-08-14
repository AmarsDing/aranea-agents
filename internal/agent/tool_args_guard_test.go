package agent

import (
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestToolArgsGuard_stripsSystemFields(t *testing.T) {
	hook := newToolArgsGuardBeforeHook(nil)
	args, _ := json.Marshal(map[string]any{
		"query":      "hello",
		"agent_id":   "should-remove",
		"session_id": "also-remove",
	})
	btArgs := &trpctool.BeforeToolArgs{Arguments: args}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil || res == nil {
		t.Fatalf("hook failed: %v", err)
	}
	// P1-3: 清洗结果经 ModifiedArguments 返回（框架唯一写回通道）。
	if res.ModifiedArguments == nil {
		t.Fatal("cleaned args must be returned via ModifiedArguments")
	}
	var out map[string]any
	if err := json.Unmarshal(res.ModifiedArguments, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["agent_id"]; ok {
		t.Fatalf("agent_id not stripped: %v", out)
	}
	if out["query"] != "hello" {
		t.Fatalf("query mutated: %v", out)
	}
}

// P1-3 / B-NEW：剥离系统字段后的参数必须经 ModifiedArguments 返回，
// 否则框架不会把清洗结果写回实际执行的 toolCall.Function.Arguments。
func TestToolArgsGuard_RewriteReachesFramework(t *testing.T) {
	hook := newToolArgsGuardBeforeHook(nil)
	args, _ := json.Marshal(map[string]any{
		"query":    "hello",
		"agent_id": "should-remove",
	})
	btArgs := &trpctool.BeforeToolArgs{Arguments: args}
	res, err := hook.HandleBeforeTool(context.Background(), btArgs)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if res == nil || res.ModifiedArguments == nil {
		t.Fatal("cleaned args must be returned via ModifiedArguments to reach tool execution")
	}
	var out map[string]any
	if err := json.Unmarshal(res.ModifiedArguments, &out); err != nil {
		t.Fatalf("ModifiedArguments not valid JSON: %v", err)
	}
	if _, ok := out["agent_id"]; ok {
		t.Fatalf("agent_id not stripped in ModifiedArguments: %v", out)
	}
}
