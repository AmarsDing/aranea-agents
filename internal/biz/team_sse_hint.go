package biz

import (
	"context"
	"strings"
)

// HintTeamRunSSE lists the latest runs for a team and emits run_finished best-effort
// after team chat completions (cron or unary ingress). Brokers and repos may be nil.
func HintTeamRunSSE(ctx context.Context, broker *TeamRunEventBroker, teams TeamRepository, teamID string) {
	if broker == nil || teams == nil || strings.TrimSpace(teamID) == "" {
		return
	}
	runs, err := teams.ListTeamRuns(ctx, teamID, 5)
	if err != nil || len(runs) == 0 {
		return
	}
	cp := runs[0]
	broker.Publish(TeamRunEvent{
		Type:      "run_finished",
		TeamID:    teamID,
		RunID:     cp.ID,
		SessionID: strings.TrimSpace(cp.SessionID),
		Run:       &cp,
	})
}
