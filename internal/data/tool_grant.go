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

// HasToolGrant reports whether a live grant exists for the pair. Expired rows
// (expires_at non-empty and <= now) count as absent so the confirmation
// prompt re-arms after TTL (BUG-MON-B). RFC3339 UTC fixed-width strings
// compare lexicographically = chronologically (project nowRFC3339 惯例).
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
	now := nowRFC3339()
	n, err := client.ToolGrant.Query().
		Where(
			toolgrant.AgentIDEQ(agentID),
			toolgrant.ToolKeyEQ(toolKey),
			toolgrant.Or(toolgrant.ExpiresAtEQ(""), toolgrant.ExpiresAtGT(now)),
		).
		Limit(1).
		Count(ctx)
	if err != nil {
		return false, entErrToBizErr(err, "TOOL")
	}
	return n > 0, nil
}

// ListToolGrants lists live grants for an agent (expired rows excluded and
// lazily deleted best-effort — 读径惰性删 per BUG-MON-B 方案).
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
	now := nowRFC3339()
	out := make([]biztool.ToolGrant, 0, len(rows))
	expiredIDs := make([]string, 0, 2)
	for _, row := range rows {
		if row.ExpiresAt != "" && row.ExpiresAt <= now {
			expiredIDs = append(expiredIDs, row.ID)
			continue
		}
		out = append(out, biztool.ToolGrant{
			ID:        row.ID,
			AgentID:   row.AgentID,
			ToolKey:   row.ToolKey,
			GrantedBy: row.GrantedBy,
			CreatedAt: row.CreatedAt,
			ExpiresAt: row.ExpiresAt,
		})
	}
	if len(expiredIDs) > 0 {
		if w := r.data.RW().Write(ctx); w != nil {
			// Best-effort lazy cleanup: failure leaves rows filtered by reads anyway.
			_, _ = w.ToolGrant.Delete().Where(toolgrant.IDIn(expiredIDs...)).Exec(ctx)
		}
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
	// Upsert: re-granting an existing (agent_id, tool_key) pair renews the
	// window (granted_by/created_at/expires_at take the new values). Ignore()
	// would keep a stale/expired expires_at and the prompt would loop.
	err := client.ToolGrant.Create().
		SetID(id).
		SetAgentID(agentID).
		SetToolKey(toolKey).
		SetGrantedBy(strings.TrimSpace(grant.GrantedBy)).
		SetCreatedAt(nowRFC3339()).
		SetExpiresAt(strings.TrimSpace(grant.ExpiresAt)).
		OnConflictColumns(toolgrant.FieldAgentID, toolgrant.FieldToolKey).
		UpdateNewValues().
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
