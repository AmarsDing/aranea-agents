package sqlite

import (
	"database/sql"
	"strings"

	"arenea/backend/internal/domain"
)

// SeedBuiltinTools 对系统内置工具集做 upsert；含 cli_admin_* 工具集（aranea/docs/25 cli.md §6）。
func SeedBuiltinTools(db *sql.DB) error {
	now := nowISO()
	allSeeds := make([]domain.Tool, 0, len(builtinToolSeeds)+len(cliAdminToolSeeds))
	allSeeds = append(allSeeds, builtinToolSeeds...)
	allSeeds = append(allSeeds, cliAdminToolSeeds...)
	for _, row := range allSeeds {
		applyBuiltinToolDefaults(&row)
		_, err := db.Exec(
			`INSERT INTO tools(
			 id, tool_key, display_name, description, category, source, risk_level, enabled, readonly, requires_confirmation,
			 supports_streaming, supports_concurrency, parameters_schema_json, result_schema_json, config_schema_json, config_json,
			 default_config_json, metadata_json, created_at, updated_at, deleted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
			ON CONFLICT(tool_key) DO UPDATE SET
			 display_name = excluded.display_name,
			 description = excluded.description,
			 category = excluded.category,
			 source = excluded.source,
			 risk_level = excluded.risk_level,
			 readonly = excluded.readonly,
			 requires_confirmation = excluded.requires_confirmation,
			 supports_streaming = excluded.supports_streaming,
			 supports_concurrency = excluded.supports_concurrency,
			 parameters_schema_json = excluded.parameters_schema_json,
			 result_schema_json = excluded.result_schema_json,
			 config_schema_json = excluded.config_schema_json,
			 default_config_json = excluded.default_config_json,
			 metadata_json = excluded.metadata_json,
			 updated_at = excluded.updated_at,
			 deleted_at = ''`,
			row.ID, row.Key, row.DisplayName, row.Description, row.Category, row.Source, row.RiskLevel, row.Enabled, row.Readonly, row.RequiresConfirmation,
			row.SupportsStreaming, row.SupportsConcurrency, row.ParametersSchemaJSON, row.ResultSchemaJSON, row.ConfigSchemaJSON, row.ConfigJSON,
			row.DefaultConfigJSON, row.MetadataJSON, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func applyBuiltinToolDefaults(row *domain.Tool) {
	if row.ID == "" {
		row.ID = "tool_" + strings.ReplaceAll(row.Key, "-", "_")
	}
	if row.Source == "" {
		row.Source = "builtin"
	}
	if row.RiskLevel == "" {
		row.RiskLevel = "low"
	}
	if row.ParametersSchemaJSON == "" {
		row.ParametersSchemaJSON = "{}"
	}
	if row.ResultSchemaJSON == "" {
		row.ResultSchemaJSON = "{}"
	}
	if row.ConfigSchemaJSON == "" {
		row.ConfigSchemaJSON = "{}"
	}
	if row.ConfigJSON == "" {
		row.ConfigJSON = "{}"
	}
	if row.DefaultConfigJSON == "" {
		row.DefaultConfigJSON = row.ConfigJSON
	}
	if row.MetadataJSON == "" {
		row.MetadataJSON = "{}"
	}
}

// CLIAdminToolKeys 返回 cli_admin_* 工具 key 的稳定列表，用于 system_admin 与策略解析器。
func CLIAdminToolKeys() []string {
	keys := make([]string, len(cliAdminToolSeeds))
	for i, t := range cliAdminToolSeeds {
		keys[i] = t.Key
	}
	return keys
}

func cliAdminMeta(risk, method, path string) string {
	return `{"group":"cli_admin","cli_admin":true,"risk":"` + risk + `","http":{"method":"` + method + `","path":"` + path + `"}}`
}

var builtinToolSeeds = []domain.Tool{
	{ID: "tool_datetime", Key: "datetime", DisplayName: "当前时间", Description: "返回当前时间、日期和时区信息。", Category: "system", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{}}`},
	{ID: "tool_web_search", Key: "web_search", DisplayName: "Web 搜索", Description: "搜索实时网络信息，返回标题、链接和摘要。", Category: "web", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"limit":{"type":"number","description":"返回结果数量"}},"required":["query"]}`},
	{ID: "tool_web_fetch", Key: "web_fetch", DisplayName: "Web 抓取", Description: "抓取 URL 并提取页面文本或 Markdown。", Category: "web", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"url":{"type":"string"},"extract_mode":{"type":"string","enum":["markdown","text","json"]}},"required":["url"]}`},
	{ID: "tool_read_file", Key: "read_file", DisplayName: "读取文件", Description: "读取工作区允许路径内的文件内容。", Category: "filesystem", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_write_file", Key: "write_file", DisplayName: "写入文件", Description: "创建或覆盖工作区文件。", Category: "filesystem", RiskLevel: "medium", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"deliver":{"type":"boolean"}},"required":["path","content"]}`},
	{ID: "tool_list_files", Key: "list_files", DisplayName: "文件列表", Description: "列出工作区目录内容。", Category: "filesystem", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}}}`},
	{ID: "tool_edit_file", Key: "edit_file", DisplayName: "编辑文件", Description: "按精确匹配修改已有文件片段。", Category: "filesystem", RiskLevel: "medium", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"]}`},
	{ID: "tool_skill_search", Key: "skill_search", DisplayName: "Skill 搜索", Description: "搜索当前系统可用 Skill。", Category: "skill", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{ID: "tool_use_skill", Key: "use_skill", DisplayName: "使用 Skill", Description: "标记本次运行使用某个 Skill，用于追踪。", Category: "skill", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{ID: "tool_memory_search", Key: "memory_search", DisplayName: "Memory 搜索", Description: "搜索 Agent 长期记忆。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{ID: "tool_memory_get", Key: "memory_get", DisplayName: "Memory 读取", Description: "读取指定 memory 内容。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{ID: "tool_read_image", Key: "read_image", DisplayName: "图片理解", Description: "分析图片内容。", Category: "media", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_read_document", Key: "read_document", DisplayName: "文档理解", Description: "分析 PDF、Office、CSV 等文档。", Category: "media", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{ID: "tool_create_image", Key: "create_image", DisplayName: "图片生成", Description: "根据文本提示生成图片。", Category: "media", RiskLevel: "medium", Enabled: false, ParametersSchemaJSON: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{ID: "tool_tts", Key: "tts", DisplayName: "文本转语音", Description: "将文本转换成语音文件。", Category: "media", RiskLevel: "medium", Enabled: false, ParametersSchemaJSON: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{ID: "tool_shell_exec", Key: "shell_exec", DisplayName: "Shell 命令", Description: "执行本地 shell 命令。", Category: "runtime", RiskLevel: "critical", Enabled: false, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","properties":{"command":{"type":"string"},"working_dir":{"type":"string"}},"required":["command"]}`},
	{ID: "tool_working_memory_read", Key: "working_memory.read", DisplayName: "工作记忆读取", Description: "读取当前任务的结构化工作记忆字段。不传 field_path 时返回整张快照。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string","description":"字段路径（可选）"}}}`},
	{ID: "tool_working_memory_list", Key: "working_memory.list", DisplayName: "工作记忆列表", Description: "列出当前任务下所有可见字段（path、preview、token_estimate、revision）。", Category: "memory", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"include_internal":{"type":"boolean","description":"是否包含 visibility=internal 字段"}}}`},
	{ID: "tool_working_memory_write", Key: "working_memory.write", DisplayName: "工作记忆写入", Description: "向当前任务写入或更新一个结构化字段。超过单字段 / 任务上限时返回 OVERFLOW，调用方应先精简或归档。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string"},"value":{"description":"任意 JSON 值"},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown"]},"visibility":{"type":"string","enum":["prompt","internal","shared"]},"pin_to_prompt":{"type":"boolean"},"reason":{"type":"string"},"if_revision":{"type":"integer","description":"乐观锁，期望的当前 revision"}},"required":["field_path","value"]}`},
	{ID: "tool_working_memory_patch", Key: "working_memory.patch", DisplayName: "工作记忆批量补丁", Description: "一次写入多个字段。任意一项失败将中断后续写入并返回错误。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"patches":{"type":"array","items":{"type":"object","properties":{"field_path":{"type":"string"},"value":{},"field_kind":{"type":"string"},"visibility":{"type":"string"},"reason":{"type":"string"},"if_revision":{"type":"integer"}},"required":["field_path","value"]}}},"required":["patches"]}`},
	{ID: "tool_working_memory_delete", Key: "working_memory.delete", DisplayName: "工作记忆删除", Description: "删除当前任务下的一个字段（历史版本仍可回滚）。", Category: "memory", Enabled: true, ParametersSchemaJSON: `{"type":"object","properties":{"field_path":{"type":"string"}},"required":["field_path"]}`},
}

