package adapter

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/graph"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// replanRetryFunc 在 AfterNode 内同步重执行失败节点（Reflexion 智能重试）。
// feedback 为失败反馈文本；实现必须保证原 state 零污染（checkpoint 隔离）。
type replanRetryFunc func(ctx context.Context, state trpcgraph.State, nodeID, feedback string) (any, error)

// NewReplanNodeCallbacks 构造 RuntimeReplanner 的 NodeCallbacks（ADR-F D2：
// graph run 域与 team 域共用同一构造）。
//
// 仲裁顺序（框架事实，executor.go mergeNodeCallbacks：AfterNode per-node 先、
// global 后）：节点静态声明（fallback_agent / on_failure=skip）先跑，恢复成功
// 则 nodeErr==nil，本回调自动跳过；仅未声明或恢复失败的失败进入智能轨。
//
// 落地语义（ADR-F D3，fail-closed）：
//   - retry（transient）→ Reflexion 智能重试（仅 agent 节点）：失败反馈注入
//     user_input 副本后同步重执行；成功即恢复，失败/非 agent 节点传播错误。
//   - insert_fallback → InterruptError HITL（静态声明场景已由 per-node 轨处理）。
//   - reroute → 退化为 skip（SkippedNodesStateKey 标记，不改框架拓扑）。
//   - rebuild_subgraph / unknown / replanner 异常 → 传播原始错误。
//
// 返回 nil（replanner 未配置）时调用方跳过 StateKeyNodeCallbacks 注入。
func NewReplanNodeCallbacks(
	replanner graph.RuntimeReplanner,
	lg loggateway.Logger,
	sessionID, spiritSessionID, graphID, execID string,
) *trpcgraph.NodeCallbacks {
	if replanner == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	cb := trpcgraph.NewNodeCallbacks()

	// OnNodeError：仅观测——节点失败事实落进程日志（重试中间态也会触发）。
	cb.RegisterOnNodeError(func(ctx context.Context, cbCtx *trpcgraph.NodeCallbackContext, _ trpcgraph.State, err error) {
		if cbCtx == nil || err == nil {
			return
		}
		lg.Warn("graph node execution failed",
			loggateway.StepID("graph.node_error"),
			loggateway.Str("execution_id", execID),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("graph_id", graphID),
			loggateway.Str("failed_node", cbCtx.NodeID),
			loggateway.Err(err),
		)
	})

	cb.RegisterAfterNode(func(ctx context.Context, cbCtx *trpcgraph.NodeCallbackContext, state trpcgraph.State, _ any, nodeErr error) (any, error) {
		if cbCtx == nil || nodeErr == nil {
			return nil, nil
		}
		exec := biz.NewGraphExecution(ctx, execID, graphID, sessionID, "")
		exec.SpiritSessionID = spiritSessionID
		action, replanErr := replanner.OnNodeFailure(ctx, exec, cbCtx.NodeID, nodeErr)
		if replanErr != nil {
			// fail-closed：replanner 异常（含预算耗尽）→ 原始错误传播。
			lg.Warn("runtime replanner: OnNodeFailure failed, node stays failed",
				loggateway.StepID("replanner.callback_fail"),
				loggateway.Str("execution_id", execID),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("graph_id", graphID),
				loggateway.Str("failed_node", cbCtx.NodeID),
				loggateway.Err(replanErr),
			)
			return nil, nil
		}
		out, applyErr := applyReplanControl(ctx, state, cbCtx.NodeID, nodeErr, action, agentNodeRetry)
		if applyErr != nil {
			lg.Info("runtime replanner: replan control applied with error",
				loggateway.StepID("replanner.control_error"),
				loggateway.Str("execution_id", execID),
				loggateway.Str("failed_node", cbCtx.NodeID),
				loggateway.Err(applyErr),
			)
		}
		return out, applyErr
	})

	return cb
}

