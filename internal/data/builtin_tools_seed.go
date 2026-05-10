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
	{key: "web_search", displayName: "Web 搜索", description: "搜索实时网络信息，返回标题、链接和摘要。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"},"limit":{"type":"number","description":"返回结果数量"}},"required":["query"]}`},
	{key: "web_fetch", displayName: "Web 抓取", description: "抓取 URL 并提取页面文本或 Markdown。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string"},"extract_mode":{"type":"string","enum":["markdown","text","json"]}},"required":["url"]}`},
	{key: "read_file", displayName: "读取文件", description: "读取工作区允许路径内的文件内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "write_file", displayName: "写入文件", description: "创建或覆盖工作区文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"deliver":{"type":"boolean"}},"required":["path","content"]}`},
	{key: "list_files", displayName: "文件列表", description: "列出工作区目录内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}}}`},
	{key: "workspace_search", displayName: "工作区搜索", description: "在工作区内按字面或正则搜索文本，结果带路径与行号。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"},"mode":{"type":"string"},"path_prefix":{"type":"string"},"glob":{"type":"string"},"max_results":{"type":"number"}},"required":["query"]}`},
	{key: "edit_file", displayName: "编辑文件", description: "按精确匹配修改已有文件片段。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}},"required":["path","old_string","new_string"]}`},
	{key: "skill_search", displayName: "Skill 搜索", description: "搜索当前系统可用 Skill。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "use_skill", displayName: "使用 Skill", description: "标记本次运行使用某个 Skill，用于追踪。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{key: "memory_search", displayName: "Memory 搜索", description: "搜索 Agent 长期记忆。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "memory_get", displayName: "Memory 读取", description: "读取指定 memory 内容。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{key: "read_image", displayName: "图片理解", description: "分析图片内容。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "read_document", displayName: "文档理解", description: "分析 PDF、Office、CSV 等文档。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "create_image", displayName: "图片生成", description: "根据文本提示生成图片。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{key: "tts", displayName: "文本转语音", description: "将文本转换成语音文件。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{key: "shell_exec", displayName: "Shell 命令", description: "执行本地 shell 命令。", category: "runtime", riskLevel: "critical", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"},"working_dir":{"type":"string"}},"required":["command"]}`},
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
