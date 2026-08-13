package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// TeamCompletionChecker defines the interface for checking team completion status.
// This interface breaks the circular dependency between agent and biz packages.
type TeamCompletionChecker interface {
	CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) biz.AllTeamsCompletedResult
}

// teamCompletionGuardBeforeHook prevents the Spirit LLM from polling team status
// via get_team_deliverable when teams are still running. This enforces the
// system-push pattern (wait for notification) instead of the LLM-polling pattern.
type teamCompletionGuardBeforeHook struct {
	checker TeamCompletionChecker
	lg      loggateway.Logger
}

func newTeamCompletionGuardBeforeHook(checker TeamCompletionChecker, lg loggateway.Logger) *teamCompletionGuardBeforeHook {
	if checker == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &teamCompletionGuardBeforeHook{
		checker: checker,
		lg:      lg,
	}
}

func (h *teamCompletionGuardBeforeHook) Point() callbacks.CallbackPoint {
	return callbacks.PointBeforeTool
}

// Priority 3 executes before command safety (priority 4), ensuring team completion
// checks run first: a polling attempt should be blocked before any other tool validation.
func (h *teamCompletionGuardBeforeHook) Priority() int { return 3 }

func (h *teamCompletionGuardBeforeHook) HandleBeforeTool(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
	if args == nil || h.checker == nil {
		return &trpctool.BeforeToolResult{}, nil
	}

	// Only guard team-dependent tools. WP-2a: synthesize_results shares the
	// same precondition as get_team_deliverable (all teams finished) — before
	// this guard covered it, 71.7% of its production calls failed with
	// "active teams still running" and burned full-context retries.
	var action string
	switch args.ToolName {
	case "get_team_deliverable":
		action = "查询交付物"
	case "synthesize_results":
		action = "合成结果"
	default:
		return &trpctool.BeforeToolResult{}, nil
	}

	// Extract spirit session ID from context
	spiritSessionID := extractSpiritSessionID(ctx)
	if spiritSessionID == "" {
		return &trpctool.BeforeToolResult{}, nil
	}

	// Check if all teams are completed
	result := h.checker.CheckAllTeamsCompleted(ctx, spiritSessionID)

	// If there are no teams or all teams are done, allow the call
	if result.TotalTeams == 0 || result.AllDone {
		return &trpctool.BeforeToolResult{}, nil
	}

	// Teams are still running - block the call and guide the LLM to wait
	h.lg.Info("blocked team-dependent tool call - teams still running",
		loggateway.StepID("tool.team_completion_guard"),
		loggateway.Str("tool", args.ToolName),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Int("total_teams", result.TotalTeams),
		loggateway.Int("completed_teams", result.CompletedTeams),
		loggateway.Int("failed_teams", result.FailedTeams),
	)

	guidance := fmt.Sprintf("团队仍在执行中（%d/%d 已完成）。请等待系统通知所有团队完成后再%s。系统会在所有团队完成后主动通知您，无需主动轮询或重试。",
		result.CompletedTeams, result.TotalTeams, action)

	return &trpctool.BeforeToolResult{
		CustomResult: guidance,
	}, nil
}

// extractSpiritSessionID extracts the spirit session ID from the context.
// Uses the same logic as the existing spiritSessionIDFromCtx function in tools package.
func extractSpiritSessionID(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}
