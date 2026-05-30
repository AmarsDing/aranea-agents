package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/event"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/tools"
)

type platformToolSeed struct {
	key          string
	displayName  string
	description  string
	category     string
	source       string
	riskLevel    string
	enabled      bool
	readonly     bool
	reqConfirm   bool
	suppStream   bool
	suppConc     bool
	paramsSchema string
	configSchema string
	registryName string
}

const webResearchConfigSchema = `{"type":"object","properties":{"provider":{"type":"string","enum":["tavily","serpapi"],"description":"Search API provider"},"api_key":{"type":"string","description":"Provider API key (optional if set in system settings)"},"search_depth":{"type":"string","enum":["basic","advanced"],"description":"Tavily search depth"},"max_results":{"type":"integer","minimum":1,"maximum":20},"fetch_top":{"type":"integer","minimum":0,"maximum":20,"description":"URLs to fetch for full page excerpts"},"timeout_sec":{"type":"integer","minimum":5,"maximum":120},"http_proxy":{"type":"string","description":"HTTP proxy URL for search and fetch"}}}`

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func applyPlatformToolDefaults(s *platformToolSeed) {
	if s.source == "" {
		s.source = "builtin"
	}
	if s.riskLevel == "" {
		s.riskLevel = "low"
	}
	if s.paramsSchema == "" {
		s.paramsSchema = "{}"
	}
}

