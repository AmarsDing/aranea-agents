package service

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// agentCatalog is the narrow Get used for Memory Center / debug-recall IDOR.
type agentCatalog interface {
	Get(ctx context.Context, id string) (biz.Agent, error)
}

// authorizeMemoryScope enforces Memory Center scope ACL for Admin RPCs.
//
// Rules (aligned with memory.md §2 + existing workspace tenancy):
//   - system workspace caller → bypass
//   - global write → requires admin Access; global read → any authenticated caller
//   - workspace → scope_id must match caller workspace (empty → forced to caller)
//   - user → scope_id must match caller user id (empty → forced); workspace admin may
//     access other users in the same tenant
//   - agent / team / session → authenticated caller required; write requires non-empty
//     scope_id (entity ownership cross-check deferred to agent/team repos)
//
// Returns the effective scope_id to use for the query (may be normalized).
func authorizeMemoryScope(ctx context.Context, scopeType, scopeID string, write bool) (string, error) {
	scopeType = strings.ToLower(strings.TrimSpace(scopeType))
	scopeID = strings.TrimSpace(scopeID)

	if workspace.IsSystem(ctx) {
		return scopeID, nil
	}

	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return "", auth.ErrUnauthorized
	}
	callerWS := workspace.IDFromContext(ctx)

	switch scopeType {
	case "":
		// Memory Center may list without a scope filter. Non-admins are narrowed
		// to their own user scope; admins may list across scopes in-tenant.
		if write {
			return "", apierror.BadRequest(apierror.DomainMemory, "scope_type is required")
		}
		if a.HasAdminAccess() {
			return scopeID, nil
		}
		return strconv.FormatInt(a.UserID, 10), nil
	case "global":
		if write && !a.HasAdminAccess() {
			return "", apierror.Forbidden(apierror.DomainMemory, "global memory write requires admin access")
		}
		return scopeID, nil
	case "workspace":
		if scopeID == "" {
			scopeID = callerWS
		}
		if err := workspace.AssertWorkspace(callerWS, scopeID); err != nil {
			return "", err
		}
		return scopeID, nil
	case "user":
		uid := strconv.FormatInt(a.UserID, 10)
		if scopeID == "" {
			scopeID = uid
		}
		if scopeID != uid && !a.HasAdminAccess() {
			return "", apierror.Forbidden(apierror.DomainMemory, "cannot access another user's memory scope")
		}
		return scopeID, nil
	case "agent", "team", "session":
		if write && scopeID == "" {
			return "", apierror.BadRequest(apierror.DomainMemory, "scope_id is required for %s writes", scopeType)
		}
		return scopeID, nil
	default:
		return "", apierror.BadRequest(apierror.DomainMemory, "unsupported scope_type: %s", scopeType)
	}
}

// authorizeMemoryWorkspaceField forces non-system callers' workspace_id filter
// to their own workspace (prevents cross-tenant listing via workspace_id param).
func authorizeMemoryWorkspaceField(ctx context.Context, workspaceID string) (string, error) {
	if workspace.IsSystem(ctx) {
		return strings.TrimSpace(workspaceID), nil
	}
	callerWS := workspace.IDFromContext(ctx)
	ws := strings.TrimSpace(workspaceID)
	if ws == "" {
		return callerWS, nil
	}
	if err := workspace.AssertWorkspace(callerWS, ws); err != nil {
		return "", err
	}
	return ws, nil
}

// assertAgentMemoryAccess enforces login + agent workspace tenancy for
// Memory Center aggregation and debug-recall RPCs. Cross-tenant callers
// receive NotFound (same contract as AgentService.assertAgentAccess).
func (s *MemoryService) assertAgentMemoryAccess(ctx context.Context, agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return apierror.BadRequest(apierror.DomainMemory, "agent_id is required")
	}
	if _, err := authorizeMemoryScope(ctx, "agent", agentID, false); err != nil {
		return err
	}
	if workspace.IsSystem(ctx) {
		return nil
	}
	if s.agentUC == nil {
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	a, err := s.agentUC.Get(ctx, agentID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), a.WorkspaceID); err != nil {
		s.lg.Warn("memory center access denied: workspace mismatch",
			loggateway.StepID("memory.idor"),
			loggateway.Str("agent_id", agentID),
			loggateway.Str("caller_ws", workspace.IDFromContext(ctx)))
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	return nil
}

func memoryHTTPQuery(ctx context.Context, keys ...string) string {
	r, ok := khttp.RequestFromServerContext(ctx)
	if !ok || r == nil {
		return ""
	}
	q := r.URL.Query()
	for _, k := range keys {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			return v
		}
	}
	return ""
}
