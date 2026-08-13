package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
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

const webResearchConfigSchema = `{"type":"object","properties":{"provider":{"type":"string","enum":["tavily","serpapi"],"description":"Search API provider"},"api_key":{"type":"string","format":"password","description":"Provider API key (optional if set in system settings)"},"search_depth":{"type":"string","enum":["basic","advanced"],"description":"Tavily search depth"},"max_results":{"type":"integer","minimum":1,"maximum":20},"fetch_top":{"type":"integer","minimum":0,"maximum":20,"description":"URLs to fetch for full page excerpts"},"timeout_sec":{"type":"integer","minimum":5,"maximum":120},"http_proxy":{"type":"string","description":"HTTP proxy URL for search and fetch"}}}`

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
	{key: "datetime", displayName: "当前时间", description: "返回当前时间、日期和时区信息。", category: "system", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "datetime"},
	{key: "web_research", displayName: "Web 研究", description: "使用 Tavily 或 SerpAPI 搜索网络并返回多源摘要与正文片段。API Key 优先 Agent 工具配置，否则使用系统设置（设置 → Web 研究）或环境变量 TAVILY_API_KEY。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"自然语言搜索问题"}},"required":["query"]}`, configSchema: webResearchConfigSchema, registryName: ""},
	{key: "duckduckgo_search", displayName: "DuckDuckGo 搜索", description: "DuckDuckGo Instant Answer（百科/定义类查询；非通用网页搜索）。", category: "web", riskLevel: "medium", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "duckduckgo"},
	{key: "web_fetch", displayName: "Web 抓取", description: "并行抓取多个 URL 并提取 Markdown/文本。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"urls":{"type":"array","items":{"type":"string"},"description":"要抓取的 URL 列表"}},"required":["urls"]}`, registryName: "httpfetch"},
	{key: "gemini_web_fetch", displayName: "Gemini 抓取", description: "使用 Gemini 模型抓取并理解 URL 内容。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string","description":"要抓取的 URL"}},"required":["url"]}`, registryName: "geminifetch"},
	{key: "google_search", displayName: "Google 搜索", description: "使用 Google Custom Search API 搜索网络信息。", category: "web", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "google_search"},
	{key: "arxiv_search", displayName: "ArXiv 搜索", description: "搜索 ArXiv 学术论文。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"search":{"type":"object","description":"搜索参数","properties":{"query":{"type":"string","description":"搜索关键词"},"id_list":{"type":"array","items":{"type":"string"},"description":"arXiv ID 列表"},"max_results":{"type":"integer","description":"最大返回结果数"},"sort_by":{"type":"string","description":"排序依据","enum":["relevance","lastUpdatedDate","submittedDate"]},"sort_order":{"type":"string","description":"排序方向","enum":["ascending","descending"]}},"required":["query"]},"read_arxiv_papers":{"type":"boolean","description":"是否读取 PDF 内容"}},"required":["search"]}`, registryName: "arxiv_search"},
	{key: "wikipedia_search", displayName: "Wikipedia 查询", description: "搜索和获取 Wikipedia 百科内容。", category: "web", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string","description":"搜索关键词"}},"required":["query"]}`, registryName: "wikipedia"},
	{key: "read_file", displayName: "读取文件", description: "读取工作区允许路径内的文件内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"start_line":{"type":"integer","description":"可选，1-based 起始行"},"num_lines":{"type":"integer","description":"可选，最大行数"}},"required":["file_name"]}`, registryName: "file"},
	{key: "read_multiple_files", displayName: "批量读取文件", description: "一次读取多个文件的内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"patterns":{"type":"array","items":{"type":"string"},"description":"Glob 模式列表，如 *.go 或 workspace://out/*.txt"},"case_sensitive":{"type":"boolean","description":"Glob 匹配是否区分大小写"}},"required":["patterns"]}`, registryName: "file"},
	{key: "save_file", displayName: "保存文件", description: "创建或覆盖工作区文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"contents":{"type":"string","description":"文件内容"},"overwrite":{"type":"boolean","description":"是否覆盖已有文件"}},"required":["file_name","contents"]}`, registryName: "file"},
	{key: "list_file", displayName: "文件列表", description: "列出工作区目录内容。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string","description":"目录路径"},"pattern":{"type":"string","description":"glob 匹配模式"}}}`, registryName: "file"},
	{key: "search_file", displayName: "文件搜索", description: "按文件名模式搜索工作区文件。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"pattern":{"type":"string","description":"glob 匹配模式"},"path":{"type":"string","description":"搜索根目录"}},"required":["pattern"]}`, registryName: "file"},
	{key: "search_content", displayName: "内容搜索", description: "在工作区内搜索文本内容，结果带路径与行号。", category: "filesystem", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"content_pattern":{"type":"string","description":"正则表达式搜索模式"},"file_pattern":{"type":"string","description":"文件 glob 匹配模式，如 *.go"},"path":{"type":"string","description":"搜索根目录"},"file_case_sensitive":{"type":"boolean","description":"文件匹配是否区分大小写"},"content_case_sensitive":{"type":"boolean","description":"内容匹配是否区分大小写"}},"required":["content_pattern","file_pattern"]}`, registryName: "file"},
	{key: "replace_content", displayName: "替换内容", description: "按精确匹配替换文件中的文本片段。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"old_string":{"type":"string","description":"要替换的原始文本"},"new_string":{"type":"string","description":"替换后的文本"},"num_replacements":{"type":"integer","description":"替换次数，0 表示 1，负数表示全部"}},"required":["file_name","old_string","new_string"]}`, registryName: "file"},
	{key: "diff_edit", displayName: "片段编辑", description: "对已有文件施加一处或多处 search/replace 片段替换，原子提交。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"edits":{"type":"array","items":{"type":"object","properties":{"search":{"type":"string"},"replace":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["search","replace"]}},"expected_mtime_ms":{"type":"integer","description":"可选，来自 read_file 的 mtime_ms"}},"required":["file_name","edits"]}`, registryName: "file"},
	{key: "patch_file", displayName: "应用补丁", description: "应用 unified diff 或结构化 hunk 列表修改文件。", category: "filesystem", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"file_name":{"type":"string","description":"文件路径"},"patch":{"type":"string","description":"Unified diff 文本"},"hunks":{"type":"array","description":"结构化 hunk，与 patch 二选一"},"expected_mtime_ms":{"type":"integer","description":"可选，来自 read_file 的 mtime_ms"}},"required":["file_name"]}`, registryName: "file"},
	{key: "skill_search", displayName: "Skill 搜索", description: "搜索当前系统可用 Skill。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "use_skill", displayName: "使用 Skill", description: "标记本次运行使用某个 Skill，用于追踪。", category: "skill", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{key: "memory_search", displayName: "Memory 搜索", description: "搜索 Agent 长期记忆。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
	{key: "memory_get", displayName: "Memory 读取", description: "读取指定 memory 内容。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{key: "memory_replace", displayName: "记忆片段替换", description: "在已有记忆中查找并替换特定文本片段，用于精确更新记忆的局部内容。", category: "memory", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"memory_id":{"type":"string","description":"要编辑的记忆 ID"},"old_text":{"type":"string","description":"要查找并替换的文本片段"},"new_text":{"type":"string","description":"替换后的文本"}},"required":["memory_id","old_text","new_text"]}`},
	{key: "memory_rethink", displayName: "记忆深度重写", description: "用全新内容完全重写已有记忆，需提供重写理由用于溯源追踪。", category: "memory", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"memory_id":{"type":"string","description":"要重写的记忆 ID"},"new_content":{"type":"string","description":"完整的新内容"},"reason":{"type":"string","description":"重写理由（用于溯源）"}},"required":["memory_id","new_content","reason"]}`},
	{key: "memory_insert", displayName: "记忆内容插入", description: "在已有记忆的指定锚点文本后插入新内容。", category: "memory", riskLevel: "medium", enabled: true, paramsSchema: `{"type":"object","properties":{"memory_id":{"type":"string","description":"要插入内容的记忆 ID"},"after_text":{"type":"string","description":"插入位置的锚点文本"},"insert_text":{"type":"string","description":"要插入的文本"}},"required":["memory_id","after_text","insert_text"]}`},
	{key: "read_image", displayName: "图片理解", description: "分析图片内容。", category: "media", riskLevel: "medium", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`},
	{key: "read_document", displayName: "文档理解", description: "分析 PDF、Office、CSV 等文档。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`, registryName: "read_document"},
	{key: "read_spreadsheet", displayName: "表格读取", description: "读取 XLSX、CSV 等表格文件，支持行范围选择和工作表切换。", category: "media", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"path":{"type":"string"},"sheet":{"type":"string"},"row":{"type":"integer"},"start_row":{"type":"integer"},"end_row":{"type":"integer"},"max_chars":{"type":"integer"}},"required":["path"]}`, registryName: "read_spreadsheet"},
	{key: "create_image", displayName: "图片生成", description: "根据文本提示生成图片。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string"},"size":{"type":"string"}},"required":["prompt"]}`},
	{key: "generate_image", displayName: "图像生成", description: "根据文本描述生成图片（通义万相/ComfyUI，需在 media_providers 配置提供方）。产物自动落盘为会话制品。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string","description":"图像描述"},"size":{"type":"string","enum":["1024x1024","1792x1024","1024x1792"],"description":"尺寸"},"style":{"type":"string","enum":["realistic","anime","oil_painting"],"description":"风格"},"count":{"type":"integer","description":"生成数量 1-4"}},"required":["prompt"]}`},
	{key: "generate_video", displayName: "视频生成", description: "根据文本描述生成视频（需在 media_providers 配置提供方）。产物自动落盘为会话制品。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"prompt":{"type":"string","description":"视频描述"},"duration_ms":{"type":"integer","description":"时长 1000-30000"},"fps":{"type":"integer","description":"帧率 24-60"},"resolution":{"type":"string","enum":["720p","1080p"],"description":"分辨率"}},"required":["prompt"]}`},
	{key: "image_to_video", displayName: "图生视频", description: "将输入图片（Artifact ID 引用）转换为视频（需在 media_providers 配置提供方）。产物自动落盘为会话制品。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"image_artifact_id":{"type":"string","description":"输入图片 Artifact ID"},"prompt":{"type":"string","description":"运动描述"},"duration_ms":{"type":"integer"},"fps":{"type":"integer"}},"required":["image_artifact_id"]}`},
	{key: "tts", displayName: "文本转语音", description: "将文本转换成语音文件。", category: "media", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}`},
	{key: "shell_exec", displayName: "Shell 命令", description: "执行本地 shell 命令。", category: "runtime", riskLevel: "critical", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"},"workdir":{"type":"string","description":"Optional working directory relative to workspace root"}},"required":["command"]}`, registryName: "hostexec"},
	{key: "send_email", displayName: "邮件发送", description: "发送电子邮件。", category: "messaging", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"to":{"type":"string"},"subject":{"type":"string"},"body":{"type":"string"}},"required":["to","subject","body"]}`, registryName: "email"},
	{key: "todo_write", displayName: "待办管理", description: "管理待办事项列表，支持创建、更新和追踪任务进度。", category: "system", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"todos":{"type":"array","items":{"type":"object","properties":{"content":{"type":"string"},"activeForm":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","completed"]}},"required":["content","activeForm","status"]}}},"required":["todos"]}`, registryName: "todo"},
	{key: "await_user_reply", displayName: "等待回复", description: "暂停执行并等待用户回复，用于多轮对话场景。", category: "session", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "await_user_reply"},
	{key: "message", displayName: "出站消息", description: "通过已注册渠道（飞书/Telegram/Slack 等）发送文本和可选文件。未指定 channel/target 时从当前会话上下文解析。", category: "session", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"text":{"type":"string","description":"消息文本，无 files 时必填"},"files":{"type":"array","items":{"type":"string"},"description":"可选本地文件路径列表"},"channel":{"type":"string","description":"渠道 ID（如 telegram、slack、feishu），省略则从会话上下文解析"},"target":{"type":"string","description":"渠道目标（如 chat_id、open_id），省略则从会话上下文解析"}}}`},
	{key: "kanban", displayName: "Kanban 任务板", description: "Graph 任务看板工具集（kanban_show/complete/block/heartbeat/comment/create/link）。Worker 需设置 ARANEA_TASK_ID。", category: "integration", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "claude_code", displayName: "Claude Code", description: "使用 Claude Code 进行代码编辑和执行，包含 Bash、Read、Write、Edit、Glob、Grep 等子工具。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "claudecode"},
	{key: "workspace_exec", displayName: "工作区执行", description: "在工作区中执行命令并管理会话。", category: "runtime", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`, registryName: "workspace_exec"},
	{key: "call_agent", displayName: "调用 Agent", description: "调用同工作区内已启用 A2A 的另一 Agent 能力。", category: "integration", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"agent_id":{"type":"string","description":"目标 Agent ID"},"capability":{"type":"string","description":"能力名称，如 chat"},"payload":{"description":"传递给目标 Agent 的 JSON 载荷"},"timeout_seconds":{"type":"integer","description":"超时秒数"}},"required":["agent_id","capability"]}`},
	{key: "knowledge_search", displayName: "知识库搜索", description: "在指定知识库集合中检索相关文本片段。", category: "integration", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"collection_id":{"type":"string"},"query":{"type":"string"},"top_k":{"type":"integer"},"min_score":{"type":"number"}},"required":["collection_id","query"]}`},
	{key: "knowledge_reflect", displayName: "知识库反思", description: "跨多个知识库检索并评估结果质量，判断是否需要补充查询。", category: "integration", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"collection_ids":{"type":"array","items":{"type":"string"}},"query":{"type":"string"},"top_k":{"type":"integer"}},"required":["collection_ids","query"]}`},
	{key: "mcp_tool_set", displayName: "MCP 工具集", description: "挂载已配置的 MCP Server 工具。", category: "integration", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "mcp_broker", displayName: "MCP Broker", description: "运行时 MCP 发现与调用（mcp_list_servers / mcp_call 等）。", category: "integration", riskLevel: "medium", enabled: false, paramsSchema: `{"type":"object","properties":{}}`, registryName: "mcpbroker"},
	{key: "working_memory_read", displayName: "工作记忆读取", description: "读取当前任务的结构化工作记忆字段。不传 field_path 时返回整张快照。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string","description":"字段路径（可选）"}}}`},
	{key: "working_memory_list", displayName: "工作记忆列表", description: "列出当前任务下所有可见字段（path、preview、token_estimate、revision）。", category: "memory", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"include_internal":{"type":"boolean","description":"是否包含 visibility=internal 字段"}}}`},
	{key: "working_memory_write", displayName: "工作记忆写入", description: "向当前任务写入或更新一个结构化字段。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"},"value":{"description":"任意 JSON 值"},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown","decision","artifact","progress","constraint"],"description":"字段语义类型：decision=决策选择,artifact=文件引用,progress=进度状态,constraint=约束要求"},"visibility":{"type":"string","enum":["prompt","internal","shared"]},"pin_to_prompt":{"type":"boolean"},"reason":{"type":"string"},"if_revision":{"type":"integer","description":"乐观锁，期望的当前 revision"}},"required":["field_path","value"]}`},
	{key: "working_memory_patch", displayName: "工作记忆批量补丁", description: "一次写入多个字段。任意一项失败将中断后续写入并返回错误。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"patches":{"type":"array","items":{"type":"object","properties":{"field_path":{"type":"string"},"value":{},"field_kind":{"type":"string","enum":["string","number","boolean","json","reference","markdown","decision","artifact","progress","constraint"]},"visibility":{"type":"string"},"reason":{"type":"string"},"if_revision":{"type":"integer"}},"required":["field_path","value"]}}},"required":["patches"]}`},
	{key: "working_memory_delete", displayName: "工作记忆删除", description: "删除当前任务下的一个字段（历史版本仍可回滚）。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{"field_path":{"type":"string"}},"required":["field_path"]}`},
	{key: "working_memory_complete", displayName: "工作记忆任务完成", description: "声明当前任务目标已达成：结束任务并将其工作记忆暂存原子归档为情节记忆（episode）。归档后暂存清空，后续写入将自动开启新任务。", category: "memory", enabled: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "model_registry_sync", displayName: "模型目录同步（仅定时任务）", description: "模型目录同步工具集（fetch_model_directory / migrate_provider_bindings / apply_model_directory / sync_provider_logos）。仅由定时任务（cron）调用，不作为常规 Agent 工具装配。", category: "system", riskLevel: "medium", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{}}`, registryName: "model_registry_sync"},
	{key: "browser", displayName: "浏览器自动化", description: "通过 Playwright MCP 实现浏览器自动化操作（导航、截图、点击、输入等）。", category: "browser", riskLevel: "critical", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{}}`, configSchema: `{"type":"object","properties":{"command":{"type":"string","description":"MCP 启动命令","default":"npx"},"args":{"type":"array","items":{"type":"string"},"description":"MCP 启动参数"},"transport":{"type":"string","enum":["stdio","sse","streamable"],"description":"MCP 传输协议","default":"stdio"},"headless":{"type":"boolean","description":"无头模式","default":true},"vision":{"type":"boolean","description":"启用视觉能力","default":false},"isolated":{"type":"boolean","description":"隔离模式","default":true},"timeout_sec":{"type":"integer","description":"MCP 连接超时（秒）"}}}`, registryName: "browser"},
	{key: "read_tool_result", displayName: "读取工具结果", description: "通过 blob_id 检索之前持久化的工具结果完整内容。当对话中的工具结果被截断时，使用此工具获取完整输出。", category: "system", riskLevel: "low", enabled: true, readonly: true, registryName: "read_tool_result"},
	{key: "plan_and_execute", displayName: "规划并执行", description: "规划并执行任务。自动评估复杂度、分配 Agent、启动编排。", category: "spirit", source: "builtin", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"task_prompt":{"type":"string","description":"The task to plan and execute"},"mode":{"type":"string","description":"Execution mode: auto|direct|single|parallel|dag|coordinator (default: auto)"}},"required":["task_prompt"]}`},
	{key: "check_progress", displayName: "查询编排进度", description: "查询编排执行进度。基于 orchestration_id 查询。已由系统推送模式替代（团队完成后自动触发 Spirit 合成），不再对 LLM 暴露。", category: "spirit", source: "builtin", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"orchestration_id":{"type":"string","description":"The orchestration ID to check progress for"}},"required":["orchestration_id"]}`},
	{key: "cancel_orchestration", displayName: "取消编排", description: "取消正在运行的编排。基于 orchestration_id 取消。", category: "spirit", source: "builtin", riskLevel: "medium", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"orchestration_id":{"type":"string","description":"The orchestration ID to cancel"}},"required":["orchestration_id"]}`},
	{key: "synthesize_results", displayName: "合成团队结果", description: "将所有已完成团队的执行结果合成为综合报告。前置条件：所有并行团队均已完成（系统主动通知后调用）。", category: "spirit", source: "builtin", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"strategy":{"type":"string","description":"合成策略","enum":["template","llm","hybrid"]}}}`},
	{key: "build_orchestration_graph", displayName: "构建编排图", description: "构建 DAG 编排图，定义子任务依赖关系和执行顺序。", category: "spirit", source: "builtin", riskLevel: "low", enabled: true, readonly: true, paramsSchema: `{"type":"object","properties":{"task_description":{"type":"string","description":"任务描述"},"nodes":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"agent_key":{"type":"string"},"depends_on":{"type":"array","items":{"type":"string"}}}}},"verification_gates":{"type":"array","items":{"type":"object"}}},"required":["task_description","nodes"]}`},
	// Subagent tools: implementation in internal/tools/subagent/service.go (FrameworkTools).
	// Registered at runtime via cfg.SubAgent in internal/tools/trpc/toolsets.go.
	// Seeded as enabled=false; users opt-in per Agent via effective tool keys.
	{key: "subagents_spawn", displayName: "启动子 Agent", description: "为当前会话启动一个后台子 Agent。适用于长时间运行、可并行化或独立验证的工作。立即返回 run id。", category: "orchestration", source: "builtin", riskLevel: "medium", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"task":{"type":"string","description":"Delegated task for the background subagent."},"timeout_seconds":{"type":"integer","description":"Optional timeout in seconds for the delegated run."}},"required":["task"]}`},
	{key: "subagents_list", displayName: "列出子 Agent", description: "列出从当前会话创建的后台子 Agent。", category: "orchestration", source: "builtin", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{}}`},
	{key: "subagents_get", displayName: "获取子 Agent", description: "获取一个后台子 Agent 运行的最新状态和结果。", category: "orchestration", source: "builtin", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string","description":"Subagent run id returned by spawn."}},"required":["id"]}`},
	{key: "subagents_cancel", displayName: "取消子 Agent", description: "取消一个后台子 Agent 运行。这是 best-effort 操作。", category: "orchestration", source: "builtin", riskLevel: "medium", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"id":{"type":"string","description":"Subagent run id returned by spawn."}},"required":["id"]}`},
	// Client tool bridge (74-voice-companion §6): execution happens on the
	// user's desktop companion via WS routing, never on the server. Opt-in;
	// confirmation required (session/persisted grant 可免确认).
	{key: "client_open_app", displayName: "打开桌面应用", description: "在用户的桌面客户端上打开应用（按客户端白名单解析目标）。需要桌面客户端在线。", category: "client", riskLevel: "medium", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"target":{"type":"string","description":"应用名或白名单别名，如 wechat、chrome"}},"required":["target"]}`, registryName: "client"},
	{key: "client_open_url", displayName: "打开网址", description: "在用户的桌面客户端默认浏览器中打开 http/https 网址。需要桌面客户端在线。", category: "client", riskLevel: "low", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"url":{"type":"string","description":"要打开的 http/https URL"}},"required":["url"]}`, registryName: "client"},
	// Computer Use 桌面自动化工具集（75-computer-use）：经 sidecar 驱动真实桌面。
	// act/launch 注入真实输入/拉起进程，高危需人工确认；默认关闭，按 Agent 生效工具键开启。
	{key: "computer_use_observe", displayName: "桌面感知", description: "感知当前桌面：返回前台窗口的可访问元素清单（ref/名称/类型/位置），供 computer_use_act 引用。", category: "computeruse", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"window_title":{"type":"string","description":"可选，限定窗口标题（正则）"},"include_screenshot":{"type":"boolean"},"max_elements":{"type":"integer"}}}`},
	{key: "computer_use_screenshot", displayName: "桌面截图", description: "截取当前桌面截图，返回 base64 PNG 与尺寸元数据。", category: "computeruse", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"region":{"type":"object","properties":{"x":{"type":"integer"},"y":{"type":"integer"},"w":{"type":"integer"},"h":{"type":"integer"}}},"zoom":{"type":"number"}}}`},
	{key: "computer_use_act", displayName: "桌面动作", description: "在桌面执行语义动作：给出目标元素的自然语言描述，系统自动定位并操作（invoke/click/type/key）。支持 dry_run 干跑；高危动作强制人工确认。支持 actions 数组批量执行：多步按序 fail-fast，任一步失败即停且错误注明已完成步数（请勿整体重试已执行步骤）。", category: "computeruse", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"target":{"type":"string","description":"目标元素的自然语言描述；坐标点击时可省略"},"action":{"type":"string","enum":["invoke","click","type","key"],"description":"动作类型，默认 invoke"},"text":{"type":"string","description":"action=type 时要输入的文本"},"combo":{"type":"string","description":"action=key 时组合键，如 ctrl+s"},"x":{"type":"integer","description":"action=click 且无 target 时的物理像素 X"},"y":{"type":"integer","description":"action=click 且无 target 时的物理像素 Y"},"button":{"type":"string","enum":["left","right","middle"]},"click_count":{"type":"integer"},"actions":{"type":"array","description":"批量动作：非空时忽略单步参数，按序 fail-fast 执行","items":{"type":"object","properties":{"target":{"type":"string"},"action":{"type":"string","enum":["invoke","click","type","key"]},"text":{"type":"string"},"combo":{"type":"string"},"x":{"type":"integer"},"y":{"type":"integer"},"button":{"type":"string","enum":["left","right","middle"]},"click_count":{"type":"integer"}},"required":["action"]}},"dry_run":{"type":"boolean"},"session_id":{"type":"string"},"confirmed_by":{"type":"string"}}}`},
	{key: "computer_use_launch", displayName: "启动桌面应用", description: "启动桌面应用：target 为应用名或完整路径（如 notepad.exe）。", category: "computeruse", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"target":{"type":"string"},"args":{"type":"string"},"work_dir":{"type":"string"},"confirmed_by":{"type":"string"}},"required":["target"]}`},
	{key: "computer_use_session", displayName: "桌面会话管理", description: "管理桌面自动化会话：action=start(可选预算)|stop|status|kill(急停)。会话累计步数预算，超限自动拒绝动作。", category: "computeruse", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"action":{"type":"string","enum":["start","stop","status","kill"]},"session_id":{"type":"string"},"max_steps":{"type":"integer"},"duration_minutes":{"type":"integer"}},"required":["action"]}`},
	// Coding agent bridge（76-coding-agent-bridge §13）：向已注册的外部编程 CLI 派发编程任务。
	{key: "coding_dispatch_task", displayName: "派发编程任务", description: "将编程任务派发给外部编程 Agent（Claude Code / Codex / CodeBuddy），在已注册的本地项目上后台执行。返回 task_id，完成后自动通知结果。", category: "coding", riskLevel: "high", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"agent_key":{"type":"string","description":"编程 Agent 标识，如 claude-code / codex / codebuddy"},"project":{"type":"string","description":"目标项目名称（模糊匹配，歧义时返回候选列表）"},"prompt":{"type":"string","description":"编程任务描述"}},"required":["agent_key","project","prompt"]}`, registryName: "coding"},
	{key: "coding_check_task", displayName: "查询编程任务", description: "查询编程任务状态和结果。省略 task_id 时返回当前会话最近的一条编程任务。", category: "coding", riskLevel: "low", enabled: false, readonly: true, paramsSchema: `{"type":"object","properties":{"task_id":{"type":"string","description":"dispatch_task 返回的任务 ID；省略时查最近任务"}}}`, registryName: "coding"},
	{key: "coding_cancel_task", displayName: "取消编程任务", description: "取消正在运行的编程任务（best-effort）。", category: "coding", riskLevel: "medium", enabled: false, reqConfirm: true, paramsSchema: `{"type":"object","properties":{"task_id":{"type":"string","description":"要取消的任务 ID"}},"required":["task_id"]}`, registryName: "coding"},
}

func ensureBuiltinPlatformTools(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
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
	qDialect := d.RenumberPlaceholders(q)
	for _, row := range builtinPlatformToolSeeds {
		applyPlatformToolDefaults(&row)
		id := "tool_" + strings.ReplaceAll(row.key, ".", "_")
		configSchema := strings.TrimSpace(row.configSchema)
		if configSchema == "" {
			configSchema = "{}"
		}
		_, err := client.ExecContext(ctx, qDialect,
			id, row.key, row.displayName, row.description, row.category, row.source, row.riskLevel,
			b2i(row.enabled), b2i(row.readonly), b2i(row.reqConfirm),
			b2i(row.suppStream), b2i(row.suppConc),
			row.paramsSchema, "{}", configSchema, "{}", "{}", "{}",
			now, now,
		)
		if err != nil {
			lg.Warn("seed step failed", loggateway.StepID("data.seed.builtin_tools"), loggateway.Str("tool_key", row.key), loggateway.Err(err))
			return fmt.Errorf("ensure builtin tools: seed %q: %w", row.key, err)
		}
	}
	if err := syncBuiltinToolsFromRegistry(ctx, client, d, lg); err != nil {
		lg.Warn("内置工具批量同步失败", loggateway.StepID("data.builtin_tool_sync"), loggateway.Err(err))
	}
	if err := syncBuiltinWebToolCatalogPatches(ctx, client, d); err != nil {
		lg.Warn("内置 Web 工具元数据同步失败", loggateway.StepID("data.builtin_tool_sync"), loggateway.Err(err))
	}
	if err := syncBuiltinFilesystemToolCatalogPatches(ctx, client, d); err != nil {
		lg.Warn("内置文件工具元数据同步失败", loggateway.StepID("data.builtin_tool_sync"), loggateway.Err(err))
	}
	return nil
}

// syncBuiltinWebToolCatalogPatches updates catalog metadata for existing DBs (seed uses ON CONFLICT DO NOTHING).
func syncBuiltinWebToolCatalogPatches(ctx context.Context, client *ent.Client, d Dialect) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const upd = `UPDATE tools SET
		enabled = ?, description = ?, parameters_schema_json = ?, config_schema_json = ?, updated_at = ?
		WHERE tool_key = ? AND source = 'builtin' AND deleted_at = ''`
	updDialect := d.RenumberPlaceholders(upd)
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
		if _, err := client.ExecContext(ctx, updDialect, p.enabled, p.desc, p.params, p.config, now, p.key); err != nil {
			return fmt.Errorf("sync web tool %q: %w", p.key, err)
		}
	}
	return nil
}

// syncBuiltinFilesystemToolCatalogPatches enables diff_edit/patch_file on existing DBs.
// These tools were previously seeded as enabled=false while their runtime implementations
// were pending; now that diffedit.go/patchfile.go are registered in the file ToolSet,
// flip existing rows to enabled=1. Seed uses ON CONFLICT DO NOTHING, so existing rows
// need an explicit UPDATE.
func syncBuiltinFilesystemToolCatalogPatches(ctx context.Context, client *ent.Client, d Dialect) error {
	if client == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const upd = `UPDATE tools SET enabled = 1, updated_at = ?
		WHERE tool_key IN (?, ?) AND source = 'builtin' AND deleted_at = ''`
	updDialect := d.RenumberPlaceholders(upd)
	if _, err := client.ExecContext(ctx, updDialect, now, "diff_edit", "patch_file"); err != nil {
		return fmt.Errorf("sync filesystem tools (diff_edit/patch_file): %w", err)
	}
	return nil
}

func syncBuiltinToolsFromRegistry(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
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
	updDialect := d.RenumberPlaceholders(upd)
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
		_, err := client.ExecContext(ctx, updDialect,
			riskLevel, b2i(reqConfirm), b2i(suppStream), b2i(suppConc),
			seed.description, seed.paramsSchema,
			now, seed.key,
		)
		if err != nil {
			lg.Warn("内置工具同步失败", loggateway.StepID("data.builtin_tool_sync"), loggateway.Str("tool_key", seed.key), loggateway.Str("registry_name", regName), loggateway.Err(err))
		}
	}
	return nil
}
