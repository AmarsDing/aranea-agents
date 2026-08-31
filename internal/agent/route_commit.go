package agent

import "aranea-agents/internal/biz"

// CommitRoute is the single writer of a turn's RouteDecision from the
// pre-planning gate outcome. Plan() / hard-route consume this; they must not
// re-decide the lane (FIT-ROUTE-1). Unforced turns stay Unspecified so the
// LLM may still self-route (S01 direct path is unchanged).
func CommitRoute(level biz.ComplexityLevel, forcePlanning bool, score float64, reason, userMessage string) biz.RouteDecision {
	d := biz.RouteDecision{
		Level:         level,
		Score:         score,
		Reason:        reason,
		ForcePlanning: forcePlanning,
		TeamEvidence:  hasTeamModeEvidence(userMessage),
	}
	if !forcePlanning {
		return d
	}
	if d.TeamEvidence {
		d.Lane = biz.RouteLanePlanTeam
		d.Mode = detectTeamIntent(userMessage)
		if d.Mode == "" {
			d.Mode = "dag"
		}
		return d
	}
	// Complex (or other ForcePlanning without team evidence) → solo plan,
	// not a silent Direct collapse and not a false team.
	d.Lane = biz.RouteLanePlanSolo
	return d
}
