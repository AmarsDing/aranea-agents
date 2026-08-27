package agent

import (
	"strings"
	"testing"
)

// S02 事故复刻（session-eval-20260827/S02-clarify）：同一 invocation 内
// 2 次空 memory_search → 24 次异质 tool_load/plan/datetime 交错 → 第 3 次
// memory_search 必须被空结果熔断拦截。该序列实证绕过了同参/轮换/空转轮次
// 三重规则，本测试钉死空结果熔断在此形态下的行为。
func TestLoopGuardS02Replay_EmptyStreakSurvivesHeterogeneousInterleave(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-s02-replay")
	empty := mustJSONValue(t, `{"query":"q","results":[],"count":0}`)
	loadOK := mustJSONValue(t, `{"success":true,"tool_name":"x"}`)

	// t2 开头两次空 memory_search（不同 query，空熔断无视参数差异）。
	for _, q := range []string{"报告 需求 偏好 主题", "季度 部门支出 CFO 预算 80万 团队建设 成本优化"} {
		if err := runLoopGuardTurn(t, g, ctx, "memory_search", `{"query":"`+q+`"}`, empty, nil); err != nil {
			t.Fatalf("memory_search empty call should pass, got blocked: %v", err)
		}
	}
	// 交错段：plan_and_execute / datetime / tool_search / 24 次异质 tool_load。
	if err := runLoopGuardTurn(t, g, ctx, "plan_and_execute", `{"task_prompt":"生成支出简报"}`, mustJSONValue(t, `{"plan_id":"tp_1","strategy":"direct","subtask_count":0}`), nil); err != nil {
		t.Fatalf("plan call: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, mustJSONValue(t, `{"date":"2026-08-27"}`), nil); err != nil {
		t.Fatalf("datetime call: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "tool_search", `{"query":"list files read file"}`, mustJSONValue(t, `{"tools":[{"name":"working_memory_list"}]}`), nil); err != nil {
		t.Fatalf("tool_search call: %v", err)
	}
	loadSeq := []string{
		"subagents_spawn", "subagents_get", "get_team_deliverable", "subagents_list",
		"working_memory_list", "subagents_list", "get_team_deliverable", "synthesize_results",
		"get_team_deliverable", "synthesize_results", "list_agent_sessions", "get_team_deliverable",
		"list_agent_sessions", "list_agent_sessions", "get_team_deliverable", "list_agent_sessions",
		"subagents_spawn", "search_messages", "search_messages", "subagents_get", "subagents_spawn",
		"subagents_list", "search_messages",
	}
	for _, name := range loadSeq {
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, loadOK, nil); err != nil {
			t.Fatalf("tool_load %s should pass under current rules: %v", name, err)
		}
	}
	// 第 3 次 memory_search：streak=2 已达阈值，必须熔断。
	err := runLoopGuardTurn(t, g, ctx, "memory_search", `{"query":"部门支出 数据文件 财务 季度报告"}`, empty, nil)
	if err == nil {
		t.Fatal("3rd memory_search after 2 consecutive empties must be blocked, even with heterogeneous interleave")
	}
	if !strings.HasPrefix(err.Error(), loopGuardMarker) {
		t.Fatalf("blocked error should carry loop guard marker, got %q", err.Error())
	}
}
