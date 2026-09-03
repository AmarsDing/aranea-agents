package graph

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func failureRecoveryOptions(n NodeDef, resolvedFallback trpcagent.Agent) []trpcgraph.Option {
	if strings.EqualFold(strings.TrimSpace(n.FuncRef), biz.SkipNodeFuncRef) {
		return nil
	}
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	if action != biz.FailureOnFailureSkip && fallback == "" {
		return nil
	}
	return []trpcgraph.Option{trpcgraph.WithPostNodeCallback(failureRecoveryAfterNode(n, resolvedFallback))}
}

func failureRecoveryAfterNode(n NodeDef, resolvedFallback trpcagent.Agent) trpcgraph.AfterNodeCallback {
	nodeID := n.ID
	action := strings.ToLower(strings.TrimSpace(n.FailureAction))
	fallback := strings.TrimSpace(n.FallbackAgent)
	primary := strings.TrimSpace(n.AgentName)
	return func(ctx context.Context, _ *trpcgraph.NodeCallbackContext, state trpcgraph.State, _ any, nodeErr error) (any, error) {
		if nodeErr == nil {
			return nil, nil
		}
		if fallback != "" {
			var fn trpcgraph.NodeFunc
			if resolvedFallback != nil {
				fn = resolvedAgentNodeFunc(resolvedFallback)
			} else {
				fn = trpcgraph.NewAgentNodeFunc(fallback)
			}
			out, err := fn(ctx, state)
			if err == nil {
				if st, ok := out.(trpcgraph.State); ok && primary != "" {
					st["_fallback_from_"+nodeID] = primary
					st["_fallback_agent_"+nodeID] = fallback
					return st, nil
				}
				return out, nil
			}
			if action == biz.FailureOnFailureSkip {
				return skipNodeUpdate(state, nodeID, nodeErr), nil
			}
			return nil, err
		}
		if action == biz.FailureOnFailureSkip {
			return skipNodeUpdate(state, nodeID, nodeErr), nil
		}
		return nil, nil
	}
}

func resolvedAgentNodeFunc(ag trpcagent.Agent) trpcgraph.NodeFunc {
	return func(ctx context.Context, state trpcgraph.State) (any, error) {
		parentAgent, _ := state[trpcgraph.StateKeyParentAgent]
		wrapper := &fallbackAgentWrapper{Agent: ag, parent: parentAgent}
		patchedState := make(trpcgraph.State, len(state)+1)
		for k, v := range state {
			patchedState[k] = v
		}
		patchedState[trpcgraph.StateKeyParentAgent] = wrapper
		return trpcgraph.NewAgentNodeFunc(ag.Info().Name)(ctx, patchedState)
	}
}

type fallbackAgentWrapper struct {
	trpcagent.Agent
	parent any
}

func (w *fallbackAgentWrapper) FindSubAgent(name string) trpcagent.Agent {
	if w.Agent != nil && w.Agent.Info().Name == name {
		return w.Agent
	}
	type subAgentProvider interface {
		FindSubAgent(name string) trpcagent.Agent
	}
	if p, ok := w.parent.(subAgentProvider); ok {
		return p.FindSubAgent(name)
	}
	return nil
}

func skipNodeUpdate(state trpcgraph.State, nodeID string, nodeErr error) map[string]any {
	if state == nil {
		state = trpcgraph.State{}
	}
	skipped := appendSkippedNode(state[biz.SkippedNodesStateKey], nodeID)
	state[biz.SkippedNodesStateKey] = skipped
	// P1-2（2026-09-03 名册×结果矩阵 mechanism 层）：向 messages 注入成员失败
	// 通告——下游合成者的 LLM 上下文由 messages 构建，通告使其对成员缺席
	// 可见（此前 _skipped_nodes 只有记录无消费，合成者不知道谁失败了）。
	// 与任务书名册（P1-1）构成双保险；17x 错误放大防线：如实标注优于编造。
	appendSkipNoticeMessage(state, nodeID, nodeErr)
	return map[string]any{biz.SkippedNodeOutputKey: nodeID}
}

// appendSkipNoticeMessage 把成员跳过通告追加到 state messages。通告以
// assistant 角色呈现（对下游节点 LLM 上下文可见）；错误文本截断防超长。
func appendSkipNoticeMessage(state trpcgraph.State, nodeID string, nodeErr error) {
	errText := "原因未知"
	if nodeErr != nil {
		errText = truncateSkipNoticeErr(nodeErr.Error())
	}
	notice := trpcmodel.Message{
		Role: trpcmodel.RoleAssistant,
		Content: "[团队通告] 成员节点 " + nodeID + " 执行失败已被跳过（" + errText +
			"）。其任务部分无产出；聚合时请如实标注该部分缺失，不得编造内容填补。",
	}
	switch existing := state[trpcgraph.StateKeyMessages].(type) {
	case []trpcmodel.Message:
		state[trpcgraph.StateKeyMessages] = append(existing, notice)
	default:
		state[trpcgraph.StateKeyMessages] = []trpcmodel.Message{notice}
	}
}

const skipNoticeErrMaxLen = 200

func truncateSkipNoticeErr(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > skipNoticeErrMaxLen {
		return string(r[:skipNoticeErrMaxLen]) + "…"
	}
	return string(r)
}
