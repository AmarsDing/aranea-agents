package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
)

const systemAdminAgentKey = "__system_admin__"

// SeedSystemAdminAgent upserts the built-in system admin agent.
// Uses raw SQL (ON CONFLICT DO NOTHING) so it remains idempotent across restarts.
//
// NOTE: The `readonly` and `kind` columns are added to the agents table via the
// ent schema extension in internal/data/ent/schema/agent.go. After editing that
// file you must regenerate: go generate ./internal/data/ent/...
// and run the corresponding SQL migration before calling this seed.
func SeedSystemAdminAgent(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Insert only if the agent_key doesn't already exist.
	// We don't use ent-generated setters because the readonly/kind columns may
	// not yet exist in all deployment environments (pending migration).
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		category_position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind
	) VALUES (
		'agent___system_admin__', ?, '系统管家', '', '',
		'active', 0, 0, '', '系统内置管理助手 Agent，负责管理 Skill / Agent / Team 等资源。',
		'', 'complete', 0, 0, '{"tools_profile":"system_admin"}', '[]', 'system',
		?, ?, '', 1, 'system'
	) ON CONFLICT(agent_key) DO NOTHING`
	if _, err := client.ExecContext(ctx, q, systemAdminAgentKey, now, now); err != nil {
		return fmt.Errorf("seed system admin agent: %w", err)
	}
	return nil
}

// SeedBuiltinCLIAdminTools seeds the cli_admin_* tool records in the tools table.
// The actual implementations are registered in internal/tools/cli_admin.
func SeedBuiltinCLIAdminTools(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tools := []struct {
		key  string
		name string
		desc string
	}{
		{"cli_admin_skill_list", "Skill 列表", "列出系统中所有已安装的 Skill"},
		{"cli_admin_skill_get", "Skill 详情", "获取指定 Skill 的详细信息"},
		{"cli_admin_skill_install_from_url", "从 URL 安装 Skill", "从 Git 仓库 URL 安装 Skill"},
		{"cli_admin_skill_import_status", "Skill 导入状态", "查询 Skill 导入任务状态"},
		{"cli_admin_skill_import_apply", "应用 Skill 导入", "确认并应用 Skill 导入"},
		{"cli_admin_agent_list", "Agent 列表", "列出系统中所有 Agent"},
		{"cli_admin_agent_get", "Agent 详情", "获取指定 Agent 的详细信息"},
		{"cli_admin_pkg_install_from_url", "从 URL 安装 Package", "从 Git 仓库 URL 安装整个 aranea package（含 MCP/Skill/Agent/Team/Graph）"},
	}
	const q = `INSERT INTO tools (
		id, tool_key, display_name, description, category, source, risk_level, enabled, readonly,
		requires_confirmation, supports_streaming, supports_concurrency,
		parameters_schema_json, result_schema_json, config_schema_json,
		config_json, default_config_json, metadata_json,
		created_at, updated_at, deleted_at
	) VALUES (
		?, ?, ?, ?, 'cli_admin', 'builtin', 'medium', 0, 1,
		0, 0, 0, '{}', '{}', '{}', '{}', '{}', '{}',
		?, ?, ''
	) ON CONFLICT(tool_key) DO NOTHING`
	for _, t := range tools {
		id := "tool_" + t.key
		if _, err := client.ExecContext(ctx, q, id, t.key, t.name, t.desc, now, now); err != nil {
			return fmt.Errorf("seed cli_admin tool %q: %w", t.key, err)
		}
	}
	return nil
}
