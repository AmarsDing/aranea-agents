# Tools 工具管理 — 产品需求

> **版本**：4.0 | **状态**：✅ 核心已实现
> **设计**：[23 tools.design.md](./23%20tools.design.md) · **开发计划**：[23-tools-development.md](./23-tools-development.md)

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| Tool 列表 | ✅ 已实现 | 展示内置 Tool、MCP Tool、系统注册 Tool 的启用状态、分类、风险级别、schema、最近调用 |
| Tool 详情 | ✅ 已实现 | 展示工具描述、参数 schema、返回结构、配置、Agent 覆盖、最近运行 |
| 启用 / 停用 | ✅ 已实现 | 支持全局启停；Agent 级可通过 allow / deny 覆盖；高风险工具需二次确认 |
| Agent 绑定 | ✅ 已实现 | 基于现有 `agent_runtime_settings` 的 allow / deny / profile 管理 |
| 参数配置 | ✅ 已实现 | Tool 自身配置与 Agent 覆盖配置分开管理 |
| 调用记录 | ✅ 已实现 | 记录 tool call 参数摘要、参数 hash、结果摘要、耗时、状态、错误 |
| 参数详情 | ✅ 已实现 | 管理员可查看脱敏后参数；敏感字段默认不明文落库 |
| 使用统计 | ✅ 已实现 | 调用次数、成功率、平均耗时、最近调用、按 Agent 分布 |
| Agent 工具覆盖 | ✅ 已实现 | `tool_agent_overrides` 表支持单 Tool 粒度的 Agent 级覆盖 |
| Tool 配置更新 | ✅ 已实现 | `PUT /v1/tools/{id}/config` 独立更新工具配置 |
| 动态上传 Tool 代码 | 否 | 不允许前端上传任意 Go / JS 代码作为 Tool |
| MCP Tool 发现 | 后续增强 | 本期先预留 `source=mcp`，可先展示已注册 MCP 工具 |
| 工具在线测试 | 后续 | 自定义工具无在线测试功能，用户无法在配置时验证工具是否可用 |
| 工具调用审计日志 | 后续 | 工具调用无审计日志，无法追溯谁在何时调用了什么工具 |

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 路由 | `/tools` 为 Tools 管理页；`/tools/runs` 为调用记录页 |
| 管理对象 | 内置 Tool + 后端注册 Tool + 可发现 MCP Tool |
| 工具来源 | `builtin` / `mcp` / `system` / `external` |
| 工具执行层级 | trpc-agent-go `llmagent.Config.Tools` / `ToolSets` 注入，执行前后由 Callbacks / Plugin 记录 |
| 配置格式 | 后端维护 `config_schema_json` 和 `parameters_schema_json`，前端按 schema 渲染表单 |
| 默认状态 | 读类和观测类默认启用；写入、执行命令、外部发送、高成本媒体生成默认谨慎启用或需确认 |
| 风险级别 | `low` / `medium` / `high` / `critical` |
| 参数记录 | 默认只存脱敏 preview、hash、类型信息；完整明文不落库 |
| 权限 | 单用户本地控制台默认拥有管理能力；敏感参数与高风险操作仍由后端做安全保护 |
| 删除 | 内置 Tool 不删除，只可停用；外部注册 Tool 软删 |

### 0.3 单用户能力与安全控制

当前产品按单用户本地控制台设计，不引入管理员 / 编辑者 / 只读用户角色矩阵。前端默认展示管理入口，后端仍必须负责安全边界。

| 能力 | 本期行为 |
|------|----------|
| 查看 Tool 列表与 schema | 允许 |
| 启用 / 停用 Tool | 允许；高风险工具需要二次确认（`confirm_key`） |
| 修改 Tool 全局配置 | 允许；配置必须按后端 schema 校验 |
| 修改 Agent 绑定 | 允许；写入 `agent_runtime_settings` 或 `tool_agent_overrides` |
| 查看调用记录 | 允许；默认展示脱敏摘要 |
| 查看参数详情 | 允许查看脱敏 preview、hash、类型、敏感标记；不展示明文 secret |
| 重放 Tool 调用 | 后续迭代；本期不提供，避免误触发写入 / 外部发送 |

