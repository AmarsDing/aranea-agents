package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (r *agentRepo) ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]biz.AgentListExtras, error) {
	out := make(map[string]biz.AgentListExtras, len(agentIDs))
	ids := make([]string, 0, len(agentIDs))
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = biz.AgentListExtras{LastRunStatus: "idle"}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}

	// Latest session per agent via a window function: the previous
	// implementation loaded EVERY session row of the page's agents into memory
	// and sorted client-side — a list-page performance bomb for agents with
	// long session histories. ROW_NUMBER is supported by both SQLite (>=3.25)
	// and Postgres, matching the existing dual-dialect raw-SQL pattern below.
	placeholders := r.data.Dialect().Placeholders(len(ids))
	latestQ := fmt.Sprintf(`SELECT agent_id, last_run_at, updated_at, state_json FROM (
		SELECT agent_id, last_run_at, updated_at, state_json,
			ROW_NUMBER() OVER (PARTITION BY agent_id ORDER BY last_run_at DESC, updated_at DESC) AS rn
		FROM sessions
		WHERE deleted_at = '' AND agent_id IN (%s)
	) latest WHERE rn = 1`, placeholders)
	latestArgs := make([]any, len(ids))
	for i, id := range ids {
		latestArgs[i] = id
	}
	latestRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, latestQ, latestArgs...)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	for latestRows.Next() {
		var agentID, lastRunAt, updatedAt, stateJSON string
		if err := latestRows.Scan(&agentID, &lastRunAt, &updatedAt, &stateJSON); err != nil {
			latestRows.Close()
			return nil, entErrToBizErr(err, "AGENT")
		}
		id := strings.TrimSpace(agentID)
		if id == "" {
			continue
		}
		ex := out[id]
		ex.LastRunAt = strings.TrimSpace(lastRunAt)
		if ex.LastRunAt == "" {
			ex.LastRunAt = strings.TrimSpace(updatedAt)
		}
		if st := runStatusFromStateJSON(r.data.lg, stateJSON); st != "" {
			ex.LastRunStatus = st
		} else if ex.LastRunAt != "" {
			ex.LastRunStatus = "idle"
		}
		out[id] = ex
	}
	if err := latestRows.Err(); err != nil {
		latestRows.Close()
		return nil, entErrToBizErr(err, "AGENT")
	}
	latestRows.Close()

	// A6: pending evolution count reads the unified store (agent-scoped rows
	// cover both L1 skill-creation proposals and L3 agent suggestions).
	// target_type is inlined (internal constant); Placeholders is dialect-aware,
	// so the query must not go through RenumberPlaceholders.
	pendingQ := fmt.Sprintf(`SELECT target_id, COUNT(*) FROM unified_evolution_suggestions
		WHERE target_type = 'agent' AND status = 'pending' AND target_id IN (%s)
		GROUP BY target_id`, placeholders)
	pendingArgs := make([]any, len(ids))
	for i, id := range ids {
		pendingArgs[i] = id
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, pendingQ, pendingArgs...)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	defer rows.Close()
	for rows.Next() {
		var targetID string
		var count int
		if err := rows.Scan(&targetID, &count); err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		id := strings.TrimSpace(targetID)
		if id == "" {
			continue
		}
		ex := out[id]
		ex.PendingEvolutionCount += count
		out[id] = ex
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	return out, nil
}

func runStatusFromStateJSON(lg loggateway.Logger, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ""
	}
	state := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		lg.Warn("unmarshal agent state_json failed", loggateway.StepID("data.agent_list_extras"), loggateway.Err(err))
		return ""
	}
	v, ok := state[biz.SessionStateRunStatus]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