// cliAdminToolSeeds 枚举 `cli_admin_*` 工具集（aranea/docs/25 cli.md §6.2）。
var cliAdminToolSeeds = []domain.Tool{
	{
		Key: "cli_admin_skill_list", DisplayName: "Skill 列表", Description: "搜索 / 分页查询 Skill。",
		Category: "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"enabled":{"type":"string","enum":["true","false"]},"limit":{"type":"integer","default":20},"offset":{"type":"integer","default":0}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/skills"),
	},
	{
		Key:         "cli_admin_skill_install_from_url",
		DisplayName: "Skill 远程安装",
		Description: "从 GitHub / GitLab / 远程 zip URL 拉取 Skill，本地预校验后调用 import + apply 入库。",
		Category:    "system",
		RiskLevel:   "high",
		Enabled:     true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["url"],"properties":{"url":{"type":"string","format":"uri"},"ref":{"type":"string"},"subpath":{"type":"string"},"name":{"type":"string"},"enable":{"type":"boolean","default":false},"publish":{"type":"boolean","default":false},"decision":{"type":"string","enum":["ask","skip","keep","refine"],"default":"ask"},"refine_provider":{"type":"string"},"refine_model":{"type":"string"},"refine_instructions":{"type":"string"},"dry_run":{"type":"boolean","default":false},"idempotency_key":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import"),
	},
	{
		Key:         "cli_admin_skill_install_from_path",
		DisplayName: "Skill 本地 zip 安装",
		Description: "上传本地 zip 包到 /api/v1/skills/import 并完成同样的轮询 + apply 流程。",
		Category:    "system",
		RiskLevel:   "high",
		Enabled:     true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"name":{"type":"string"},"enable":{"type":"boolean","default":false},"publish":{"type":"boolean","default":false},"decision":{"type":"string","enum":["ask","skip","keep","refine"],"default":"ask"},"dry_run":{"type":"boolean","default":false},"idempotency_key":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import"),
	},
	{
		Key: "cli_admin_skill_import_status", DisplayName: "Skill 导入状态",
		Description: "查询导入任务的最新状态、候选与冲突组。",
		Category:    "system", RiskLevel: "low", Enabled: true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id"],"properties":{"job_id":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("low", "GET", "/api/v1/skills/import/{job_id}"),
	},
	{
		Key:         "cli_admin_skill_import_apply",
		DisplayName: "Skill 导入应用",
		Description: "把决策（create / skip / keep_existing / keep_incoming / merge）发给 /apply 入库。",
		Category:    "system",
		RiskLevel:   "high",
		Enabled:     true, Readonly: true, RequiresConfirmation: true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id","decisions"],"properties":{"job_id":{"type":"string"},"decisions":{"type":"array","items":{"type":"object"}}}}`,
		MetadataJSON:         cliAdminMeta("high", "POST", "/api/v1/skills/import/{job_id}/apply"),
	},
	{
		Key:         "cli_admin_skill_refine_conflict",
		DisplayName: "Skill 冲突 AI 炼化",
		Description: "用 AI 把一个冲突组中的多份 Skill 合并成一个新的 candidate。",
		Category:    "system",
		RiskLevel:   "medium",
		Enabled:     true, Readonly: true,
		ParametersSchemaJSON: `{"type":"object","required":["job_id","group_id"],"properties":{"job_id":{"type":"string"},"group_id":{"type":"string"},"instructions":{"type":"string"},"provider":{"type":"string"},"model":{"type":"string"}}}`,
		MetadataJSON:         cliAdminMeta("medium", "POST", "/api/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine"),
	},
	{Key: "cli_admin_skill_enable", DisplayName: "Skill 启用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/skills/{id}/enabled")},
	{Key: "cli_admin_skill_disable", DisplayName: "Skill 停用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/skills/{id}/enabled")},
	{Key: "cli_admin_skill_delete", DisplayName: "Skill 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/skills/{id}")},
	{Key: "cli_admin_agent_list", DisplayName: "Agent 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"status":{"type":"string"},"provider":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/agents")},
	{Key: "cli_admin_agent_get", DisplayName: "Agent 详情", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string","description":"agent id 或 agent_key"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/agents/{id}")},
	{Key: "cli_admin_agent_create", DisplayName: "Agent 创建", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["agent_key","display_name"],"properties":{"agent_key":{"type":"string"},"display_name":{"type":"string"},"provider":{"type":"string"},"model":{"type":"string"},"agent_description":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/agents")},
	{Key: "cli_admin_agent_update", DisplayName: "Agent 更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/agents/{id}")},
	{Key: "cli_admin_agent_delete", DisplayName: "Agent 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/agents/{id}")},
	{Key: "cli_admin_agent_tools_get", DisplayName: "Agent 工具策略读取", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["agent_id"],"properties":{"agent_id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/agents/{id}/tools/effective")},
	{Key: "cli_admin_agent_tools_set", DisplayName: "Agent 工具策略修改", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["agent_id"],"properties":{"agent_id":{"type":"string"},"tools_enabled":{"type":"boolean"},"tools_profile":{"type":"string"},"tools_allow":{"type":"array","items":{"type":"string"}},"tools_deny":{"type":"array","items":{"type":"string"}},"tools_concurrent_allow":{"type":"array","items":{"type":"string"}},"dry_run":{"type":"boolean","default":false}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/agents/{id}/tools/policy")},
	{Key: "cli_admin_team_list", DisplayName: "Team 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/teams")},
	{Key: "cli_admin_team_create", DisplayName: "Team 创建", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["team_key","display_name"],"properties":{"team_key":{"type":"string"},"display_name":{"type":"string"},"agents":{"type":"array","items":{"type":"string"}}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/teams")},
	{Key: "cli_admin_team_update", DisplayName: "Team 更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/teams/{id}")},
	{Key: "cli_admin_team_delete", DisplayName: "Team 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/teams/{id}")},
	{Key: "cli_admin_team_run", DisplayName: "Team 触发执行", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["team_id","message"],"properties":{"team_id":{"type":"string"},"message":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/team-runs")},
	{Key: "cli_admin_tool_list", DisplayName: "Tool 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"},"category":{"type":"string"},"enabled":{"type":"string","enum":["true","false"]},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/tools")},
	{Key: "cli_admin_tool_enable", DisplayName: "Tool 启用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/enabled")},
	{Key: "cli_admin_tool_disable", DisplayName: "Tool 停用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/enabled")},
	{Key: "cli_admin_tool_config_set", DisplayName: "Tool 配置写入", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id","config"],"properties":{"id":{"type":"string"},"config":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/tools/{id}/config")},
	{Key: "cli_admin_plugin_list", DisplayName: "Plugin 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/plugins")},
	{Key: "cli_admin_plugin_enable", DisplayName: "Plugin 启用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}")},
	{Key: "cli_admin_plugin_disable", DisplayName: "Plugin 停用", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}")},
	{Key: "cli_admin_plugin_order_set", DisplayName: "Plugin 排序", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["entries"],"properties":{"entries":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"sort_order":{"type":"integer"}},"required":["id","sort_order"]}}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/plugins")},
	{Key: "cli_admin_plugin_config_set", DisplayName: "Plugin 配置写入", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id","config"],"properties":{"id":{"type":"string"},"config":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/plugins/{id}")},
	{Key: "cli_admin_mcp_list", DisplayName: "MCP Server 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/mcp-servers")},
	{Key: "cli_admin_mcp_add", DisplayName: "MCP Server 新增", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["name","transport"],"properties":{"name":{"type":"string"},"transport":{"type":"string","enum":["streamable_http","stdio","sse"]},"url":{"type":"string"},"headers":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/mcp-servers")},
	{Key: "cli_admin_mcp_update", DisplayName: "MCP Server 更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/mcp-servers/{id}")},
	{Key: "cli_admin_mcp_delete", DisplayName: "MCP Server 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/mcp-servers/{id}")},
	{Key: "cli_admin_mcp_test", DisplayName: "MCP Server 连通性测试", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "POST", "/api/v1/mcp-servers/{id}/test")},
	{Key: "cli_admin_cron_list", DisplayName: "Cron 任务列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/cron-tasks")},
	{Key: "cli_admin_cron_add", DisplayName: "Cron 任务新增", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["name","schedule_expr"],"properties":{"name":{"type":"string"},"schedule_expr":{"type":"string"},"agent_key":{"type":"string"},"message":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/cron-tasks")},
	{Key: "cli_admin_cron_update", DisplayName: "Cron 任务更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}")},
	{Key: "cli_admin_cron_delete", DisplayName: "Cron 任务删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/cron-tasks/{id}")},
	{Key: "cli_admin_cron_pause", DisplayName: "Cron 任务暂停", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}")},
	{Key: "cli_admin_cron_resume", DisplayName: "Cron 任务恢复", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/cron-tasks/{id}")},
	{Key: "cli_admin_cron_trigger", DisplayName: "Cron 任务立即触发", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/cron-tasks/{id}/trigger")},
	{Key: "cli_admin_channel_list", DisplayName: "Channel 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/channels")},
	{Key: "cli_admin_channel_add", DisplayName: "Channel 新增", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["name","kind"],"properties":{"name":{"type":"string"},"kind":{"type":"string"},"config":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("high", "POST", "/api/v1/channels")},
	{Key: "cli_admin_channel_update", DisplayName: "Channel 更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/channels/{id}")},
	{Key: "cli_admin_channel_delete", DisplayName: "Channel 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/channels/{id}")},
	{Key: "cli_admin_channel_test", DisplayName: "Channel 连通性测试", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "POST", "/api/v1/channels/{id}/test")},
	{Key: "cli_admin_channel_send", DisplayName: "Channel 推送消息", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id","text"],"properties":{"id":{"type":"string"},"text":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "POST", "/api/v1/channels/{id}/send")},
	{Key: "cli_admin_provider_list", DisplayName: "LLM Provider 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"keyword":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/llm-provider-models")},
	{Key: "cli_admin_provider_add", DisplayName: "LLM Provider 新增", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["provider","model"],"properties":{"provider":{"type":"string"},"model":{"type":"string"},"name":{"type":"string"},"config":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/llm-provider-models")},
	{Key: "cli_admin_provider_update", DisplayName: "LLM Provider 更新", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"patch":{"type":"object"}}}`, MetadataJSON: cliAdminMeta("medium", "PATCH", "/api/v1/llm-provider-models/{id}")},
	{Key: "cli_admin_provider_delete", DisplayName: "LLM Provider 删除", Category: "system", RiskLevel: "high", Enabled: true, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "DELETE", "/api/v1/llm-provider-models/{id}")},
	{Key: "cli_admin_provider_inspect", DisplayName: "LLM Provider 自检", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "POST", "/api/v1/llm-provider-models/inspect")},
	{Key: "cli_admin_session_list", DisplayName: "Session 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"limit":{"type":"integer"},"offset":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/sessions")},
	{Key: "cli_admin_session_get", DisplayName: "Session 详情", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/sessions/{id}")},
	{Key: "cli_admin_session_send", DisplayName: "Session 发送消息", Category: "system", RiskLevel: "medium", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","required":["agent_key","content"],"properties":{"session_id":{"type":"string"},"agent_key":{"type":"string"},"content":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("medium", "POST", "/api/v1/chat/messages")},
	{Key: "cli_admin_monitor_audit", DisplayName: "审计日志查询", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"kind":{"type":"string"},"actor":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/monitor/audit")},
	{Key: "cli_admin_monitor_events", DisplayName: "运行事件查询", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/monitor/events")},
	{Key: "cli_admin_monitor_traces", DisplayName: "Trace 列表", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"agent_id":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},"limit":{"type":"integer"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/monitor/traces")},
	{Key: "cli_admin_monitor_usage_overview", DisplayName: "Token 用量概览", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("low", "GET", "/api/v1/model-usage/overview")},
	{Key: "cli_admin_system_health", DisplayName: "后端健康检查", Category: "system", RiskLevel: "low", Enabled: true, Readonly: true, ParametersSchemaJSON: `{"type":"object","properties":{}}`, MetadataJSON: cliAdminMeta("low", "GET", "/healthz")},
	{Key: "cli_admin_system_backup", DisplayName: "系统备份", Category: "system", RiskLevel: "high", Enabled: false, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","properties":{"include_skills":{"type":"boolean","default":true}}}`, MetadataJSON: cliAdminMeta("high", "POST", "/api/v1/system/backup")},
	{Key: "cli_admin_system_restore", DisplayName: "系统恢复", Category: "system", RiskLevel: "high", Enabled: false, Readonly: true, RequiresConfirmation: true, ParametersSchemaJSON: `{"type":"object","required":["snapshot_id"],"properties":{"snapshot_id":{"type":"string"}}}`, MetadataJSON: cliAdminMeta("high", "POST", "/api/v1/system/restore")},
}
