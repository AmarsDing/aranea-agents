package team

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// BuildTeamRunSummary aggregates run-level and per-member stats for Monitor / automation.
func BuildTeamRunSummary(run biz.TeamRun, steps []biz.TeamRunStep) map[string]any {
	members := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		members = append(members, map[string]any{
			"agent_id":       s.AgentID,
			"agent_key":      s.AgentKey,
			"agent_name":     s.AgentName,
			"role":           s.Role,
			"sort_order":     s.SortOrder,
			"status":         s.Status,
			"token_in":       s.TokenIn,
			"token_out":      s.TokenOut,
			"duration_ms":    s.DurationMS,
			"cost_micro_usd": s.CostMicroUSD,
			"output_preview": preview(s.OutputPreview, 256),
		})
	}
	return map[string]any{
		"run_id":          run.ID,
		"team_id":         run.TeamID,
		"session_id":      run.SessionID,
		"mode":            run.Mode,
		"status":          run.Status,
		"duration_ms":     run.DurationMS,
		"token_in":        run.TokenIn,
		"token_out":       run.TokenOut,
		"cost_micro_usd":  run.CostMicroUSD,
		"member_count":    len(members),
		"members":         members,
		"output_preview":  preview(run.OutputPreview, 512),
		"error_message":   strings.TrimSpace(run.ErrorMessage),
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
