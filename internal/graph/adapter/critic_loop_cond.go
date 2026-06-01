package adapter

import (
	"context"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const DefaultCriticLoopThreshold = 0.0

func criticLoopCondFunc(threshold float64, lg loggateway.Logger) trpcgraph.ConditionalFunc {
	return func(ctx context.Context, state trpcgraph.State) (string, error) {
		msgs, ok := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
		if !ok || len(msgs) == 0 {
			return "retry", nil
		}
		lastMsg := msgs[len(msgs)-1]
		for _, tc := range lastMsg.ToolCalls {
			if tc.Function.Name == biz.OrchestrationControlToolName {
				d, err := biz.ParseOrchestrationDecision(tc.Function.Arguments, lg)
				if err == nil {
					if biz.IsApprovedDecision(d, threshold) {
						return "approved", nil
					}
					return "retry", nil
				}
			}
		}
		content := strings.ToLower(lastMsg.Content)
		if containsWord(content, "approved") {
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

func containsWord(s, word string) bool {
	for {
		idx := strings.Index(s, word)
		if idx < 0 {
			return false
		}
		beforeOk := idx == 0 || !isAlphaNum(rune(s[idx-1]))
		afterIdx := idx + len(word)
		afterOk := afterIdx >= len(s) || !isAlphaNum(rune(s[afterIdx]))
		if beforeOk && afterOk {
			return true
		}
		s = s[afterIdx:]
	}
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func RegisterCriticLoopCondFunc(reg RegistryRegistrar, threshold float64, lg loggateway.Logger) {
	reg.RegisterCondFuncInstance(biz.CriticLoopCondFuncRef, criticLoopCondFunc(threshold, lg))
}

type RegistryRegistrar interface {
	RegisterCondFuncInstance(name string, fn any)
}
