package team

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// BuildTeamRunSummary aggregates run-level and per-member stats for Monitor / automation.
func BuildTeamRunSummary(run biz.TeamRun, steps []biz.TeamRunStep) map[string]any {
	return SummaryMapFromData(biz.BuildTeamRunSummaryData(run, steps))
}

// SummaryMapFromData serializes summary data for WS team_summary consumers.
func SummaryMapFromData(data biz.TeamRunSummaryData) map[string]any {
	members := make([]map[string]any, 0, len(data.Members))
	for _, m := range data.Members {
		members = append(members, map[string]any{
			"agent_id":        m.AgentID,
			"agent_key":       m.AgentKey,
			"agent_name":      m.AgentName,
			"role":            m.Role,
			"sort_order":      m.SortOrder,
			"status":          m.Status,
			"token_in":        m.TokenIn,
			"token_out":       m.TokenOut,
			"duration_ms":     m.DurationMS,
			"cost_micro_usd":  m.CostMicroUSD,
			"tool_call_count": m.ToolCallCount,
			"output_preview":  m.OutputPreview,
		})
	}
	return map[string]any{
		"run_id":          data.RunID,
		"team_id":         data.TeamID,
		"session_id":      data.SessionID,
		"mode":            data.Mode,
		"status":          data.Status,
		"duration_ms":     data.DurationMS,
		"token_in":        data.TokenIn,
		"token_out":       data.TokenOut,
		"cost_micro_usd":  data.CostMicroUSD,
		"member_count":    data.MemberCount,
		"tool_call_count": data.ToolCallCount,
		"members":         members,
		"output_preview":  data.OutputPreview,
		"error_message":   data.ErrorMessage,
	}
}

// TeamSummaryEnvelope emits a structured team_summary for WS team/monitor consumers.
func TeamSummaryEnvelope(run biz.TeamRun, steps []biz.TeamRunStep) event.Envelope {
	env := event.NewEnvelope(event.EnvelopeTypeTeamSummary, "team-runner", strings.TrimSpace(run.SessionID))
	env.TeamID = run.TeamID
	summary := BuildTeamRunSummary(run, steps)
	env.Metadata = map[string]any{
		"run_id":       run.ID,
		"run":          run,
		"team_summary": summary,
	}
	return env
}
