package agent

import (
	"context"

	"aranea-agents/internal/biz"
)

// TeamAgentHelperAdapter implements biz.TeamAgentHelper by delegating to
// the agent package's utility functions. Inject into team.Runner via
// SetAgentHelper to eliminate the team→agent direct import for helpers.
type TeamAgentHelperAdapter struct{}

var _ biz.TeamAgentHelper = (*TeamAgentHelperAdapter)(nil)

func (h *TeamAgentHelperAdapter) RFC3339Now() string {
	return RFC3339Now()
}

func (h *TeamAgentHelperAdapter) UserIDFromCtx(ctx context.Context) string {
	return UserIDFromCtx(ctx)
}

func (h *TeamAgentHelperAdapter) UserOptionsJSON(agent biz.Agent, dialogMode, provider, model string, contextRatio float64, anchor *biz.TeamMemberAnchorRef) (string, error) {
	a := agentAnchorFromRef(anchor)
	return UserOptionsJSON(agent, dialogMode, provider, model, contextRatio, a)
}

func (h *TeamAgentHelperAdapter) AssistantOptionsJSON(agent biz.Agent, anchor *biz.TeamMemberAnchorRef) (string, error) {
	a := agentAnchorFromRef(anchor)
	return AssistantOptionsJSON(agent, a)
}

func (h *TeamAgentHelperAdapter) MergeReasoningIntoAssistantOptionsJSON(optsJSON, reasoning string) (string, error) {
	return MergeReasoningIntoAssistantOptionsJSON(optsJSON, reasoning)
}

func (h *TeamAgentHelperAdapter) DisplayMarkdownFromStream(result biz.TeamStreamResult) string {
	esr := EventStreamResult{
		HasError:   result.HasError,
		LastError:  result.LastError,
		HasContent: result.HasContent,
	}
	return DisplayMarkdownFromStream(esr)
}

func (h *TeamAgentHelperAdapter) EstimateTokensIfMissing(promptTok, completionTok int, input, output string) (int, int) {
	return EstimateTokensIfMissing(promptTok, completionTok, input, output)
}

func (h *TeamAgentHelperAdapter) ResolveRalphLoopTurn(settingsJSON string) biz.RalphLoopResult {
	// ResolveRalphLoopTurn takes *biz.AgentRuntimeSettings, not a JSON string.
	// The team runner passes the Agent model which has Settings field.
	// This adapter method is for future use when settings are passed as JSON.
	return biz.RalphLoopResult{}
}

func agentAnchorFromRef(ref *biz.TeamMemberAnchorRef) *TeamMemberAnchor {
	if ref == nil {
		return nil
	}
	return &TeamMemberAnchor{
		AgentID: ref.AgentID,
		Name:    ref.Name,
		Role:    ref.Role,
	}
}
