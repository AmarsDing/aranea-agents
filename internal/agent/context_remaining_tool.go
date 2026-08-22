package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const contextRemainingToolName = "get_context_remaining"

type contextRemainingOutput struct {
	EstTotalInput int            `json:"est_total_input"`
	WindowTokens  int            `json:"window_tokens"`
	Remaining     int            `json:"remaining"`
	OccupancyPct  float64        `json:"occupancy_pct"`
	EstTokens     map[string]int `json:"est_tokens,omitempty"`
	ToolsCount    int            `json:"tools_count"`
	Note          string         `json:"note,omitempty"`
}

func newContextRemainingTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		getContextRemaining,
		trpcfunction.WithName(contextRemainingToolName),
		trpcfunction.WithDescription("Report how much of the current chat context window is used and remaining. Call this when you are unsure whether the turn can absorb another large read or tool result. Returns estimated input tokens, the product window, remaining tokens, and per-category estimates."),
	)
}

func getContextRemaining(ctx context.Context, _ struct{}) (contextRemainingOutput, error) {
	window := llmcontext.WindowFromContext(ctx)
	out := contextRemainingOutput{WindowTokens: window}
	b := ContextBudgetFromContext(ctx)
	if b == nil {
		out.Remaining = window
		out.Note = "no context budget ledger on this turn; remaining is the full window estimate"
		return out, nil
	}
	snap := b.Snapshot()
	out.EstTotalInput = snap.EstTotalInput
	out.EstTokens = snap.EstTokens
	out.ToolsCount = snap.ToolsCount
	rem := window - snap.EstTotalInput
	if rem < 0 {
		rem = 0
	}
	out.Remaining = rem
	if window > 0 {
		out.OccupancyPct = float64(snap.EstTotalInput) / float64(window) * 100
	}
	return out, nil
}

func shouldAttachContextRemaining(ag biz.Agent, eff map[string]bool) bool {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return false
	}
	prof := strings.ToLower(strings.TrimSpace(ag.Settings.ToolsProfile))
	if prof == "coding" || prof == "spirit" || prof == "full" {
		return true
	}
	if eff == nil {
		return false
	}
	if !(eff["shell_exec"] || eff["exec_command"] || eff["diff_edit"] || eff["save_file"]) {
		return false
	}
	return ShouldAttachWorkingContract(ag)
}
