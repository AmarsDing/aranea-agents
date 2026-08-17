package tool

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// DefaultToolGrantTTL is the default lifetime of a persisted "always allow"
// grant (BUG-MON-B, ruled 2026-08-17): 72h covers one drill cycle without
// crossing a week. After expiry the confirmation prompt re-arms (fail-closed).
const DefaultToolGrantTTL = 72 * time.Hour

// ToolGrant is a persisted "always allow" grant recorded when a user
// approves a tool confirmation with the always scope. Presence of a grant
// skips the confirmation prompt for the (AgentID, ToolKey) pair.
type ToolGrant struct {
	ID        string
	AgentID   string
	ToolKey   string
	GrantedBy string
	CreatedAt string
	// ExpiresAt is the RFC3339 UTC deadline ('' = never expires, reserved
	// for a future explicit "permanent" option). Read paths treat rows with
	// ExpiresAt <= now as absent.
	ExpiresAt string
}

// ToolGrantReader reads persisted tool grants.
// Stability:evolving
type ToolGrantReader interface {
	HasToolGrant(ctx context.Context, agentID, toolKey string) (bool, error)
	ListToolGrants(ctx context.Context, agentID string) ([]ToolGrant, error)
}

// ToolGrantWriter writes persisted tool grants. Create is idempotent:
// granting the same (agentID, toolKey) twice must not fail.
// Stability:evolving
type ToolGrantWriter interface {
	CreateToolGrant(ctx context.Context, grant ToolGrant) error
	DeleteToolGrant(ctx context.Context, agentID, toolKey string) error
}

// ToolGrantStore combines reader and writer; used only for Wire wiring.
// Stability:evolving
type ToolGrantStore interface {
	ToolGrantReader
	ToolGrantWriter
}

// WithToolGrantStore wires the persisted grant store into the usecase.
func WithToolGrantStore(store ToolGrantStore) ToolUsecaseOption {
	return func(u *ToolUsecase) { u.grants = store }
}

// HasToolGrant reports whether a persisted grant exists for the pair.
// Store errors degrade to false (fail-closed: the confirmation prompt
// still appears) — never to silently allowed.
func (u *ToolUsecase) HasToolGrant(ctx context.Context, agentID, toolKey string) bool {
	if u == nil || u.grants == nil {
		return false
	}
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return false
	}
	has, err := u.grants.HasToolGrant(ctx, agentID, toolKey)
	if err != nil {
		u.lg.Warn("tool grant lookup failed, treating as no grant",
			loggateway.Str("agent_id", agentID),
			loggateway.Str("tool_key", toolKey),
			loggateway.Err(err))
		return false
	}
	return has
}

// GrantTool persists an "always allow" grant for the (agentID, toolKey) pair.
// The grant carries the default 72h TTL (DefaultToolGrantTTL); re-granting an
// existing pair renews the window from now.
func (u *ToolUsecase) GrantTool(ctx context.Context, agentID, toolKey, grantedBy string) error {
	if u == nil || u.grants == nil {
		return apierror.Internal("TOOL", "tool grant store unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return apierror.BadRequest("TOOL", "agent_id and tool_key are required")
	}
	return u.grants.CreateToolGrant(ctx, ToolGrant{
		AgentID:   agentID,
		ToolKey:   toolKey,
		GrantedBy: strings.TrimSpace(grantedBy),
		ExpiresAt: time.Now().UTC().Add(DefaultToolGrantTTL).Format(time.RFC3339),
	})
}

// RevokeToolGrant removes a persisted grant. Idempotent.
func (u *ToolUsecase) RevokeToolGrant(ctx context.Context, agentID, toolKey string) error {
	if u == nil || u.grants == nil {
		return apierror.Internal("TOOL", "tool grant store unavailable")
	}
	return u.grants.DeleteToolGrant(ctx, agentID, toolKey)
}

// ListToolGrants lists persisted grants for an agent.
func (u *ToolUsecase) ListToolGrants(ctx context.Context, agentID string) ([]ToolGrant, error) {
	if u == nil || u.grants == nil {
		return nil, apierror.Internal("TOOL", "tool grant store unavailable")
	}
	return u.grants.ListToolGrants(ctx, agentID)
}
