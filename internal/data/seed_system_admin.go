package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	systemprompts "aranea-agents/internal/scenario/system"
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
		'', 'complete', 0, 0, '{"tools":{"profile":"system_admin"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'', ''
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
		'active', TRUE, FALSE, '', '系统内置总管家，用户唯一对话入口，自动组装团队并委派工作。',
		'', 'complete', 0, 0, '{"tools":{"profile":"spirit"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'', ''
	) ON CONFLICT(agent_key) DO UPDATE SET
		status = excluded.status,
		is_default = excluded.is_default,
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
	files, err := loadSpiritPromptMarkdown(scenarioDir, lg)
	if err != nil {
		return entErrToBizErr(err, "SEED")
	}
	if len(files) == 0 {
		lg.Warn("spirit prompts empty (filesystem + embed)",
			loggateway.StepID("data.seed.spirit_prompt_files"))
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
	keep := make([]string, 0, len(files))
	for _, f := range files {
		fileName := strings.TrimSuffix(f.name, ".md")
		keep = append(keep, fileName)
		id := "apf_spirit_" + fileName
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, agentID, fileName, f.body, sortOrder, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.spirit_prompt_files"), loggateway.Str("file_name", fileName), loggateway.Err(err))
			return entErrToBizErr(err, "SEED")
		}
		sortOrder++
	}
	if err := deleteStaleSpiritPromptFiles(ctx, client, d, agentID, keep); err != nil {
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

// spiritPromptMarkdownAllow is the only prompt set mounted on __spirit__.
// company_lead.md / dept_lead.md / orchestrator.md belong on their own agents;
// seeding them here duplicated Graph rules and made the spirit look like a CEO.
var spiritPromptMarkdownAllow = map[string]struct{}{
	"IDENTITY.md":     {},
	"CAPABILITIES.md": {},
	"DECISION.md":     {},
}

func isSpiritPromptMarkdown(name string) bool {
	_, ok := spiritPromptMarkdownAllow[name]
	return ok
}

func deleteStaleSpiritPromptFiles(ctx context.Context, client *ent.Client, d Dialect, agentID string, keep []string) error {
	if client == nil || len(keep) == 0 {
		return nil
	}
	placeholders := make([]string, len(keep))
	args := make([]any, 0, 1+len(keep))
	args = append(args, agentID)
	for i, name := range keep {
		placeholders[i] = "?"
		args = append(args, name)
	}
	q := `DELETE FROM agent_prompt_files WHERE agent_id = ? AND file_name NOT IN (` + strings.Join(placeholders, ",") + `)`
	_, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), args...)
	return err
}

type promptMDFile struct {
	name string
	body string
}

// loadSpiritPromptMarkdown prefers on-disk scenario prompts, then embedded fallback.
func loadSpiritPromptMarkdown(scenarioDir string, lg loggateway.Logger) ([]promptMDFile, error) {
	promptDir := filepath.Join(scenarioDir, "system", "prompts")
	entries, err := os.ReadDir(promptDir)
	if err == nil {
		out := make([]promptMDFile, 0, len(spiritPromptMarkdownAllow))
		for _, e := range entries {
			if e.IsDir() || !isSpiritPromptMarkdown(e.Name()) {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(promptDir, e.Name()))
			if readErr != nil {
				return nil, readErr
			}
			out = append(out, promptMDFile{name: e.Name(), body: string(data)})
		}
		if len(out) > 0 {
			return out, nil
		}
	} else {
		lg.Warn("spirit prompt dir missing, using embedded prompts",
			loggateway.StepID("data.seed.spirit_prompt_files"),
			loggateway.Str("dir", promptDir),
			loggateway.Err(err))
	}
	names, listErr := systemprompts.ListTopLevelMarkdown()
	if listErr != nil {
		return nil, listErr
	}
	out := make([]promptMDFile, 0, len(spiritPromptMarkdownAllow))
	for _, name := range names {
		if !isSpiritPromptMarkdown(name) {
			continue
		}
		body, readErr := systemprompts.ReadMarkdown(name)
		if readErr != nil {
			return nil, readErr
		}
		out = append(out, promptMDFile{name: name, body: body})
	}
	return out, nil
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
		'', 'complete', 0, 0, '{"tools":{"profile":"system_memory"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'', ''
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
		'', 'complete', 0, 0, '{"tools":{"profile":"system_skills"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'', ''
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

// SeedVoiceButlerAgent seeds the built-in voice butler agent (M74 V9, design 74 §15).
// The voice butler is the voice-mode foreground agent: quick answers, delegation
// of complex tasks to the spirit via delegate_to_spirit, and result broadcasting.
// It is a peer of the spirit (not a replacement): the spirit keeps full capability.
func SeedVoiceButlerAgent(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
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
		'agent___voice_butler__', ?, '语音助手', 'openrouter', 'gpt-4.1-mini',
		'active', FALSE, FALSE, '', '系统内置语音前台助手，语音模式对话入口：快答闲聊、复杂任务委派精灵助手后台执行、完成结果实时播报。',
		'', 'complete', 0, 0, '{"tools":{"profile":"chat_only"}}', '[]', 'system',
		?, ?, '', TRUE, 'system_builtin', 'system',
		'', ''
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
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), biz.VoiceButlerAgentKey, now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.voice_butler_agent"), loggateway.Err(err))
		return entErrToBizErr(err, "SEED")
	}
	return nil
}