安全控制不依赖"角色"实现，而是由后端按工具风险、字段敏感度和执行环境控制：

- 高风险工具启用和执行前需要显式确认。
- 参数记录默认脱敏，只存 preview 和 hash。
- schema 外字段被拒绝。
- 系统注入字段（如 `agent_id`、`session_id`、`workspace_id`）不允许模型或前端覆盖。

---

## 1. Tool 与 Skill / Plugin 的边界

| 概念 | 定位 | 管理重点 |
|------|------|----------|
| **Tool** | Agent 可调用的具体能力，例如文件、搜索、MCP、媒体、业务 API | 启用、授权、参数 schema、调用记录、风险控制 |
| **Skill** | Agent 使用某类能力的说明、知识和操作规范 | 上传、验证、冲突、版本、使用追踪 |
| **Plugin** | trpc-agent-go Runtime 回调中间件，拦截模型、工具、事件链路 | 启用、排序、绑定、运行日志 |

---

## 2. Tool 配置分层

| 层级 | 来源 | 说明 | 优先级 |
|------|------|------|--------|
| 系统默认 | 代码内置 registry + seed | Tool 名称、描述、参数 schema、默认风险级别 | 低 |
| 全局配置 | `tools.config_json` | 管理员在 Tools 页配置，例如默认超时、provider chain | 中 |
| Agent 策略 | `agent_runtime_settings` | `tools_profile`、`tools_allow_json`、`tools_deny_json` | 高 |
| Agent Tool 覆盖 | `tool_agent_overrides` | 单 Tool 粒度的 Agent 级 config override / allow / deny | 高 |
| 单次调用上下文 | Runtime context | session、user、workspace、request_id 等系统注入参数 | 最高 |

模型只能控制 `parameters_schema_json` 中声明为可见的字段。`tenant_id`、`agent_id`、`workspace_id`、`session_id`、`request_id` 这类系统字段由后端注入，不进入模型可写参数。

---

## 3. Tool 分类与内置工具

### 3.1 分类

| 分类 | 示例 | 风险 |
|------|------|------|
| `system` | `datetime`、`todo_write` | 低风险 |
| `filesystem` | `read_file`、`save_file`、`list_file`、`replace_content`、`search_content` | 读低风险，写中高风险 |
| `web` | `duckduckgo_search`、`web_fetch`、`gemini_web_fetch`、`google_search`、`arxiv_search`、`wikipedia_search` | 中风险 |
| `search` | 同 web 分类搜索工具 | 低中风险 |
| `memory` | `memory_search`、`memory_get`、`working_memory.*` | 低中风险 |
| `skill` | `skill_search`、`use_skill` | 低中风险 |
| `media` | `read_image`、`read_document`、`create_image`、`tts` | 成本与隐私风险 |
| `session` | `await_user_reply` | 低风险 |
| `runtime` | `shell_exec`、`claude_code`、`workspace_exec` | 高风险 |
| `messaging` | `send_email` | 高风险 |
| `integration` | `mcp_tool_set`、`mcp_broker`、`openapi` | 中高风险 |
| `composition` | `call_agent`（Agent-as-Tool） | 中风险 |
| `knowledge` | `knowledge_search` | 低风险 |

### 3.2 已实现内置工具

> 与 `internal/data/builtin_tools_seed.go` 保持同步

