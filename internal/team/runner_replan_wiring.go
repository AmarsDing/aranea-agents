package team

import (
	graphadapter "aranea-agents/internal/graph/adapter"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// replanCallbacksRunOption 构造 replanner 全局回调的 run 级注入 option
// （ADR-F D2：team 域接线——静态声明由节点级 per-node 回调先处理，本注入的
// 全局回调仅作为未恢复失败的智能兜底；GraphAgent.Run 会把 RuntimeState 合并
// 进 initialState 供 executor getMergedCallbacks 消费）。
//
// 仅 graph 模式（graphExecID 非空）且配置了 replanner 时返回非 nil；其余场景
// 保持纯静态路径（现状行为）。DAG 派步团队无 graph executor，callbacks 无消费
// 者，不注入。
func (r *Runner) replanCallbacksRunOption(sessID, spiritSessionID, graphID, graphExecID string) trpcagent.RunOption {
	if r.cfg.Replanner == nil || graphExecID == "" {
		return nil
	}
	cb := graphadapter.NewReplanNodeCallbacks(r.cfg.Replanner, r.lg, sessID, spiritSessionID, graphID, graphExecID)
	if cb == nil {
		return nil
	}
	return trpcagent.MergeRuntimeState(map[string]any{trpcgraph.StateKeyNodeCallbacks: cb})
}
