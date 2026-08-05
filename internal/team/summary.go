package team

import (
	"aranea-agents/internal/biz"
)

// BuildTeamRunSummary aggregates run-level and per-member stats for Monitor / automation.
func BuildTeamRunSummary(run biz.TeamRunRecord, steps []biz.TeamRunStep) map[string]any {
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
			"session_id":      m.SessionID,
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

// TeamSummaryActivityEvent was removed in S-3（2026-08-05）: no production
// callers remained (runner publishTeamRunSummary emits a SystemNoticeEvent
// instead), and its legacy teamID-only stage ID formula could not be made
// run-isolated without a rootTaskID source.