// applyReplanControl 把 ReplanAction 映射为 AfterNode 行为（ADR-F D3）。
//
// retryExec 为 nil 时 retry 退化为 fail-closed（非 agent 节点无 prompt 语义，
// 同参重跑无 Reflexion 收益）。返回契约与框架 AfterNodeCallback 对齐：
// (result, nil) 恢复节点并继续路由；(nil, err) 以 err 终结节点；(nil, nil)
// 传播原始 nodeErr。
func applyReplanControl(
	ctx context.Context,
	state trpcgraph.State,
	nodeID string,
	nodeErr error,
	action *graph.ReplanAction,
	retryExec replanRetryFunc,
) (any, error) {
	if action == nil {
		return nil, nil
	}
	cmd := graph.NewControlCommand(action, nodeID, nodeErr)
	switch action.Type {
	case graph.ReplanRetry:
		if state != nil {
			state[graph.StateKeyControlCommand] = cmd
		}
		if retryExec == nil {
			// 无执行载体：fail-closed（C-23 语义保持）。
			return nil, nodeErr
		}
		feedback := buildReflexionFeedback(nodeErr)
		out, rerr := retryExec(ctx, state, nodeID, feedback)
		if rerr != nil {
			if em := event.TraceEmitterFromContext(ctx); em != nil {
				em.LogWarn("graph.replan.applied", "智能重试失败",
					fmt.Sprintf("节点 %s 携带失败反馈重试仍未成功", nodeID),
					event.P("action", "retry"),
					event.P("node_id", nodeID),
					event.P("error", rerr.Error()))
			}
			return nil, rerr
		}
		if em := event.TraceEmitterFromContext(ctx); em != nil {
			em.LogDone("graph.replan.applied",
				fmt.Sprintf("智能重试成功：节点 %s 携带失败反馈重试后恢复", nodeID),
				event.P("action", "retry"),
				event.P("node_id", nodeID),
				event.P("cause", cmd.Cause))
		}
		return out, nil
	case graph.ReplanInsertFallback:
		if state != nil {
			state[graph.StateKeyControlCommand] = cmd
		}
		// 未声明 fallback 的 agent_incapable：无运行时换脑通道 → HITL 暂停
		// （graph 域 resume API / team 域 waiting_human 链消费）。
		intr := trpcgraph.NewInterruptError(map[string]any{
			"control":        "insert_fallback",
			"fallback_agent": cmd.FallbackAgent,
			"node_id":        nodeID,
			"cause":          cmd.Cause,
		})
		intr.NodeID = nodeID
		intr.TaskID = nodeID
		intr.Key = graph.StateKeyControlCommand
		return nil, intr
	case graph.ReplanReroute:
		// 框架不支持运行时拓扑改写 → 退化为 skip（与静态 on_failure=skip
		// 同语义：标记 SkippedNodesStateKey + 屏障照常推进下游）。
		if em := event.TraceEmitterFromContext(ctx); em != nil {
			em.LogWarn("graph.replan.applied", "路由受阻降级为跳过",
				fmt.Sprintf("节点 %s 路由受阻，按跳过处理继续下游", nodeID),
				event.P("action", "reroute_skip"),
				event.P("node_id", nodeID),
				event.P("cause", cmd.Cause))
		}
		return replanSkipNodeUpdate(state, nodeID), nil
	default:
		// rebuild_subgraph / 未知动作：fail-closed（子图重建出批次范围）。
		if state != nil {
			state[graph.StateKeyControlCommand] = cmd
		}
		return nil, nil
	}
}

// agentNodeRetry 是生产 retryExec：FindSubAgent 命中判定 agent 节点（未命中
// = function/tool 等无 prompt 语义节点，返回错误触发 fail-closed），命中则
// Reflexion 副本重执行。
func agentNodeRetry(ctx context.Context, state trpcgraph.State, nodeID, feedback string) (any, error) {
	parent, _ := state[trpcgraph.StateKeyParentAgent].(trpcagent.Agent)
	resolver, ok := parent.(interface{ FindSubAgent(string) trpcagent.Agent })
	if !ok || resolver == nil || resolver.FindSubAgent(nodeID) == nil {
		return nil, fmt.Errorf("replan retry: node %q is not a resolvable agent node", nodeID)
	}
	// Reflexion 副本：浅拷一层后重写 user_input——callback 拿到的 stateCopy
	// 会经 syncResumeState 进 checkpoint，直接改写会污染恢复点。
	retryState := make(trpcgraph.State, len(state)+1)
	for k, v := range state {
		retryState[k] = v
	}
	if cur, _ := retryState[trpcgraph.StateKeyUserInput].(string); cur != "" {
		retryState[trpcgraph.StateKeyUserInput] = feedback + cur
	}
	return trpcgraph.NewAgentNodeFunc(nodeID)(ctx, retryState)
}

// buildReflexionFeedback 把节点失败原因格式化为注入重试 prompt 的反馈文本
// （Reflexion：失败上下文写回，避免同参重试同坑复踩）。截断防 prompt 膨胀。
func buildReflexionFeedback(err error) string {
	msg := err.Error()
	const maxCauseLen = 500
	if len(msg) > maxCauseLen {
		msg = msg[:maxCauseLen] + "..."
	}
	return fmt.Sprintf("[重试反馈] 上一次尝试失败：%s。请分析失败原因并调整方法后重试。\n\n", msg)
}

// replanSkipNodeUpdate 复刻静态 skip 语义（internal/graph/trpc skip_node.go
// 未导出 appendSkippedNode，此处等价实现）：标记 SkippedNodesStateKey 并返回
// skip 输出（AfterNode overridden → 节点恢复，下游屏障照常推进）。
func replanSkipNodeUpdate(state trpcgraph.State, nodeID string) map[string]any {
	if state == nil {
		state = trpcgraph.State{}
	}
	skipped := make([]string, 0, 4)
	switch v := state[biz.SkippedNodesStateKey].(type) {
	case []string:
		skipped = append(skipped, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				skipped = append(skipped, s)
			}
		}
	}
	dup := false
	for _, existing := range skipped {
		if existing == nodeID {
			dup = true
			break
		}
	}
	if !dup {
		skipped = append(skipped, nodeID)
	}
	state[biz.SkippedNodesStateKey] = skipped
	return map[string]any{biz.SkippedNodeOutputKey: nodeID}
}
