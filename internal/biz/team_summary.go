package biz

import "strings"

// BuildTeamRunSummaryData aggregates run-level and per-member stats from a run and its steps.
func BuildTeamRunSummaryData(run TeamRun, steps []TeamRunStep) TeamRunSummaryData {
	members := make([]TeamRunMemberSummaryData, 0, len(steps))
	totalToolCalls := 0
	for _, s := range steps {
		totalToolCalls += s.ToolCallCount
		members = append(members, TeamRunMemberSummaryData{
			AgentID:       s.AgentID,
			AgentKey:      s.AgentKey,
			AgentName:     s.AgentName,
			Role:          s.Role,
			SortOrder:     s.SortOrder,
			Status:        s.Status,
			TokenIn:       s.TokenIn,
			TokenOut:      s.TokenOut,
			DurationMS:    s.DurationMS,
			CostMicroUSD:  s.CostMicroUSD,
			ToolCallCount: s.ToolCallCount,
			OutputPreview: previewRunSummaryText(s.OutputPreview, 256),
		})
	}
	return TeamRunSummaryData{
		RunID:         run.ID,
		TeamID:        run.TeamID,
		SessionID:     run.SessionID,
		Mode:          run.Mode,
		Status:        run.Status,
		DurationMS:    run.DurationMS,
		TokenIn:       run.TokenIn,
		TokenOut:      run.TokenOut,
		CostMicroUSD:  run.CostMicroUSD,
		MemberCount:   len(members),
		ToolCallCount: totalToolCalls,
		OutputPreview: previewRunSummaryText(run.OutputPreview, 512),
		ErrorMessage:  strings.TrimSpace(run.ErrorMessage),
		Members:       members,
	}
}

func previewRunSummaryText(s string, max int) string {
	return strings.TrimSpace(truncateRunSummaryRunes(strings.TrimSpace(s), max))
}

func truncateRunSummaryRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