var builtinPlatformToolSeeds = []platformToolSeed{
	{key: "datetime", displayName: "当前时间", description: "返回当前时间、日期和时区信息。", category: "system", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "web_research", displayName: "Web 研究", description: "使用 Tavily 或 SerpAPI 搜索网络并返回多源摘要与正文片段。API Key 优先 Agent 工具配置，否则使用系统设置（设置 → Web 研究）或环境变量 TAVILY_API_KEY。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"自然语言搜索问题"}},"required":["query"]}`, configSchema: webResearchConfigSchema, registryName: ""},
	{key: "duckduckgo_search", displayName: "DuckDuckGo 搜索", description: "DuckDuckGo Instant Answer（百科/定义类查询；非通用网页搜索）。", category: "web", riskLevel: "medium", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "duckduckgo"},
	{key: "web_fetch", displayName: "Web 抓取", description: "并行抓取多个 URL 并提取 Markdown/文本。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"urls":{"type":"array","items":{"type":"string"},"description":"要抓取的 URL 列表"}},"required":["urls"]}`, registryName: "httpfetch"},
	{key: "gemini_web_fetch", displayName: "Gemini 抓取", description: "使用 Gemini 模型抓取并理解 URL 内容。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string","description":"要抓取的 URL"}},"required":["url"]}`, registryName: "geminifetch"},
	{key: "google_search", displayName: "Google 搜索", description: "使用 Google Custom Search API 搜索网络信息。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "google_search"},
	{key: "arxiv_search", displayName: "ArXiv 搜索", description: "搜索 ArXiv 学术论文。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"max_results":{"type":"number","description":"最大返回结果数"}},"required":["query"]}`, registryName: "arxiv_search"},
	{key: "wikipedia_search", displayName: "Wikipedia 查询", description: "搜索和获取 Wikipedia 百科内容。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "wikipedia"},
	{key: "read_file", displayName: "读取文件", description: "读取工作区允许路径内的文件内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"start_line":{"type":"integer","description":"可选，1-based 起始行"},"num_lines":{"type":"integer","description":"可选，最大行数"}},"required":["file_name"]}`, registryName: "file"},
	{key: "read_multiple_files", displayName: "批量读取文件", description: "一次读取多个文件的内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"},"description":"文件路径列表"}},"required":["paths"]}`, registryName: "file"},
	{key: "save_file", displayName: "保存文件", description: "创建或覆盖工作区文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"文件路径"},"content":{"type":"string","description":"文件内容"}},"required":["path","content"]}`, registryName: "file"},
	{key: "list_file", displayName: "文件列表", description: "列出工作区目录内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"目录路径"},"pattern":{"type":"string","description":"glob 匹配模式"}}}`, registryName: "file"},
	{key: "search_file", displayName: "文件搜索", description: "按文件名模式搜索工作区文件。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"pattern":{"type":"string","description":"glob 匹配模式"},"path":{"type":"string","description":"搜索根目录"}},"required":["pattern"]}`, registryName: "file"},
	{key: "search_content", displayName: "内容搜索", description: "在工作区内搜索文本内容，结果带路径与行号。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"path":{"type":"string","description":"搜索根目录"}},"required":["query"]}`, registryName: "file"},
	{key: "replace_content", displayName: "替换内容", description: "按精确匹配替换文件中的文本片段。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"old_string":{"type":"string","description":"要替换的原始文本"},"new_string":{"type":"string","description":"替换后的文本"},"num_replacements":{"type":"integer","description":"替换次数，0 表示 1，负数表示全部"}},"required":["file_name","old_string","new_string"]}`, registryName: "file"},
	{key: "diff_edit", displayName: "片段编辑", description: "对已有文件施加一处或多处 search/replace 片段替换，原子提交。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"edits":{"type":"array","items":{"type":"object","properties":{"search":{"type":"string"},"replace":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["search","replace"]}},"expected_mtime_ms":{"type":"integer","description":"可选，来自 read_file 的 mtime_ms"}},"required":["file_name","edits"]}`, registryName: "file"},
	{key: "patch_file", displayName: "应用补丁", description: "应用 unified diff 或结构化 hunk 列表修改文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"patch":{"type":"string","description":"Unified diff 文本"},"hunks":{"type":"array","description":"结构化 hunk，与 patch 二选一"},"expected_mtime_ms":{"type":"integer","description":"可选，来自 read_file 的 mtime_ms"}},"required":["file_name"]}`, registryName: "file"},
	{key: "skill_search", displayName: "Skill 搜索", description: "搜索当前系统可用 Skill。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "use_skill", displayName: "使用 Skill", description: "标记本次运行使用某个 Skill，用于追踪。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{key: "memory_search", displayName: "Memory 搜索", description: "搜索 Agent 长期记忆。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "memory_get", displayName: "Memory 读取", description: "读取指定 memory 内容。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{key: "read_image", displayName: "图片理解", description: "分析图片内容。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "read_document", displayName: "文档理解", description: "分析 PDF、Office、CSV 等文档。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "create_image", displayName: "图片生成", description: "根据文本提示生成图片。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{key: "tts", displayName: "文本转语音", description: "将文本转换成语音文件。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{key: "shell_exec", displayName: "Shell 命令", description: "执行本地 shell 命令。", category: "runtime", riskLevel: "critical", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"},"workdir":{"type":"string","description":"Optional working directory relative to workspace root"}},"required":["command"]}`, registryName: "hostexec"},
	{key: "send_email", displayName: "邮件发送", description: "发送电子邮件。", category: "messaging", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"to":{"type":"string"},"subject":{"type":"string"},"body":{"type":"string"}},"required":["to","subject","body"]}`, registryName: "email"},
	{key: "todo_write", displayName: "待办管理", description: "管理待办事项列表，支持创建、更新和追踪任务进度。", category: "system", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"content":{"type":"string"},"activeForm":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]}},"required":["content","activeForm","status"]}}},"required":["todos"]}`, registryName: "todo"},
	{key: "await_user_reply", displayName: "等待回复", description: "暂停执行并等待用户回复，用于多轮对话场景。", category: "session", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "await_user_reply"},
	{key: "kanban", displayName: "Kanban 任务板", description: "Graph 任务看板工具集（kanban_show/complete/block/heartbeat/comment/create/link）。Worker 需设置 ARANEA_TASK_ID。", category: "integration", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "claude_code", displayName: "Claude Code", description: "使用 Claude Code 进行代码编辑和执行，包含 Bash、Read、Write、Edit、Glob、Grep 等子工具。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "claudecode"},
	{key: "workspace_exec", displayName: "工作区执行", description: "在工作区中执行命令并管理会话。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`, registryName: "workspace_exec"},
	{key: "call_agent", displayName: "调用 Agent", description: "调用同工作区内已启用 A2A 的另一 Agent 能力。", category: "integration", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"agent_id":{"type":"string","description":"目标 Agent ID"},"capability":{"type":"string","description":"能力名称，如 chat"},"payload":{"description":"传递给目标 Agent 的 JSON 载荷"},"timeout_seconds":{"type":"integer","description":"超时秒数"}},"required":["agent_id","capability"]}`},
	{key: "knowledge_search", displayName: "知识库搜索", description: "在指定知识库集合中检索相关文本片段。", category: "integration", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"collection_id":{"type":"string"},"query":{"type":"string"},"top_k":{"type":"integer"},"min_score":{"type":"number"}},"required":["collection_id","query"]}`},
	{key: "knowledge_reflect", displayName: "知识库反思", description: "跨多个知识库检索并评估结果质量，判断是否需要补充查询。", category: "integration", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"collection_ids":{"type":"array","items":{"type":"string"}},"query":{"type":"string"},"top_k":{"type":"integer"}},"required":["collection_ids","query"]}`},
	{key: "mcp_tool_set", displayName: "MCP 工具集", description: "挂载已配置的 MCP Server 工具。", category: "integration", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "mcp_broker", displayName: "MCP Broker", description: "运行时 MCP 发现与调用（mcp_list_servers / mcp_call 等）。", category: "integration", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{}}`, registryName: "mcpbroker"},
	{key: "working_memory.read", displayName: "工作记忆读取", description: "读取当前任务的结构化工作记忆字段。不传 field_path 时返回整张快照。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string","description":"字段路径（可选）"}}}`},
	{key: "working_memory.list", displayName: "工作记忆列表", description: "列出当前任务下所有可见字段（path、preview、token_estimate、revision）。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"include_internal":{"type":"boolean","description":"是否包含 visibility=internal 字段"}}}`},
	{key: "working_memory.write", displayName: "工作记忆写入", description: "向当前任务写入或更新一个结构化字段。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"},"value":{"description":"任意 JSON 值"},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown"]},"visibility":{"type":"string","enum":["prompt","internal","shared"]},"pin_to_prompt":{"type":"boolean"},"reason":{"type":"string"},"if_revision":{"type":"integer","description":"乐观锁，期望的当前 revision"}},"required":["field_path","value"]}`},
	{key: "working_memory.patch", displayName: "工作记忆批量补丁", description: "一次写入多个字段。任意一项失败将中断后续写入并返回错误。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"patches":{"type":"array","items":{"type":"object","properties":{"field_path":{"type":"string"},"value":{},"field_kind":{"type":"string"},"visibility":{"type":"string"},"reason":{"type":"string"},"if_revision":{"type":"integer"}},"required":["field_path","value"]}}},"required":["patches"]}`},
	{key: "working_memory.delete", displayName: "工作记忆删除", description: "删除当前任务下的一个字段（历史版本仍可回滚）。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"}},"required":["field_path"]}`},
	{key: "model_registry_sync", displayName: "模型目录同步", description: "模型目录同步工具集（fetch_model_directory / migrate_provider_bindings / apply_model_directory / sync_provider_logos）。", category: "system", riskLevel: "medium", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "model_registry_sync"},
}

func ensureBuiltinPlatformTools(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO tools (
			id, tool_key, display_name, description, category, source, risk_level, enabled, readonly, requires_confirmation,
			supports_streaming, supports_concurrency, parameters_schema_json, result_schema_json, config_schema_json, config_json,
			default_config_json, metadata_json, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(tool_key) DO NOTHING`
	for _, row := range builtinPlatformToolSeeds {
		applyPlatformToolDefaults(&row)
		id := "tool_" + strings.ReplaceAll(row.key, ".", "_")
		configSchema := strings.TrimSpace(row.configSchema)
		if configSchema == "" {
			configSchema = "{}"
		}
		_, err := client.ExecContext(ctx, q,
			id, row.key, row.displayName, row.description, row.category, row.source, row.riskLevel,
			b2i(row.enabled), b2i(row.readonly), b2i(row.reqConfirm),
			b2i(row.suppStream), b2i(row.suppConc),
			row.paramsSchema, "{}", configSchema, "{}", "{}", "{}",
			now, now,
		)
		if err != nil {
			return fmt.Errorf("ensure builtin tools: seed %q: %w", row.key, err)
		}
	}
	if err := syncBuiltinToolsFromRegistry(ctx, client); err != nil {
		event.SysLogWarn("system.data.builtin_tool_sync", "内置工具批量同步失败", event.P("error", err))
	}
	if err := syncBuiltinWebToolCatalogPatches(ctx, client); err != nil {
		event.SysLogWarn("system.data.builtin_tool_sync", "内置 Web 工具元数据同步失败", event.P("error", err))
	}
	return nil
}

// syncBuiltinWebToolCatalogPatches updates catalog metadata for existing DBs (seed uses ON CONFLICT DO NOTHING).
func syncBuiltinWebToolCatalogPatches(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const upd = `UPDATE tools SET
		enabled = ?, description = ?, parameters_schema_json = ?, config_schema_json = ?, updated_at = ?
		WHERE tool_key = ? AND source = 'builtin' AND deleted_at = ''`
	patches := []struct {
		key, desc, params, config string
		enabled                   int
	}{
		{
			key:     "duckduckgo_search",
			enabled: 0,
			desc:    "DuckDuckGo Instant Answer（百科/定义类查询；非通用网页搜索）。",
			params:  `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`,
			config:  "{}",
		},
		{
			key:     "web_research",
			enabled: 1,
			desc:    "使用 Tavily 或 SerpAPI 搜索网络并返回多源摘要与正文片段。API Key 优先 Agent 工具配置，否则使用系统设置（设置 → Web 研究）或环境变量 TAVILY_API_KEY。",
			params:  `{"type":"object","properties":{"query":{"type":"string","description":"自然语言搜索问题"}},"required":["query"]}`,
			config:  webResearchConfigSchema,
		},
		{
			key:     "web_fetch",
			enabled: 1,
			desc:    "并行抓取多个 URL 并提取 Markdown/文本。",
			params:  `{"type":"object","properties":{"urls":{"type":"array","items":{"type":"string"},"description":"要抓取的 URL 列表"}},"required":["urls"]}`,
			config:  "{}",
		},
	}
	for _, p := range patches {
		if _, err := client.ExecContext(ctx, upd, p.enabled, p.desc, p.params, p.config, now, p.key); err != nil {
			return fmt.Errorf("sync web tool %q: %w", p.key, err)
		}
	}
	return nil
}

func syncBuiltinToolsFromRegistry(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}
	regMap := make(map[string]*tools.ToolRegistration, len(tools.Registry()))
	for i := range tools.Registry() {
		regMap[tools.Registry()[i].Name] = tools.Registry()[i]
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const upd = `UPDATE tools SET
		risk_level = ?, requires_confirmation = ?, supports_streaming = ?, supports_concurrency = ?,
		description = COALESCE(NULLIF(?, ''), description),
		parameters_schema_json = COALESCE(NULLIF(?, ''), parameters_schema_json),
		updated_at = ?
		WHERE tool_key = ? AND source = 'builtin' AND deleted_at = ''`
	for _, seed := range builtinPlatformToolSeeds {
		regName := strings.TrimSpace(seed.registryName)
		if regName == "" {
			continue
		}
		reg, ok := regMap[regName]
		if !ok {
			continue
		}
		riskLevel := strings.TrimSpace(reg.RiskLevel)
		if riskLevel == "" {
			riskLevel = seed.riskLevel
		}
		if riskLevel == "" {
			riskLevel = "low"
		}
		reqConfirm := reg.RequiresConfirmation
		suppStream := reg.SupportsStreaming
		suppConc := reg.SupportsConcurrency
		_, err := client.ExecContext(ctx, upd,
			riskLevel, b2i(reqConfirm), b2i(suppStream), b2i(suppConc),
			seed.description, seed.paramsSchema,
			now, seed.key,
		)
		if err != nil {
			event.SysLogWarn("system.data.builtin_tool_sync", "内置工具同步失败", event.P("tool_key", seed.key), event.P("registry_name", regName), event.P("error", err))
		}
	}
	return nil
}