| Tool Key | 名称 | 分类 | 风险 | 默认 | 说明 |
|----------|------|------|------|------|------|
| `datetime` | 当前时间 | system | low | 启用 | 返回当前时间、日期和时区信息 |
| `duckduckgo_search` | DuckDuckGo 搜索 | web | medium | 启用 | 搜索实时网络信息 |
| `web_fetch` | Web 抓取 | web | medium | 启用 | 抓取 URL 并提取页面文本 |
| `gemini_web_fetch` | Gemini 抓取 | web | medium | 启用 | 使用 Gemini 模型抓取并理解 URL 内容 |
| `google_search` | Google 搜索 | web | medium | 启用 | 使用 Google Custom Search API |
| `arxiv_search` | ArXiv 搜索 | web | low | 启用 | 搜索 ArXiv 学术论文 |
| `wikipedia_search` | Wikipedia 查询 | web | low | 启用 | 搜索和获取 Wikipedia 内容 |
| `read_file` | 读取文件 | filesystem | low | 启用 | 读取工作区文件内容 |
| `read_multiple_files` | 批量读取文件 | filesystem | low | 启用 | 一次读取多个文件 |
| `save_file` | 保存文件 | filesystem | medium | 启用 | 创建或覆盖工作区文件 |
| `list_file` | 文件列表 | filesystem | low | 启用 | 列出工作区目录内容 |
| `search_file` | 文件搜索 | filesystem | low | 启用 | 按文件名模式搜索 |
| `search_content` | 内容搜索 | filesystem | low | 启用 | 在工作区内搜索文本内容 |
| `replace_content` | 替换内容 | filesystem | medium | 启用 | 按精确匹配替换文件中的文本 |
| `skill_search` | Skill 搜索 | skill | low | 启用 | 搜索当前系统可用 Skill |
| `use_skill` | 使用 Skill | skill | low | 启用 | 标记本次运行使用某个 Skill |
| `memory_search` | Memory 搜索 | memory | low | 启用 | 搜索 Agent 长期记忆 |
| `memory_get` | Memory 读取 | memory | low | 启用 | 读取指定 memory 内容 |
| `read_image` | 图片理解 | media | medium | 启用 | 分析图片内容 |
| `read_document` | 文档理解 | media | medium | 启用 | 分析 PDF、Office、CSV 等文档 |
| `create_image` | 图片生成 | media | medium | 停用 | 根据文本提示生成图片 |
| `tts` | 文本转语音 | media | medium | 停用 | 将文本转换成语音文件 |
| `shell_exec` | Shell 命令 | runtime | critical | 停用 | 执行本地 shell 命令，需确认 |
| `send_email` | 邮件发送 | messaging | high | 停用 | 发送电子邮件，需确认 |
| `todo_write` | 待办管理 | system | low | 启用 | 管理待办事项列表 |
| `await_user_reply` | 等待回复 | session | low | 启用 | 暂停执行并等待用户回复 |
| `claude_code` | Claude Code | runtime | high | 停用 | 代码编辑和执行，需确认 |
| `workspace_exec` | 工作区执行 | runtime | high | 停用 | 在工作区中执行命令，需确认 |
| `working_memory.read` | 工作记忆读取 | memory | low | 启用 | 读取当前任务的结构化工作记忆 |
| `working_memory.list` | 工作记忆列表 | memory | low | 启用 | 列出当前任务下所有可见字段 |
| `working_memory.write` | 工作记忆写入 | memory | low | 启用 | 向当前任务写入或更新字段 |
| `working_memory.patch` | 工作记忆批量补丁 | memory | low | 启用 | 一次写入多个字段 |
| `working_memory.delete` | 工作记忆删除 | memory | low | 启用 | 删除当前任务下的一个字段 |

### 3.3 框架注册但未在种子表独立列出的工具

> 与 `internal/tools/toolset.go` Registry 保持同步

