package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
)

// SafetyLimitAdapter converts the project's agent safety settings into
// framework llmagent.Option functions that enable framework safety limits.
// When MaxLLMCalls or MaxToolIterations are configured (> 0), the framework
// enforces per-turn limits to prevent runaway agent behavior.
// Legacy rows that violate the coupling invariant (see biz.ValidateSafetyLimitCoupling)
// are defensively elevated so the turn can still end with a graceful summary.
func SafetyLimitAdapter(ag biz.Agent, lg loggateway.Logger) []llmagent.Option {
	if ag.Settings == nil {
		return nil
	}
	maxLLMCalls, maxToolIterations, elevated := biz.CoupledSafetyLimits(ag.Settings)
	if elevated {
		lg.Warn("max_llm_calls 低于优雅收尾所需余量，已防御性抬升",
			loggateway.StepID("agent.safety_limit_elevated"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Int("max_llm_calls", maxLLMCalls),
			loggateway.Int("max_tool_iterations", maxToolIterations))
	}
	var opts []llmagent.Option
	if maxLLMCalls > 0 {
		opts = append(opts, llmagent.WithMaxLLMCalls(maxLLMCalls))
	}
	if maxToolIterations > 0 {
		opts = append(opts, llmagent.WithMaxToolIterations(maxToolIterations))
	}
	return opts
}
