package adapter

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func criticLoopCondFunc(threshold float64) trpcgraph.ConditionalFunc {
	return func(ctx context.Context, state trpcgraph.State) (string, error) {
		msgs, ok := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
		if !ok || len(msgs) == 0 {
			return "retry", nil
		}
		lastMsg := msgs[len(msgs)-1]
		for _, tc := range lastMsg.ToolCalls {
			if tc.Function.Name == biz.OrchestrationControlToolName {
				d, err := biz.ParseOrchestrationDecision(tc.Function.Arguments)
				if err == nil {
					if biz.IsApprovedDecision(d, threshold) {
						return "approved", nil
					}
					return "retry", nil
				}
			}
		}
		content := strings.ToLower(lastMsg.Content)
		if strings.Contains(content, "approved") {
			return "approved", nil
		}
		if threshold > 0 {
			score := biz.ExtractScore(content)
			if score > 0 && score >= threshold {
				return "approved", nil
			}
		}
		return "retry", nil
	}
}

func RegisterCriticLoopCondFunc(reg RegistryRegistrar, threshold float64) {
	reg.RegisterCondFuncInstance(biz.CriticLoopCondFuncRef, criticLoopCondFunc(threshold))
}

type RegistryRegistrar interface {
	RegisterCondFuncInstance(name string, fn any)
}