// SeedSystemAgentRuntimeSettings ensures the five system agents have runtime_settings rows.
// Missing rows cause GetAgentRuntimeSettings → NOT_FOUND and break evolution/settings UX.
func SeedSystemAgentRuntimeSettings(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	agents := []struct {
		id           string
		toolsProfile string
	}{
		{id: "agent___spirit__", toolsProfile: "spirit"},
		{id: "agent___system_admin__", toolsProfile: "system_admin"},
		{id: "agent___memory__", toolsProfile: "system_memory"},
		{id: "agent___skills__", toolsProfile: "system_skills"},
	}
	// Minimal insert: rely on column defaults for the rest of the wide settings row.
	const q = `INSERT INTO agent_runtime_settings (agent_id, tools_profile, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (agent_id) DO UPDATE SET
			tools_profile = excluded.tools_profile,
			updated_at = excluded.updated_at`
	for _, a := range agents {
		if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), a.id, a.toolsProfile, now, now); err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.system_agent_runtime_settings"), loggateway.Str("agent_id", a.id), loggateway.Err(err))
			return entErrToBizErr(err, "SEED")
		}
	}

	// Voice butler (M74 V9, design 74 §15): chat_only profile + explicit
	// intent_pass_enabled=false. The DB column defaults to true
	// (agent_runtime_setting.go), so the minimal insert above would silently
	// enable IntentPass — the voice fast path must not pay its latency.
	const qVoice = `INSERT INTO agent_runtime_settings (agent_id, tools_profile, intent_pass_enabled, created_at, updated_at)
		VALUES (?, 'chat_only', FALSE, ?, ?)
		ON CONFLICT (agent_id) DO UPDATE SET
			tools_profile = excluded.tools_profile,
			intent_pass_enabled = excluded.intent_pass_enabled,
			updated_at = excluded.updated_at`
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(qVoice), "agent___voice_butler__", now, now); err != nil {
		lg.Warn("seed step failed", loggateway.StepID("data.seed.system_agent_runtime_settings"), loggateway.Str("agent_id", "agent___voice_butler__"), loggateway.Err(err))
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
		{agentID: "agent___system_admin__", prefix: "apf_system_admin_", dirName: "system_admin"},
		{agentID: "agent___voice_butler__", prefix: "apf_voice_butler_", dirName: "voice_butler"},
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
		files, loadErr := loadButlerPromptMarkdown(scenarioDir, b.dirName, lg)
		if loadErr != nil {
			lg.Warn("butler prompts load failed",
				loggateway.StepID("data.seed.butler_prompt_files"),
				loggateway.Str("dir", b.dirName),
				loggateway.Err(loadErr))
			continue
		}
		sortOrder := 0
		for _, f := range files {
			fileName := strings.TrimSuffix(f.name, ".md")
			id := b.prefix + fileName
			if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), id, b.agentID, fileName, f.body, sortOrder, now, now); err != nil {
				lg.Warn("seed step failed", loggateway.StepID("data.seed.butler_prompt_files"), loggateway.Str("file_name", fileName), loggateway.Err(err))
				return entErrToBizErr(err, "SEED")
			}
			sortOrder++
		}
	}
	return nil
}

