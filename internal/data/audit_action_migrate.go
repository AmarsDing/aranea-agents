package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// auditActionNormalizeMap 旧 action → 规范 "<verb>.<resource>"（动词在前）。
var auditActionNormalizeMap = map[string]string{
	"agent.create":          "create.agent",
	"agent.update":          "update.agent",
	"agent.delete":          "delete.agent",
	"tool.create":           "create.tool",
	"tool.update":           "update.tool",
	"tool.delete":           "delete.tool",
	"mcp_server.create":     "create.mcp_server",
	"mcp_server.update":     "update.mcp_server",
	"mcp_server.delete":     "delete.mcp_server",
	"archive.session.batch": "archive.session",
	"delete.session.batch":  "delete.session",
	"skill.filesystem.sync": "sync.skill",
}

// RunAuditActionNormalizeMigration 将 audit_logs 历史行规范化为 verb-first action + detail JSON 契约。
func RunAuditActionNormalizeMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("audit action normalize migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationAuditActionNormalize, lg)
	if err != nil {
		return fmt.Errorf("audit action normalize migration: check gate: %w", err)
	}
	if applied {
		return nil
	}

	hasTable, err := tableExistsWithDialect(ctx, client, lg, "audit_logs", d)
	if err != nil {
		return fmt.Errorf("audit action normalize migration: check table: %w", err)
	}
	if !hasTable {
		if err := recordMigrationApplied(ctx, client, d, MigrationAuditActionNormalize, migrationNameAuditActionNormalize, lg); err != nil {
			return fmt.Errorf("audit action normalize migration: record: %w", err)
		}
		return nil
	}

	for old, canonical := range auditActionNormalizeMap {
		if _, err := client.ExecContext(ctx,
			`UPDATE audit_logs SET action = $1 WHERE action = $2`, canonical, old); err != nil {
			return fmt.Errorf("audit action normalize migration: %s→%s: %w", old, canonical, err)
		}
	}

	// 纯文本 detail 包装为 {"summary":...}（to_json 负责转义，输出紧凑 JSON 与写入端一致）。
	if _, err := client.ExecContext(ctx,
		`UPDATE audit_logs SET detail = '{"summary":' || to_json(detail)::text || '}' WHERE detail <> '' AND detail NOT LIKE '{%'`); err != nil {
		return fmt.Errorf("audit action normalize migration: wrap detail: %w", err)
	}

	if err := recordMigrationApplied(ctx, client, d, MigrationAuditActionNormalize, migrationNameAuditActionNormalize, lg); err != nil {
		return fmt.Errorf("audit action normalize migration: record: %w", err)
	}
	lg.Info("audit action normalize migration: done", loggateway.StepID("migration.audit_action_normalize"))
	return nil
}
