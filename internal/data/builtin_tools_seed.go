package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
)

// platformToolSeed matches rows for the legacy `tools` table (sessionmemory schema).
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
}

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

// builtinPlatformToolSeeds aligns with pkg/backend capability sqlite seeds (non-cli_admin subset).
var builtinPlatformToolSeeds = []platformToolSeed{
	{key: "datetime", displayName: "当前时间", description: "返回当前时间、日期和时区信息。", category: "system", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "duckduckgo_search", displayName: "DuckDuckGo 搜索", description: "使用 DuckDuckGo 搜索实时网络信息，返回标题、链接和摘要。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`},
	{key: "web_fetch", displayName: "Web 抓取", description: "抓取 URL 并提取页面文本或 Markdown。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string","description":"要抓取的 URL"}},"required":["url"]}`},
	{key: "gemini_web_fetch", displayName: "Gemini 抓取", description: "使用 Gemini 模型抓取并理解 URL 内容。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string","description":"要抓取的 URL"}},"required":["url"]}`},
	{key: "google_search", displayName: "Google 搜索", description: "使用 Google Custom Search API 搜索网络信息。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`},
	{key: "arxiv_search", displayName: "ArXiv 搜索", description: "搜索 ArXiv 学术论文。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"max_results":{"type":"number","description":"最大返回结果数"}},"required":["query"]}`},
	{key: "wikipedia_search", displayName: "Wikipedia 查询", description: "搜索和获取 Wikipedia 百科内容。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`},
	{key: "read_file", displayName: "读取文件", description: "读取工作区允许路径内的文件内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"文件路径"}},"required":["path"]}`},
	{key: "read_multiple_files", displayName: "批量读取文件", description: "一次读取多个文件的内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"},"description":"文件路径列表"}},"required":["paths"]}`},
	{key: "save_file", displayName: "保存文件", description: "创建或覆盖工作区文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"文件路径"},"content":{"type":"string","description":"文件内容"}},"required":["path","content"]}`},
	{key: "list_file", displayName: "文件列表", description: "列出工作区目录内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"目录路径"},"pattern":{"type":"string","description":"glob 匹配模式"}}}`},
	{key: "search_file", displayName: "文件搜索", description: "按文件名模式搜索工作区文件。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"pattern":{"type":"string","description":"glob 匹配模式"},"path":{"type":"string","description":"搜索根目录"}},"required":["pattern"]}`},
	{key: "search_content", displayName: "内容搜索", description: "在工作区内搜索文本内容，结果带路径与行号。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"path":{"type":"string","description":"搜索根目录"}},"required":["query"]}`},
	{key: "replace_content", displayName: "替换内容", description: "按精确匹配替换文件中的文本片段。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"文件路径"},"old_text":{"type":"string","description":"要替换的原始文本"},"new_text":{"type":"string","description":"替换后的文本"}},"required":["path","old_text","new_text"]}`},
	{key: "skill_search", displayName: "Skill 搜索", description: "搜索当前系统可用 Skill。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "use_skill", displayName: "使用 Skill", description: "标记本次运行使用某个 Skill，用于追踪。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{key: "memory_search", displayName: "Memory 搜索", description: "搜索 Agent 长期记忆。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "memory_get", displayName: "Memory 读取", description: "读取指定 memory 内容。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{key: "read_image", displayName: "图片理解", description: "分析图片内容。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "read_document", displayName: "文档理解", description: "分析 PDF、Office、CSV 等文档。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "create_image", displayName: "图片生成", description: "根据文本提示生成图片。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{key: "tts", displayName: "文本转语音", description: "将文本转换成语音文件。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{key: "shell_exec", displayName: "Shell 命令", description: "执行本地 shell 命令。", category: "runtime", riskLevel: "critical", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"},"working_dir":{"type":"string"}},"required":["command"]}`},
	{key: "send_email", displayName: "邮件发送", description: "发送电子邮件。", category: "messaging", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"to":{"type":"string"},"subject":{"type":"string"},"body":{"type":"string"}},"required":["to","subject","body"]}`},
	{key: "todo_write", displayName: "待办管理", description: "管理待办事项列表，支持创建、更新和追踪任务进度。", category: "system", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"content":{"type":"string"},"activeForm":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]"}},"required":["content","activeForm","status"]}}},"required":["todos"]}`},
	{key: "await_user_reply", displayName: "等待回复", description: "暂停执行并等待用户回复，用于多轮对话场景。", category: "session", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "claude_code", displayName: "Claude Code", description: "使用 Claude Code 进行代码编辑和执行，包含 Bash、Read、Write、Edit、Glob、Grep 等子工具。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "workspace_exec", displayName: "工作区执行", description: "在工作区中执行命令并管理会话。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`},
	{key: "working_memory.read", displayName: "工作记忆读取", description: "读取当前任务的结构化工作记忆字段。不传 field_path 时返回整张快照。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string","description":"字段路径（可选）"}}}`},
	{key: "working_memory.list", displayName: "工作记忆列表", description: "列出当前任务下所有可见字段（path、preview、token_estimate、revision）。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"include_internal":{"type":"boolean","description":"是否包含 visibility=internal 字段"}}}`},
	{key: "working_memory.write", displayName: "工作记忆写入", description: "向当前任务写入或更新一个结构化字段。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"},"value":{"description":"任意 JSON 值"},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown"]},"visibility":{"type":"string","enum":["prompt","internal","shared"]},"pin_to_prompt":{"type":"boolean"},"reason":{"type":"string"},"if_revision":{"type":"integer","description":"乐观锁，期望的当前 revision"}},"required":["field_path","value"]}`},
	{key: "working_memory.patch", displayName: "工作记忆批量补丁", description: "一次写入多个字段。任意一项失败将中断后续写入并返回错误。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"patches":{"type":"array","items":{"type":"object","properties":{"field_path":{"type":"string"},"value":{},"field_kind":{"type":"string"},"visibility":{"type":"string"},"reason":{"type":"string"},"if_revision":{"type":"integer"}},"required":["field_path","value"]}}},"required":["patches"]}`},
	{key: "working_memory.delete", displayName: "工作记忆删除", description: "删除当前任务下的一个字段（历史版本仍可回滚）。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"}},"required":["field_path"]}`},
}

// ensureBuiltinPlatformTools inserts default capability catalog rows if missing.
// Without this, GetEffectiveTools returns an empty matrix and native chat never advertises OpenAI tools.
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
		_, err := client.ExecContext(ctx, q,
			id, row.key, row.displayName, row.description, row.category, row.source, row.riskLevel,
			b2i(row.enabled), b2i(row.readonly), b2i(row.reqConfirm),
			b2i(row.suppStream), b2i(row.suppConc),
			row.paramsSchema, "{}", "{}", "{}", "{}", "{}",
			now, now,
		)
		if err != nil {
			return fmt.Errorf("ensure builtin tools: seed %q: %w", row.key, err)
		}
	}
	return nil
}
