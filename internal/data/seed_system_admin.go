package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const systemAdminAgentKey = "__system_admin__"

func SeedSystemAdminAgent(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		taxonomy_position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind,
		position_key, agent_variant
	) VALUES (
		'agent___system_admin__', ?, '系统管家', 'openrouter', 'gpt-4.1-mini',
		'active', 0, 0, '', '系统内置管理助手，负责管理 Skill、Agent、Team 等系统资源，提供系统级运维能力。',
		'', 'complete', 0, 0, '{"tools_profile":"system_admin"}', '[]', 'system',
		?, ?, '', 1, 'system_builtin',
		'system_admin', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		display_name = excluded.display_name,
		agent_description = excluded.agent_description,
		readonly = excluded.readonly,
		kind = excluded.kind,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, q, systemAdminAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.system_admin_agent"), loggateway.Err(err))
		return kerrors.InternalServer("SEED", "seed system admin agent: "+err.Error())
	}
	return nil
}

func SeedSpiritAgent(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		taxonomy_position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind,
		position_key, agent_variant
	) VALUES (
		'agent___spirit__', ?, '精灵助手', 'openrouter', 'gpt-4.1-mini',
		'active', 0, 0, '', '系统内置总管家，用户唯一对话入口，自动组装团队并委派工作。',
		'', 'complete', 0, 0, '{"tools_profile":"spirit"}', '[]', 'system',
		?, ?, '', 1, 'system_builtin',
		'spirit', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		display_name = excluded.display_name,
		agent_description = excluded.agent_description,
		readonly = excluded.readonly,
		kind = excluded.kind,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, q, biz.SpiritAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_agent"), loggateway.Err(err))
		return kerrors.InternalServer("SEED", "seed spirit agent: "+err.Error())
	}
	return nil
}

func SeedSpiritPromptFiles(ctx context.Context, client *ent.Client, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	promptDir := filepath.Join(scenarioDir, "system", "prompts")
	entries, err := os.ReadDir(promptDir)
	if err != nil {
		lg.Warn("spirit prompt dir not found, skipping",
			loggateway.StepID("data.seed.spirit_prompt_files"),
			loggateway.Str("dir", promptDir),
			loggateway.Err(err))
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const agentID = "agent___spirit__"
	const q = `INSERT INTO agent_prompt_files (
		id, agent_id, file_name, body, sort_order, created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		body = excluded.body,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`
	sortOrder := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fileName := strings.TrimSuffix(e.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(promptDir, e.Name()))
		if err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_prompt_files"), loggateway.Str("file", e.Name()), loggateway.Err(err))
			return kerrors.InternalServer("SEED", "read spirit prompt file "+e.Name()+": "+err.Error())
		}
		id := "apf_spirit_" + fileName
		if _, err := client.ExecContext(ctx, q, id, agentID, fileName, string(data), sortOrder, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_prompt_files"), loggateway.Str("file_name", fileName), loggateway.Err(err))
			return kerrors.InternalServer("SEED", "seed spirit prompt file "+fileName+": "+err.Error())
		}
		sortOrder++
	}
	return nil
}

func SeedBuiltinCLIAdminTools(ctx context.Context, client *ent.Client, lg loggateway.Logger) error {
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
			lg.Warn("seed step failed", loggateway.StepID("data.seed.cli_admin_tools"), loggateway.Str("tool_key", t.key), loggateway.Err(err))
			return kerrors.InternalServer("SEED", "seed cli_admin tool "+t.key+": "+err.Error())
		}
	}
	return nil
}
