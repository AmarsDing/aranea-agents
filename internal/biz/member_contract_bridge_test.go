package biz

import (
	"strings"
	"testing"
)

// ack/ 前缀键是团队内确认信号，不得泄漏进团队间信封的 StructuredJSON。
func TestMarshalNonReservedStateKeys_ExcludesAckKeys(t *testing.T) {
	state := map[string]any{
		"summary":    "总结",
		"cognition":  map[string]any{"assumptions": []string{"a1"}},
		"research":   map[string]any{"findings": "A"},
		"ack/design": map[string]any{"status": "accepted", "by": "reviewer"},
	}
	out := marshalNonReservedStateKeys(state)
	if strings.Contains(out, "ack/") || strings.Contains(out, "accepted") {
		t.Fatalf("ack keys must be excluded from StructuredJSON, got: %s", out)
	}
	if !strings.Contains(out, "research") {
		t.Fatalf("business topic must survive, got: %s", out)
	}
	if strings.Contains(out, "总结") || strings.Contains(out, "assumptions") {
		t.Fatalf("reserved keys must stay excluded, got: %s", out)
	}
}

// 契约声明 required topic 未出现在最终 deliverable map → 完成时 advisory 名单。
func TestRequiredTopicsMissingFromState(t *testing.T) {
	defJSON := `{"enable_state_deliverable":true,"deliverable_contract":{"entries":[
		{"topic":"design","required":true},
		{"topic":"notes"}
	]},"members":[{"agent_id":"a1"}]}`
	team := Team{ID: "t1", DefinitionJSON: defJSON}

	// design 缺失 → 出现在名单；notes 非 required 不出现
	missing := requiredTopicsMissingFromState(team, map[string]any{
		"notes": map[string]any{"x": 1},
	})
	if len(missing) != 1 || missing[0] != "design" {
		t.Fatalf("expected [design], got %v", missing)
	}

	// design 已写 → 无缺失
	if got := requiredTopicsMissingFromState(team, map[string]any{
		"design": map[string]any{"arch": "x"},
	}); got != nil {
		t.Fatalf("expected none missing, got %v", got)
	}

	// 无契约团队 → 永不缺失
	noContract := Team{ID: "t2", DefinitionJSON: `{"enable_state_deliverable":true,"members":[{"agent_id":"a1"}]}`}
	if got := requiredTopicsMissingFromState(noContract, map[string]any{}); got != nil {
		t.Fatalf("no contract → nil, got %v", got)
	}
}
