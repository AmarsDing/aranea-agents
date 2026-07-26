package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect/sql"
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

	sessions, err := r.data.RW().Read(ctx).Session.Query().
		Where(
			entsession.AgentIDIn(ids...),
			entsession.DeletedAtEQ(""),
		).
		Order(
			entsession.ByLastRunAt(sql.OrderDesc()),
			entsession.ByUpdatedAt(sql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(ids))
	for _, sess := range sessions {
		id := strings.TrimSpace(sess.AgentID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ex := out[id]
		ex.LastRunAt = strings.TrimSpace(sess.LastRunAt)
		if ex.LastRunAt == "" {
			ex.LastRunAt = strings.TrimSpace(sess.UpdatedAt)
		}
		if st := runStatusFromStateJSON(r.data.lg, sess.StateJSON); st != "" {
			ex.LastRunStatus = st
		} else if ex.LastRunAt != "" {
			ex.LastRunStatus = "idle"
		}
		out[id] = ex
	}

	// A6: pending evolution count reads the unified store (agent-scoped rows
	// cover both L1 skill-creation proposals and L3 agent suggestions).
	// target_type is inlined (internal constant); Placeholders is dialect-aware,
	// so the query must not go through RenumberPlaceholders.
	placeholders := r.data.Dialect().Placeholders(len(ids))
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
