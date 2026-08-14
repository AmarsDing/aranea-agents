package biz

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

// SafetyLimitGracefulHeadroom is the number of extra LLM calls required beyond
// MaxToolIterations so a turn can still end with a graceful final summary:
//   - +1: the tool-less wrap-up request after the budget is exhausted
//     (framework omits tool declarations once ToolIterationBudgetExhausted);
//   - +1: one more tool-less request if the model still emits tool calls on the
//     wrap-up turn (framework rejects them with synthesized tool results and
//     loops once more before the final text answer).
//
// Without this headroom MaxLLMCalls hard-stops the turn with a StopError before
// the summary is produced.
const SafetyLimitGracefulHeadroom = 2

// ValidateSafetyLimitCoupling rejects MaxLLMCalls / MaxToolIterations
// combinations at the persistence boundary that would hard-stop the turn
// without a final summary. When either limit is unset (<= 0), validation is a
// no-op.
func ValidateSafetyLimitCoupling(s *AgentRuntimeSettings) error {
	if s == nil || s.MaxLLMCalls <= 0 || s.MaxToolIterations <= 0 {
		return nil
	}
	if s.MaxLLMCalls < s.MaxToolIterations+SafetyLimitGracefulHeadroom {
		return apierror.BadRequest("AGENT", fmt.Sprintf(
			"max_llm_calls (%d) must be >= max_tool_iterations (%d) + %d to leave headroom for the graceful final summary",
			s.MaxLLMCalls, s.MaxToolIterations, SafetyLimitGracefulHeadroom))
	}
	return nil
}

// CoupledSafetyLimits returns the effective (maxLLMCalls, maxToolIterations)
// for agent construction, defensively raising maxLLMCalls to satisfy the
// coupling invariant for legacy rows persisted before ValidateSafetyLimitCoupling.
// elevated reports whether such a raise was applied.
func CoupledSafetyLimits(s *AgentRuntimeSettings) (maxLLMCalls, maxToolIterations int, elevated bool) {
	if s == nil {
		return 0, 0, false
	}
	maxLLMCalls, maxToolIterations = s.MaxLLMCalls, s.MaxToolIterations
	if maxLLMCalls > 0 && maxToolIterations > 0 &&
		maxLLMCalls < maxToolIterations+SafetyLimitGracefulHeadroom {
		maxLLMCalls = maxToolIterations + SafetyLimitGracefulHeadroom
		elevated = true
	}
	return maxLLMCalls, maxToolIterations, elevated
}
