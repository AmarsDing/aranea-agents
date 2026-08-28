package agent

import (
	"fmt"
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// S02 事故复刻（session-eval-20260827/S02-clarify）：同一 invocation 内
// 2 次空 memory_search → 24 次异质 tool_load/plan/datetime 交错 → 第 3 次
// memory_search 必须被空结果熔断拦截。该序列实证绕过了同参/轮换/空转轮次
// 三重规则，本测试钉死空结果熔断在此形态下的行为。
//
// Q1 行为模式闸上线后的隔离说明：原序列的 tool_load 含大量重复装载
// （subagents_list ×4 / get_team_deliverable ×5 …），会命中装载闸的重复
// 拦截——那正是 S02 根修的目标行为（由
// TestLoopGuardS02Replay_RunawayLoadSeqNowStopped 专验）。本测试聚焦空结果
// 熔断，故装载段使用全异质工具名并抬高配额，剥离装载闸干扰。
func TestLoopGuardS02Replay_EmptyStreakSurvivesHeterogeneousInterleave(t *testing.T) {
	g := newToolLoopGuard(nil)
	g.setGateThresholds("", 100, 0, 0) // 抬高装载配额：本测试不验装载闸
	g.setBootstrapRatio(-1)            // 关闭占比闸：本测试只验空结果熔断
	g.setPlanDrift(-1)                 // 关闭计划漂移拦截：本测试要跑完 23 次装载
	ctx := newTestInvocationContext("inv-s02-replay")
	empty := mustJSONValue(t, `{"query":"q","results":[],"count":0}`)
	loadOK := mustJSONValue(t, `{"success":true,"tool_name":"x"}`)

	// t2 开头两次空 memory_search（不同 query，空熔断无视参数差异）。
	for _, q := range []string{"报告 需求 偏好 主题", "季度 部门支出 CFO 预算 80万 团队建设 成本优化"} {
		if err := runLoopGuardTurn(t, g, ctx, "memory_search", `{"query":"`+q+`"}`, empty, nil); err != nil {
			t.Fatalf("memory_search empty call should pass, got blocked: %v", err)
		}
	}
	// 交错段：plan_and_execute / datetime / tool_search / 23 次异质 tool_load。
	if err := runLoopGuardTurn(t, g, ctx, "plan_and_execute", `{"task_prompt":"生成支出简报"}`, mustJSONValue(t, `{"plan_id":"tp_1","strategy":"direct","subtask_count":0}`), nil); err != nil {
		t.Fatalf("plan call: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "datetime", `{}`, mustJSONValue(t, `{"date":"2026-08-27"}`), nil); err != nil {
		t.Fatalf("datetime call: %v", err)
	}
	if err := runLoopGuardTurn(t, g, ctx, "tool_search", `{"query":"list files read file"}`, mustJSONValue(t, `{"tools":[{"name":"working_memory_list"}]}`), nil); err != nil {
		t.Fatalf("tool_search call: %v", err)
	}
	baseSeq := []string{
		"subagents_spawn", "subagents_get", "get_team_deliverable", "subagents_list",
		"working_memory_list", "synthesize_results", "list_agent_sessions", "search_messages",
	}
	for i := 0; i < 23; i++ {
		name := fmt.Sprintf("%s_x%02d", baseSeq[i%len(baseSeq)], i) // 全异质，剥离装载闸
		if err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, loadOK, nil); err != nil {
			t.Fatalf("tool_load %s should pass (distinct, quota raised): %v", name, err)
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

// S02 根修验证（Q1 行为模式闸）：按事故原始 tool_load 序列（含重复装载）
// 重放，装载闸必须在第 6 次调用（首个重复装载）起拦截，并在累计 5 次拦截
// 后升级 StopError 强制终止节点——失控的 23 次装载实际只发生 7 次异质
// 装载 + 5 次被拦尝试，节点在第 12 次调用时被终结。
func TestLoopGuardS02Replay_RunawayLoadSeqNowStopped(t *testing.T) {
	g := newToolLoopGuard(nil)
	ctx := newTestInvocationContext("inv-s02-runaway")
	loadOK := mustJSONValue(t, `{"success":true,"tool_name":"x"}`)

	// S02 原始装载序列（重复装载是失控特征之一）。
	loadSeq := []string{
		"subagents_spawn", "subagents_get", "get_team_deliverable", "subagents_list",
		"working_memory_list", "subagents_list", "get_team_deliverable", "synthesize_results",
		"get_team_deliverable", "synthesize_results", "list_agent_sessions", "get_team_deliverable",
		"list_agent_sessions", "list_agent_sessions", "get_team_deliverable", "list_agent_sessions",
		"subagents_spawn", "search_messages", "search_messages", "subagents_get", "subagents_spawn",
		"subagents_list", "search_messages",
	}
	passed := 0
	blocked := 0
	stopAt := -1
	for i, name := range loadSeq {
		// 结果中的 tool_name 与请求同名（装载成功语义）。
		err := runLoopGuardTurn(t, g, ctx, "tool_load", `{"tool_name":"`+name+`"}`, mustJSONValue(t, `{"success":true,"tool_name":"`+name+`"}`), nil)
		if err == nil {
			passed++
			continue
		}
		if _, ok := trpcagent.AsStopError(err); ok {
			stopAt = i
			break
		}
		if !strings.HasPrefix(err.Error(), loopGuardMarker) {
			t.Fatalf("load %d (%s) block should carry guard marker, got %q", i, name, err.Error())
		}
		blocked++
	}
	if stopAt == -1 {
		t.Fatal("runaway load sequence must be force-stopped by saturation StopError")
	}
	// C1 占比闸 + 重复装载：连续自举在第 6 次窗口命中占比闸，节点在饱和前
	// 被拦住，不会跑完 23 次。通过次数远低于事故观测的 24。
	if passed > 6 {
		t.Fatalf("ratio+repeat should stop runaway before quota-8, passed=%d", passed)
	}
	if blocked+1 < loopGuardSaturatedStopThreshold-1 {
		t.Fatalf("expected saturation after several blocks, passed=%d blocked=%d stopAt=%d", passed, blocked, stopAt)
	}
	_ = loadOK
}
