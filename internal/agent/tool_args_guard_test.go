package agent

import (
	"context"
	"encoding/json"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestToolArgsGuard_stripsSystemFields(t *testing.T) {
	hook := newToolArgsGuardBeforeHook()
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
	var out map[string]any
	if err := json.Unmarshal(btArgs.Arguments, &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["agent_id"]; ok {
		t.Fatalf("agent_id not stripped: %v", out)
	}
	if out["query"] != "hello" {
		t.Fatalf("query mutated: %v", out)
	}
}
