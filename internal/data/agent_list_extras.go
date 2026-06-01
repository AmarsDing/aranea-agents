package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/evolutionsuggestion"
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

	sessions, err := r.data.entClient.Session.Query().
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

	pending, err := r.data.entClient.EvolutionSuggestion.Query().
		Where(
			evolutionsuggestion.AgentIDIn(ids...),
			evolutionsuggestion.StatusEQ("pending"),
		).
		Select(evolutionsuggestion.FieldAgentID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range pending {
		id := strings.TrimSpace(row.AgentID)
		if id == "" {
			continue
		}
		ex := out[id]
		ex.PendingEvolutionCount++
		out[id] = ex
	}
	return out, nil
}

func runStatusFromStateJSON(lg loggateway.Logger, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return ""
	}
	state := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		lg.Warn("unmarshal agent state_json failed", loggateway.StepID("data.agent_list_extras"), loggateway.Err(err))
		return ""
	}
	return strings.TrimSpace(state[biz.SessionStateRunStatus])
}
