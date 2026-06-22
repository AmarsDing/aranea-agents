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
)

const systemAdminAgentKey = biz.SystemAdminAgentKey

func SeedSystemAdminAgent(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind, source,
		position_key, agent_variant
	) VALUES (
		'agent___system_admin__', ?, '系统管家', 'openrouter', 'gpt-4.1-mini',
		'active', FALSE, FALSE, '', '系统内置管理助手，负责管理 Skill、Agent、Team 等系统资源，提供系统级运维能力。',
		'', 'complete', 0, 0, '{"tools_profile":"system_admin"}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'system_admin', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		status = excluded.status,
		agent_description = excluded.agent_description,
		system_prompt_mode = excluded.system_prompt_mode,
		config_json = excluded.config_json,
		deleted_at = excluded.deleted_at,
		readonly = excluded.readonly,
		kind = excluded.kind,
		source = excluded.source,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), systemAdminAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.system_admin_agent"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

func SeedSpiritAgent(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind, source,
		position_key, agent_variant
	) VALUES (
		'agent___spirit__', ?, '精灵助手', 'openrouter', 'gpt-4.1-mini',
		'active', FALSE, FALSE, '', '系统内置总管家，用户唯一对话入口，自动组装团队并委派工作。',
		'', 'complete', 0, 0, '{"tools":{"profile":"spirit"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'spirit', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		status = excluded.status,
		agent_description = excluded.agent_description,
		system_prompt_mode = excluded.system_prompt_mode,
		config_json = excluded.config_json,
		deleted_at = excluded.deleted_at,
		readonly = excluded.readonly,
		kind = excluded.kind,
		source = excluded.source,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), biz.SpiritAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_agent"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

func SeedSpiritPromptFiles(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
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
			return entErrToBizErr(err, "SEED")
		}
		id := "apf_spirit_" + fileName
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, agentID, fileName, string(data), sortOrder, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_prompt_files"), loggateway.Str("file_name", fileName), loggateway.Err(err))
			return entErrToBizErr(err, "SEED")
		}
		sortOrder++
	}
	return nil
}

func SeedMemoryAgent(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind, source,
		position_key, agent_variant
	) VALUES (
		'agent___memory__', ?, '记忆管家', 'openrouter', 'gpt-4.1',
		'active', FALSE, FALSE, '', '基于学术原则的智能记忆管理者：选择性记忆、质量驱动遗忘、记忆蒸馏',
		'', 'complete', 0, 0, '{"tools_profile":"system_memory"}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'memory', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		status = excluded.status,
		agent_description = excluded.agent_description,
		system_prompt_mode = excluded.system_prompt_mode,
		config_json = excluded.config_json,
		deleted_at = excluded.deleted_at,
		readonly = excluded.readonly,
		kind = excluded.kind,
		source = excluded.source,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), biz.MemoryAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.memory_agent"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