| Registry Name | 说明 | 注入方式 |
|---------------|------|----------|
| `file` | File ToolSet（包含 read_file/save_file/list_file 等） | ToolSetFactory |
| `hostexec` | Shell 命令执行 ToolSet | ToolSetFactory |
| `httpfetch` | HTTP 页面抓取 | Factory |
| `geminifetch` | Gemini 页面抓取 | Factory |
| `duckduckgo` | DuckDuckGo 搜索 | Factory |
| `google_search` | Google Custom Search ToolSet | ToolSetFactory |
| `arxiv_search` | ArXiv 搜索 ToolSet | ToolSetFactory |
| `wikipedia` | Wikipedia ToolSet | ToolSetFactory |
| `email` | 邮件发送 ToolSet | ToolSetFactory |
| `todo` | 待办管理 | Factory |
| `await_user_reply` | 等待用户回复 | Factory |
| `claudecode` | Claude Code ToolSet | ToolSetFactory |
| `workspace_exec` | 工作区执行 | Factory |
| `openapi` | OpenAPI 动态 REST 工具 | ToolSetFactory |
| `agent` | Agent-as-Tool 委托 | 通过 AssemblyConfig.AgentTools |
| `mcp` | MCP ToolSet 外部工具 | ToolSetFactory |
| `mcpbroker` | MCP Broker 运行时发现 | Factory |

---

## 4. 信息架构与路由

| 路由 | 页面 | 说明 |
|------|------|------|
| `/tools` | Tools 管理 | 列表、启停、分类筛选、配置入口、Agent 绑定入口 |
| `/tools/:id` | Tool 详情 | schema、配置、调用示例、Agent 覆盖、最近调用 |
| `/tools/runs` | Tool 调用记录 | 按 Tool / Agent / Session / 状态 / 时间筛选调用明细 |
| `/agents/:id/tools` | Agent 工具配置 | 嵌入 Agent 设置页，编辑 profile、allow、deny、并发许可 |

侧栏放在管理类导航下，文案为「Tools 管理」和「Tool 调用记录」。

---

## 5. Tools 管理页

### 5.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Tools 管理」 |
| 副标题 | 展示当前工作区、已启用数量、近 24 小时调用量 |
| 右上操作 | 「刷新」「同步内置工具」「查看调用记录」 |
| 概览卡片 | 总工具数、启用工具数、高风险启用数、近 24h 失败率 |
| 工具栏 | 搜索、分类、来源、风险级别、启用状态、仅看异常 |
| 主表 | `QTable` 服务端分页，支持按调用量、失败率、最近调用排序 |
| 详情抽屉 | 点击行打开右侧 `QDrawer` / `QDialog` 查看配置与 schema |

搜索行为：

- 搜索字段匹配 `key`、`display_name`、`description`、`category`。
- 输入框使用 `debounce="300"`。
- 搜索、筛选、每页条数变化后重置到第 1 页。

### 5.2 表格列

| 列 | 字段 | UI 与交互 |
|----|------|-----------|
| 名称 | `display_name`、`key` | 主行展示名称，副行展示 key；内置工具带 `builtin` chip |
| 分类 | `category` | `QChip`，按分类上色 |
| 来源 | `source` | `builtin` / `mcp` / `system` / `external` |
| 风险 | `risk_level` | `low` 灰绿，`medium` 黄，`high` 橙，`critical` 红 |
| 启用 | `enabled` | `QToggle dense`；无权限或内置锁定时禁用 |
| Agent 覆盖 | `agent_override_count` | 展示有 allow / deny 覆盖的 Agent 数 |
| 使用频率 | `invoke_count`、`invoke_count_24h` | 总调用 + 近 24 小时调用 |
| 成功率 | `success_count`、`failure_count` | 展示成功率，低于阈值标红 |
| 耗时 | `avg_duration_ms`、`p95_duration_ms` | 平均与 P95 |
| 最近调用 | `last_invoked_at`、`last_status` | 无调用展示「未调用」 |
| 操作 | `permissions` | 详情、配置、绑定 Agent、查看调用 |

### 5.3 启用 / 停用行为

