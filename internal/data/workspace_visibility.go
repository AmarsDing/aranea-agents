package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/workspace"
)

// workspaceSharedOrOwnIDs returns workspace_id values visible to the caller for
// shareable entities (agent/team/channel/mcp/skill/tool/plugin/graph).
// Empty string = shared/legacy. Returns nil when the caller is system (no filter).
func workspaceSharedOrOwnIDs(ctx context.Context) []string {
	if workspace.IsSystem(ctx) {
		return nil
	}
	return []string{"", workspace.IDFromContext(ctx)}
}

// workspacePrivateIDs returns workspace_id values for private entities
// (cron_task, tasks_v2, task_plans, eval_runs).
// System: nil (no filter). Default workspace also sees legacy empty rows.
func workspacePrivateIDs(ctx context.Context) []string {
	if workspace.IsSystem(ctx) {
		return nil
	}
	callerWS := workspace.IDFromContext(ctx)
	if callerWS == workspace.DefaultWorkspaceID {
		return []string{callerWS, ""}
	}
	return []string{callerWS}
}

// SetAppWorkspaceID sets the Postgres GUC app.workspace_id on db.
// Required for RLS policies in 20261011_tenant_rls_phase1.sql to take effect.
// Prefer calling once per request/transaction after resolving the caller workspace
// (see middleware.WorkspaceFilter). No-op when db is nil.
//
// Uses set_config(..., false) so the value lasts for the session/connection
// (pool-safe only if each request uses a dedicated conn or SET LOCAL in a txn).
func SetAppWorkspaceID(ctx context.Context, db *sql.DB, workspaceID string) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `SELECT set_config('app.workspace_id', $1, false)`, workspaceID)
	return err
}

// SetAppWorkspaceIDOnConn sets app.workspace_id on a single acquired connection.
// Prefer this over SetAppWorkspaceID when using connection pools: set GUC on the
// conn used for the request, then release the conn.
func SetAppWorkspaceIDOnConn(ctx context.Context, conn *sql.Conn, workspaceID string) error {
	if conn == nil {
		return nil
	}
	_, err := conn.ExecContext(ctx, `SELECT set_config('app.workspace_id', $1, true)`, workspaceID)
	return err
}
