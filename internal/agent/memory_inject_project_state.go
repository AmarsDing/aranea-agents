package agent

import (
	"encoding/json"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// projectStateCueBudgetRunes 是 P2-4 project-state 切片的注入预算（rune）。
// Ensemble QSP 有界记忆实测中位注入 ~300 token ≈ 600 中文 rune；硬帽由
// biz.TeamProjectState.RenderSlice 保证（严格不越帽，不加省略号）。
const projectStateCueBudgetRunes = 600

// ProjectStateCueFromInvocation 从 invocation 读取 P2-4 结构化项目状态并
// 按预算渲染注入切片。读路径与 deliverable 工具一致：RuntimeState（graph
// 运行内的 node-start 快照，权威）→ session state（运行外/跨 run 回退）。
// 未携带 project_state（非团队运行或 enable_project_state 未开启）时返回
// ""——调用方零分支开销。
func ProjectStateCueFromInvocation(inv *trpcagent.Invocation, budgetRunes int) string {
	m, ok := projectStateMapFromInvocation(inv)
	if !ok {
		return ""
	}
	return biz.TeamProjectStateFromMap(m).RenderSlice(budgetRunes)
}

// projectStateMapFromInvocation 的 RuntimeState 分支遵循 deliverable 的
// 权威性约定：key 存在即为权威（哪怕是空 map），不回退 session——避免上一
// turn 的会话残留漏进正在运行的 graph。
func projectStateMapFromInvocation(inv *trpcagent.Invocation) (map[string]any, bool) {
	if inv == nil {
		return nil, false
	}
	if inv.RunOptions.RuntimeState != nil {
		if raw, found := inv.RunOptions.RuntimeState[biz.ProjectStateKey]; found {
			if m, ok := toProjectStateMap(raw); ok {
				return m, true
			}
		}
	}
	if inv.Session != nil {
		if raw, found := inv.Session.GetState(biz.ProjectStateKey); found && len(raw) > 0 {
			var out map[string]any
			if err := json.Unmarshal(raw, &out); err == nil {
				return out, true
			}
		}
	}
	return nil, false
}

// toProjectStateMap 规范化 state 值：已解码 map 直通，session 种子的 raw
// JSON bytes 反序列化。
func toProjectStateMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case []byte:
		if len(t) == 0 {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(t, &out); err != nil {
			return nil, false
		}
		return out, true
	}
	return nil, false
}
