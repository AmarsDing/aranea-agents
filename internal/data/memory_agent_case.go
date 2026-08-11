package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// memoryAgentCaseRepo implements the Agent Case ports (P3 M2) over the
// memory_agent_cases table via raw SQL. Idempotency anchor: unique index on
// (agent_id, source_session_id) — re-extraction/retries overwrite in place.
type memoryAgentCaseRepo struct {
	data *Data
}

var (
	_ biz.AgentCaseReader = (*memoryAgentCaseRepo)(nil)
	_ biz.AgentCaseWriter = (*memoryAgentCaseRepo)(nil)
)

// NewMemoryAgentCaseStore creates the Agent Case reader/writer backed by
// data. Returns nil when data is nil.
func NewMemoryAgentCaseStore(data *Data) *memoryAgentCaseRepo {
	if data == nil {
		return nil
	}
	return &memoryAgentCaseRepo{data: data}
}

// GetAgentCaseBySession loads the case extracted from a session. Returns
// (nil, nil) when none exists — the worker treats that as "not yet extracted".
func (r *memoryAgentCaseRepo) GetAgentCaseBySession(ctx context.Context, agentID, sessionID string) (*biz.AgentCase, error) {
	if r == nil || r.data == nil {
		return nil, nil
	}
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT id, user_id, goal, approach, outcome, outcome_summary, pitfalls, tools_used, quality
		FROM memory_agent_cases
		WHERE agent_id = ? AND source_session_id = ?`), strings.TrimSpace(agentID), strings.TrimSpace(sessionID))
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_AGENT_CASE")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, entErrToBizErr(rows.Err(), "MEMORY_AGENT_CASE")
	}
	var c biz.AgentCase
	var toolsJSON string
	if err := rows.Scan(&c.ID, &c.UserID, &c.Goal, &c.Approach, &c.Outcome, &c.OutcomeSummary, &c.Pitfalls, &toolsJSON, &c.Quality); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_AGENT_CASE")
	}
	_ = json.Unmarshal([]byte(toolsJSON), &c.ToolsUsed)
	c.AgentID = strings.TrimSpace(agentID)
	c.SourceSessionID = strings.TrimSpace(sessionID)
	return &c, nil
}

// UpsertAgentCase inserts or replaces the case for (agent, session). Empty
// goal or agentID is rejected silently — a case without a goal is noise.
func (r *memoryAgentCaseRepo) UpsertAgentCase(ctx context.Context, c biz.AgentCase) error {
	if r == nil || r.data == nil {
		return nil
	}
	agentID := strings.TrimSpace(c.AgentID)
	sessionID := strings.TrimSpace(c.SourceSessionID)
	if agentID == "" || sessionID == "" || strings.TrimSpace(c.Goal) == "" {
		return nil
	}
	outcome := strings.TrimSpace(c.Outcome)
	switch outcome {
	case biz.AgentCaseOutcomeSuccess, biz.AgentCaseOutcomePartial, biz.AgentCaseOutcomeFailure:
	default:
		outcome = biz.AgentCaseOutcomePartial
	}
	quality := c.Quality
	if quality <= 0 {
		quality = biz.ExtractionQualityHeuristic
	}
	toolsJSON, _ := json.Marshal(c.ToolsUsed)
	id := strings.TrimSpace(c.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		INSERT INTO memory_agent_cases (id, agent_id, user_id, source_session_id, goal, approach, outcome, outcome_summary, pitfalls, tools_used, quality, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, source_session_id) DO UPDATE SET
			goal = excluded.goal,
			approach = excluded.approach,
			outcome = excluded.outcome,
			outcome_summary = excluded.outcome_summary,
			pitfalls = excluded.pitfalls,
			tools_used = excluded.tools_used,
			quality = excluded.quality,
			updated_at = excluded.updated_at`),
		id, agentID, strings.TrimSpace(c.UserID), sessionID,
		strings.TrimSpace(c.Goal), strings.TrimSpace(c.Approach), outcome,
		strings.TrimSpace(c.OutcomeSummary), strings.TrimSpace(c.Pitfalls),
		string(toolsJSON), quality, now, now)
	return entErrToBizErr(err, "MEMORY_AGENT_CASE")
}