1. 用户切换 `QToggle`。
2. 前端进入行级 loading，不立即确认最终状态。
3. 调用 `PATCH /v1/tools/{id}/enabled`。
4. 成功后更新该行和概览卡片。
5. 失败后恢复原状态并提示错误。

高风险工具启用时必须二次确认：

- 弹窗展示工具名、风险级别、影响范围。
- 若工具属于 `runtime`、`messaging`、`external`，要求输入工具 key 确认（`confirm_key` 字段）。
- 后端仍需做权限校验，不能只依赖前端确认。

### 5.4 详情抽屉

详情抽屉分为 5 个 Tab：

| Tab | 内容 |
|-----|------|
| 概览 | 描述、分类、风险、来源、启用状态、依赖能力 |
| 参数 | `parameters_schema_json` 渲染的只读 schema、必填字段、示例参数 |
| 配置 | `config_schema_json` 渲染的可编辑表单；敏感字段仅显示已配置 |
| Agent | 使用该工具的 Agent、allow / deny 覆盖、profile 命中 |
| 调用 | 最近 20 条调用记录、错误摘要、跳转完整记录 |

---

## 6. Agent 工具配置

当前系统已有 `agent_runtime_settings` 工具字段，继续复用，避免重复建一套 Agent 工具策略表。

### 6.1 配置项

| 字段 | 来源 | 说明 |
|------|------|------|
| `tools_enabled` | `agent_runtime_settings` | Agent 是否启用工具调用 |
| `tools_profile` | `agent_runtime_settings` | `chat_only` / `read_only` / `coding` / `research` / `full` 等预设 |
| `tools_allow_json` | `agent_runtime_settings` | Agent 明确允许的工具 key 或 `group:filesystem` |
| `tools_deny_json` | `agent_runtime_settings` | Agent 明确禁止的工具 key 或工具组 |
| `tools_concurrent_allow_json` | `agent_runtime_settings` | 允许并发执行的工具 key |
| `tools_parallel_enabled` | `agent_runtime_settings` | 是否允许并行工具调用 |
| `tools_streaming_enabled` | `agent_runtime_settings` | 是否启用流式工具 |
| `tools_retry_enabled` | `agent_runtime_settings` | 是否启用工具调用重试 |
| `tools_retry_max_attempts` | `agent_runtime_settings` | 重试最大次数 |
| `tools_retry_initial_interval_ms` | `agent_runtime_settings` | 重试初始间隔 |
| `tools_retry_backoff_factor` | `agent_runtime_settings` | 重试退避因子 |
| `tools_retry_max_interval_ms` | `agent_runtime_settings` | 重试最大间隔 |
| `tools_retry_jitter` | `agent_runtime_settings` | 是否启用重试抖动 |

### 6.2 Profile 预设

| Profile | 包含工具组 |
|---------|-----------|
| `chat_only` | 无工具 |
| `read_only` | `datetime`、`read_file`、`read_multiple_files`、`list_file`、`search_file`、`search_content`、`todo_write` |
| `coding` | `group:filesystem`、`group:web`、`group:skill`、`group:session`、`datetime` |
| `research` | 搜索 + 文件读取 + skill + memory + `datetime` |
| `full` | 全部工具组 |

### 6.3 Agent 页 UI

在 Agent 详情页新增「Tools」Tab：

| 区域 | 需求 |
|------|------|
| 总开关 | `tools_enabled` |
| Profile | `QSelect`，展示 profile 说明和包含的工具组 |
| Allow | `QSelect multiple`，选择工具或 `group:filesystem` |
| Deny | `QSelect multiple`，选择工具或工具组 |
| 并发许可 | `QSelect multiple`，只允许选择幂等 / 读类工具 |
| 预览 | 展示最终可用工具列表，按后端 `GET /agents/{id}/tools/effective` 返回的结果 |

前端不应自行计算最终工具列表，只展示后端返回的结果，避免与后端策略不一致。

---

