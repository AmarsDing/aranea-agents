package team

import (
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
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

// TeamSummaryActivityEvent emits a structured team_summary as a biz.ActivityEvent
// for WS team/monitor consumers. Replaces the legacy TeamSummaryEnvelope helper
// during the dual-bus unification (Teams domain → ActivityEventBus).
func TeamSummaryActivityEvent(run biz.TeamRun, steps []biz.TeamRunStep) biz.ActivityEvent {
	summary := BuildTeamRunSummary(run, steps)
	// SessionID = spirit session ID (not run.SessionID which is the team
	// session ID) so the frontend WS filter and listActivities API return
	// this team_stage summary event. Matches publishSpiritTeamAssembled.
	return biz.ActivityEvent{
		Event: biz.ActivityEventCompleted,
		Activity: biz.Activity{
			ID:              agent.TeamStageActivityID(run.TeamID),
			Kind:            biz.ActivityKindTeamStage,
			Status:          biz.ActivityStatusCompleted,
			SessionID:       run.SpiritSessionID,
			SpiritSessionID: run.SpiritSessionID,
			TeamID:          run.TeamID,
			Timestamp:        time.Now().UTC(),
			Stage:           "completed",
			Meta: map[string]any{
				"run_id":       run.ID,
				"run":          run,
				"team_summary": summary,
			},
		},
		Domain: biz.ActivityDomainChat,
	}
}