func loadButlerPromptMarkdown(scenarioDir, dirName string, lg loggateway.Logger) ([]promptMDFile, error) {
	promptDir := filepath.Join(scenarioDir, "system", "prompts", dirName)
	entries, err := os.ReadDir(promptDir)
	if err == nil {
		out := make([]promptMDFile, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(promptDir, e.Name()))
			if readErr != nil {
				return nil, readErr
			}
			out = append(out, promptMDFile{name: e.Name(), body: string(data)})
		}
		if len(out) > 0 {
			return out, nil
		}
	} else {
		lg.Warn("butler prompt dir missing, using embedded prompts",
			loggateway.StepID("data.seed.butler_prompt_files"),
			loggateway.Str("dir", promptDir),
			loggateway.Err(err))
	}
	names, listErr := systemprompts.ListSubdirMarkdown(dirName)
	if listErr != nil {
		return nil, listErr
	}
	out := make([]promptMDFile, 0, len(names))
	for _, name := range names {
		body, readErr := systemprompts.ReadMarkdown(dirName + "/" + name)
		if readErr != nil {
			return nil, readErr
		}
		out = append(out, promptMDFile{name: name, body: body})
	}
	return out, nil
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
			// dry_run=true: 默认仅模拟执行，避免首次启动误删记忆。生产环境可通过管理面板修改 message 关闭。
			configJSON: `{"target_type":"agent","schedule_type":"cron","cron_expression":"0 3 * * *","timezone":"Asia/Shanghai","message":"请执行 memory_butler_dream_cycle 离线记忆整理：依次清理 misaligned 记忆、遗忘不活跃记忆、去重、蒸馏。使用 dry_run=true 仅模拟执行并输出报告，不实际删除。","retry_max_attempts":3}`,
		},
		{
			id:          "cron_skill_health_scan",
			taskKey:     "skill_health_scan",
			name:        "技能健康扫描",
			description: "每周一凌晨4点触发技能管家执行 Skill 健康度分析",
			agentID:     "agent___skills__",
			configJSON:  `{"target_type":"agent","schedule_type":"cron","cron_expression":"0 4 * * 1","timezone":"Asia/Shanghai","message":"请使用 skills_butler_analyze_skill_health 对所有已安装 Skill 执行健康度分析，并输出健康扫描报告。","retry_max_attempts":3}`,
		},
	}
	// upsert 语义：
	// - 新行：插入完整配置。
	// - 已存在且为旧 schema（无 schedule_type，runner/前端无法解析，必然每分钟失败并进死信）：
	//   修复 config 并复活（重置 dead 状态与失败计数）——死信是该配置 bug 导致的，非用户意图。
	// - 已存在且为新 schema：视为用户已在管理面板编辑过，不做任何覆盖，保留用户修改。
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
		status = 'active',
		enabled = TRUE,
		metadata_json = '{}',
		updated_at = excluded.updated_at
	WHERE cron_task.config_json NOT LIKE '%schedule_type%'
		AND cron_task.deleted_at = ''`
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
		if strings.HasSuffix(key, biz.CompanyOfficeDeptSuffix) {
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
			'', 'complete', 0, 0, '{"tools":{"profile":"dept_lead"},"memory_enabled":true}', '[]', 'system',
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
	if _, err := client.ExecContext(ctx, d.RenumberPlaceholders(q), agentID, agentKey, displayName, description, now, now, ""); err != nil {
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
		body, embErr := systemprompts.ReadMarkdown("dept_lead.md")
		if embErr != nil {
			lg.Warn("dept_lead.md not found (disk+embed), skipping",
				loggateway.StepID("data.seed.dept_lead_prompt_files"),
				loggateway.Str("path", promptPath),
				loggateway.Err(err))
			return nil
		}
		lg.Warn("dept_lead.md missing on disk, using embedded prompt",
			loggateway.StepID("data.seed.dept_lead_prompt_files"),
			loggateway.Str("path", promptPath),
			loggateway.Err(err))
		data = []byte(body)
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
