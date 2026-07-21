package data

import (
	"context"
	"strings"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/data/ent/toolgrant"
	"aranea-agents/pkg/apierror"
)

type toolGrantRepo struct {
	data *Data
}

var (
	_ biztool.ToolGrantReader = (*toolGrantRepo)(nil)
	_ biztool.ToolGrantWriter = (*toolGrantRepo)(nil)
)

// NewToolGrantRepo implements biztool.ToolGrantReader/Writer.
func NewToolGrantRepo(d *Data) *toolGrantRepo {
	return &toolGrantRepo{data: d}
}

func (r *toolGrantRepo) HasToolGrant(ctx context.Context, agentID, toolKey string) (bool, error) {
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return false, nil
	}
	client := r.data.RW().Read(ctx)
	if client == nil {
		return false, apierror.Internal("TOOL", "ent client unavailable")
	}
	n, err := client.ToolGrant.Query().
		Where(toolgrant.AgentIDEQ(agentID), toolgrant.ToolKeyEQ(toolKey)).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, entErrToBizErr(err, "TOOL")
	}
	return n > 0, nil
}

func (r *toolGrantRepo) ListToolGrants(ctx context.Context, agentID string) ([]biztool.ToolGrant, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	client := r.data.RW().Read(ctx)
	if client == nil {
		return nil, apierror.Internal("TOOL", "ent client unavailable")
	}
	rows, err := client.ToolGrant.Query().
		Where(toolgrant.AgentIDEQ(agentID)).
		Order(toolgrant.ByToolKey()).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	out := make([]biztool.ToolGrant, 0, len(rows))
	for _, row := range rows {
		out = append(out, biztool.ToolGrant{
			ID:        row.ID,
			AgentID:   row.AgentID,
			ToolKey:   row.ToolKey,
			GrantedBy: row.GrantedBy,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func (r *toolGrantRepo) CreateToolGrant(ctx context.Context, grant biztool.ToolGrant) error {
	agentID := strings.TrimSpace(grant.AgentID)
	toolKey := strings.TrimSpace(grant.ToolKey)
	if agentID == "" {
		return apierror.BadRequest("TOOL", "agent_id is required")
	}
	if toolKey == "" {
		return apierror.BadRequest("TOOL", "tool_key is required")
	}
	client := r.data.RW().Write(ctx)
	if client == nil {
		return apierror.Internal("TOOL", "ent client unavailable")
	}
	id := strings.TrimSpace(grant.ID)
	if id == "" {
		id = uniqueToolID("grant")
	}
	// Idempotent create: a conflicting (agent_id, tool_key) row means the
	// grant already exists, which is the desired end state.
	err := client.ToolGrant.Create().
		SetID(id).
		SetAgentID(agentID).
		SetToolKey(toolKey).
		SetGrantedBy(strings.TrimSpace(grant.GrantedBy)).
		SetCreatedAt(nowRFC3339()).
		OnConflictColumns(toolgrant.FieldAgentID, toolgrant.FieldToolKey).
		Ignore().
		Exec(ctx)
	return entErrToBizErr(err, "TOOL")
}

func (r *toolGrantRepo) DeleteToolGrant(ctx context.Context, agentID, toolKey string) error {
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return nil
	}
	client := r.data.RW().Write(ctx)
	if client == nil {
		return apierror.Internal("TOOL", "ent client unavailable")
	}
	// Idempotent delete: removing a non-existent grant is a no-op.
	_, err := client.ToolGrant.Delete().
		Where(toolgrant.AgentIDEQ(agentID), toolgrant.ToolKeyEQ(toolKey)).
		Exec(ctx)
	return entErrToBizErr(err, "TOOL")
}