## 7. Tool 调用记录页

### 7.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Tool 调用记录」 |
| 右上 | 刷新、导出 CSV（后续）、清理策略入口（后续） |
| 筛选 | Tool、Agent、Session、状态、时间范围、仅看错误 |
| 主表 | 服务端分页 |
| 详情弹窗 | 参数、输出、错误、metadata |

### 7.2 表格列

| 列 | 字段 | 说明 |
|----|------|------|
| 时间 | `started_at` | 本地化显示 |
| Tool | `tool_key`、`tool_display_name` | 标签 + 名称 |
| Agent | `agent_display_name` / `agent_key` | 可点击跳转 |
| Session | `session_id` | 可复制，后续跳转会话 |
| 状态 | `status` | 成功、错误、阻断、取消 |
| 耗时 | `duration_ms` | 超过 P95 标记 |
| 参数摘要 | `input_preview` | 单行截断 |
| 结果摘要 | `output_preview` | 单行截断 |
| 错误 | `error_message` | 错误时展示 |
| 操作 | - | 查看详情 |

### 7.3 详情弹窗

| Tab | 内容 |
|-----|------|
| 参数 | `params_json`（脱敏）、`redaction_applied` 标记 |
| 输出 | `output_preview`、`output_hash` |
| 错误 | `error_code`、`error_message`、metadata |
| 上下文 | request_id、invocation_id、session_id、message_id、agent_id |

详情弹窗必须明确提示：「参数已脱敏，hash 仅用于排查重复调用，不能还原原文。」

---

## 8. 与现有模块的关系

| 模块 | 关系 |
|------|------|
| Skill（`20 skill.md`） | `skill_search` / `use_skill` 的调用记录可关联 Skill 运行记录 |
| Plugin（`22 plugin.md`） | Plugin 负责运行链路回调；Tool Callbacks / Adapter 负责权威调用记录 |
| Monitor（`18 monitor.md`） | Tool 调用可同步发实时事件，Monitor 页面展示 `tool.call` / `tool.result` |
| Agent 设置 | 复用 `agent_runtime_settings` 的工具策略字段 |
| Knowledge（`37 knowledge.md`） | `knowledge_search` 工具与知识库模块联动 |
| MCP | MCP ToolSet / Broker 通过 `tool_agent_overrides` 和 Agent 设置管理 |

---

## 9. 前端状态与交互细节

### 9.1 状态展示

| 状态 | UI |
|------|----|
| 首次加载 | `QInnerLoading` 或 `QSkeleton` |
| 空列表 | `QBanner` +「同步内置工具」 |
| 搜索无结果 | 文案「没有匹配的 Tool」+「重置筛选」 |
| 请求失败 | `QBanner` negative +「重试」 |
| 无权限 | 按钮禁用，`QTooltip` 展示原因 |
| 高风险开启 | `QDialog` 二次确认 |

### 9.2 可访问性

- 所有 icon-only 按钮必须有 `aria-label`。
- `QToggle` 需要明确 label，不只依赖颜色表达启用状态。
- 风险级别除颜色外，还必须显示文字。
- 详情抽屉打开后焦点进入标题，关闭后回到触发按钮。
- 调用记录错误信息使用可复制文本，不只放 tooltip。

---

## 10. 后续需求

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 工具在线测试 | P3 | 自定义工具可在配置时在线测试 |
| 工具调用审计日志 | P3 | `tool_invocation_audit` 表 + 查询 API |
| 工作区搜索增强 | P1 | `workspace_search` 字面检索工具（P0-WS） |
| 代码沙箱执行 | P2 | E2B / Jupyter / Container 沙箱 |
| 多渠道通知 | P2 | 统一通知接口（邮件/IM/Webhook） |
| 数据库查询工具 | P3 | 安全 SQL 查询（只读，白名单表） |
| 工作流编排工具 | P3 | 与 Graph Workflow 模块对齐 |