func SeedSkillsAgent(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agents (
		id, agent_key, display_name, provider, model, status,
		is_default, is_favorite, icon, agent_description,
		position_id, system_prompt_mode, context_window,
		budget_monthly_cents, config_json, roles_json, created_by,
		created_at, updated_at, deleted_at, readonly, kind, source,
		position_key, agent_variant
	) VALUES (
		'agent___skills__', ?, '技能管家', 'openrouter', 'gpt-4.1',
		'active', FALSE, FALSE, '', '基于使用数据的技能进化/消亡决策、工具权重优化、编排分析',
		'', 'complete', 0, 0, '{"tools_profile":"system_skills"}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'skills', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		status = excluded.status,
		agent_description = excluded.agent_description,
		system_prompt_mode = excluded.system_prompt_mode,
		config_json = excluded.config_json,
		deleted_at = excluded.deleted_at,
		readonly = excluded.readonly,
		kind = excluded.kind,
		source = excluded.source,
		position_key = excluded.position_key,
		agent_variant = excluded.agent_variant,
		updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), biz.SkillsAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.skills_agent"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

func SeedButlerPromptFiles(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	type butlerPrompt struct {
		agentID string
		prefix  string
		dirName string
	}
	butlers := []butlerPrompt{
		{agentID: "agent___memory__", prefix: "apf_memory_", dirName: "memory"},
		{agentID: "agent___skills__", prefix: "apf_skills_", dirName: "skills"},
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agent_prompt_files (
		id, agent_id, file_name, body, sort_order, created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		body = excluded.body,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`
	for _, b := range butlers {
		promptDir := filepath.Join(scenarioDir, "system", "prompts", b.dirName)
		entries, err := os.ReadDir(promptDir)
		if err != nil {
			lg.Warn("butler prompt dir not found, skipping",
				loggateway.StepID("data.seed.butler_prompt_files"),
				loggateway.Str("dir", promptDir),
				loggateway.Err(err))
			continue
		}
		sortOrder := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			fileName := strings.TrimSuffix(e.Name(), ".md")
			data, err := os.ReadFile(filepath.Join(promptDir, e.Name()))
			if err != nil {
				lg.Warn("seed step failed", loggateway.StepID("data.seed.butler_prompt_files"), loggateway.Str("file", e.Name()), loggateway.Err(err))
				return entErrToBizErr(err, "SEED")
			}
			id := b.prefix + fileName
			if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, b.agentID, fileName, string(data), sortOrder, now, now); err != nil {
				lg.Warn("seed step failed", loggateway.StepID("data.seed.butler_prompt_files"), loggateway.Str("file_name", fileName), loggateway.Err(err))
				return entErrToBizErr(err, "SEED")
			}
			sortOrder++
		}
	}
	return nil
}

func SeedCronTasks(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tasks := []struct {
		id          string
		taskKey     string
		name        string
		description string
		agentID     string
		configJSON  string
	}{
		{
			id:          "cron_dream_cycle",
			taskKey:     "dream_cycle",
			name:        "记忆整理周期",
			description: "每日凌晨3点触发记忆管家执行 dream_cycle：删除 misaligned 记忆、遗忘不活跃记忆、去重、蒸馏",
			agentID:     "agent___memory__",
			// dry_run=true: 默认仅模拟执行，避免首次启动误删记忆。生产环境可通过管理面板关闭。
			configJSON: `{"schedule":"0 3 * * *","dry_run":true}`,
		},
		{
			id:          "cron_skill_health_scan",
			taskKey:     "skill_health_scan",
			name:        "技能健康扫描",
			description: "每周一凌晨4点触发技能管家执行 Skill 健康度分析",
			agentID:     "agent___skills__",
			configJSON:  `{"schedule":"0 4 * * 1"}`,
		},
	}
	const q = `INSERT INTO cron_task (
		id, task_key, name, description, status, enabled, sort_order,
		agent_id, config_json, metadata_json, created_at, updated_at, deleted_at
	) VALUES (
		?, ?, ?, ?, 'active', TRUE, 0,
		?, ?, '{}', ?, ?, ''
	) ON CONFLICT(task_key) DO UPDATE SET
		name = excluded.name,
		description = excluded.description,
		agent_id = excluded.agent_id,
		config_json = excluded.config_json,
		updated_at = excluded.updated_at`
	for _, t := range tasks {
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), t.id, t.taskKey, t.name, t.description, t.agentID, t.configJSON, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.cron_tasks"), loggateway.Str("task_key", t.taskKey), loggateway.Err(err))
			return entErrToBizErr(err, "SEED")
		}
	}
	return nil
}

func SeedBuiltinCLIAdminTools(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
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
		?, ?, ?, ?, 'cli_admin', 'builtin', 'medium', FALSE, TRUE,
		FALSE, FALSE, FALSE, '{}', '{}', '{}', '{}', '{}', '{}',
		?, ?, ''
	) ON CONFLICT(tool_key) DO NOTHING`
	for _, t := range tools {
		id := "tool_" + t.key
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, t.key, t.name, t.desc, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.cli_admin_tools"), loggateway.Str("tool_key", t.key), loggateway.Err(err))
			return entErrToBizErr(err, "SEED")
		}
	}
	return nil
}

// SeedDeptLeadAgents creates department lead agents for all existing department-level
// org nodes. This is called during seed to ensure every department has a lead agent.
func SeedDeptLeadAgents(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	// Query all department-level org nodes that don't have a dept_lead_agent_id
	rows, err := client.QueryContext(ctx, `SELECT id, org_key, name, description FROM organizations WHERE level = 'department' AND deleted_at = ''`)
	if err != nil {
		lg.Warn("seed step failed: query departments", loggateway.StepID("data.seed.dept_lead_agents"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	defer rows.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	for rows.Next() {
		var id, key, name, desc string
		if err := rows.Scan(&id, &key, &name, &desc); err != nil {
			lg.Warn("seed step failed: scan department row", loggateway.StepID("data.seed.dept_lead_agents"), loggateway.Err(err))
			continue
		}
		agentKey := biz.DeptLeadAgentKeyPrefix + key + "__"
		agentID := "agent___dept_lead_" + key + "__"
		displayName := "部门主管-" + name
		description := "部门主管，负责「" + name + "」的资源协调和跨部门交付审批。"
		if desc != "" {
			description = "部门主管，负责「" + name + "」的资源协调和跨部门交付审批。" + desc
		}

		const q = `INSERT INTO agents (
			id, agent_key, display_name, provider, model, status,
			is_default, is_favorite, icon, agent_description,
			position_id, system_prompt_mode, context_window,
			budget_monthly_cents, config_json, roles_json, created_by,
			created_at, updated_at, deleted_at, readonly, kind, source,
			position_key, agent_variant
		) VALUES (
			?, ?, ?, 'openrouter', 'gpt-4.1-mini',
			'active', FALSE, FALSE, '', ?,
			'', 'complete', 0, 0, '{"tools_profile":"dept_lead","memory_enabled":true}', '[]', 'system',
			?, ?, '', TRUE, 'system_builtin', 'system',
			?, 'dept_lead'
		) ON CONFLICT(agent_key) DO UPDATE SET
			status = excluded.status,
			agent_description = excluded.agent_description,
			system_prompt_mode = excluded.system_prompt_mode,
			config_json = excluded.config_json,
			deleted_at = excluded.deleted_at,
			readonly = excluded.readonly,
			kind = excluded.kind,
			source = excluded.source,
			position_key = excluded.position_key,
			agent_variant = excluded.agent_variant,
			updated_at = excluded.updated_at`
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), agentID, agentKey, displayName, description, now, now, key+"_dept_lead"); err != nil {
			lg.Warn("seed step failed: create dept lead agent",
				loggateway.StepID("data.seed.dept_lead_agents"),
				loggateway.Str("dept_key", key),
				loggateway.Err(err))
			continue
		}

		// Link dept_lead_agent_id on the org node
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(`UPDATE organizations SET dept_lead_agent_id = ?, updated_at = ? WHERE id = ? AND (dept_lead_agent_id = '' OR dept_lead_agent_id IS NULL)`), agentID, now, id); err != nil {
			lg.Warn("seed step failed: link dept lead to org node",
				loggateway.StepID("data.seed.dept_lead_agents"),
				loggateway.Str("dept_id", id),
				loggateway.Err(err))
		}
	}
	return nil
}

// SeedDeptLeadPromptFiles seeds the dept_lead.md prompt file for each department lead agent.
func SeedDeptLeadPromptFiles(ctx context.Context, client *ent.Client, d Dialect, scenarioDir string, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	promptPath := filepath.Join(scenarioDir, "system", "prompts", "dept_lead.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		lg.Warn("dept_lead.md not found, skipping",
			loggateway.StepID("data.seed.dept_lead_prompt_files"),
			loggateway.Str("path", promptPath),
			loggateway.Err(err))
		return nil
	}

	// Find all dept lead agents
	rows, err := client.QueryContext(ctx, `SELECT id, agent_key FROM agents WHERE agent_variant = 'dept_lead' AND deleted_at = ''`)
	if err != nil {
		lg.Warn("seed step failed: query dept lead agents", loggateway.StepID("data.seed.dept_lead_prompt_files"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	defer rows.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO agent_prompt_files (
		id, agent_id, file_name, body, sort_order, created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?
	) ON CONFLICT(id) DO UPDATE SET
		body = excluded.body,
		sort_order = excluded.sort_order,
		updated_at = excluded.updated_at`

	for rows.Next() {
		var agentID, agentKey string
		if err := rows.Scan(&agentID, &agentKey); err != nil {
			lg.Warn("seed step failed: scan dept lead agent row", loggateway.StepID("data.seed.dept_lead_prompt_files"), loggateway.Err(err))
			continue
		}
		id := "apf_dept_lead_" + agentKey
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, agentID, "dept_lead", string(data), 0, now, now); err != nil {
			lg.Warn("seed step failed: seed dept lead prompt file",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("agent_key", agentKey),
				loggateway.Err(err))
		}
	}
	return nil
}
