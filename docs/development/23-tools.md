# Tools 工具管理 — 产品需求

> **版本**：4.3 | **状态**：✅ 核心已实现；**片段编辑 ✅ Phase 4**；**工作区统一 ✅ Phase 5**
> **设计**：[23 tools.design.md](./23%20tools.design.md) · **开发计划**：[23-tools-development.md](./23-tools-development.md) · **片段编辑**：[23 tools-fragment-edit.md](./23%20tools-fragment-edit.md)

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
| 工具在线测试 | ✅ 已实现 | `TestTool` RPC + 工具详情「在线测试」 |
| 工具调用审计日志 | 后续 | 工具调用无审计日志，无法追溯谁在何时调用了什么工具 |
| 片段级文件编辑 | ✅ 已实现 | `diff_edit` / `patch_file` + SessionFileState；见 [23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) |
| **工具工作区统一** | ✅ 已实现 | file / shell / claude_code 共用 `workspace_root`；见 [23-tools-development.md §Phase 5](./23-tools-development.md#phase-5工具工作区统一p0) |

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

### 2.1 工具工作区（Workspace Root）

Agent 运行时对「需要目录」的工具共用 **单一工作区根** `workspace_root`（Cursor 式：Agent 在用户选定的项目目录内读写文件、跑 shell）。

| 决策项 | 默认值 |
|--------|--------|
| 工作区含义 | 绝对路径目录；file 工具路径均相对或受限于此根 |
| 解析来源 | 系统 `root_directory` + `workspace/{agent_key}`；Tool 配置 `filesystem_dir` / `base_dir`；环境变量 `ARANEA_WORKSPACE_ROOT` / `WORKSPACE_ROOT`（见设计 §7.8） |
| file 工具 | 严格限制在工作区内 |
| `shell_exec` | 默认 cwd = 工作区根；调用参数 `workdir` 可指定子目录或（在 OS 权限内）绝对路径 |
| `claude_code` | 未单独配置 `claude_code_dir` 时与工作区根相同 |
| 不需工作区的工具 | web 搜索/抓取、email、todo、MCP、memory、knowledge 等（见设计 §7.8 矩阵） |
| `workspace_exec` | 依赖 CodeExecutor 工作区，与 file/shell 根目录 **可能不同**；不替代日常 `shell_exec` |

**验收**：同一 turn 内 `save_file` 写入的文件，紧随其后的 `exec_command` 可在工作区内访问；shell 不再落在 Server 进程当前目录。

---

## 3. Tool 分类与内置工具

### 3.1 分类

| 分类 | 示例 | 风险 |
|------|------|------|
| `system` | `datetime`、`todo_write` | 低风险 |
| `filesystem` | `read_file`、`save_file`、`list_file`、`replace_content`、`search_content` | 读低风险，写中高风险 |
| `web` | `web_research`、`web_fetch`、`duckduckgo_search`、`gemini_web_fetch`、`google_search`、`arxiv_search`、`wikipedia_search` | 中风险 |
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
| `web_research` | Web 研究 | web | medium | 启用 | Tavily / SerpAPI 统一搜索；密钥见系统设置 → Web 研究 |
| `duckduckgo_search` | DuckDuckGo 搜索 | web | medium | 停用 | Instant Answer（非通用网页搜索） |
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
| `diff_edit` | 片段编辑 | filesystem | medium | 启用 | 多片段 SEARCH/REPLACE，原子提交；见 [23 tools-fragment-edit.md](./23%20tools-fragment-edit.md) |
| `patch_file` | 应用补丁 | filesystem | medium | 启用 | unified diff / 结构化 hunk；见同上 |
| `skill_search` | Skill 搜索 | skill | low | 启用 | 搜索当前系统可用 Skill |
| `use_skill` | 使用 Skill | skill | low | 启用 | 标记本次运行使用某个 Skill |
| `memory_search` | Memory 搜索 | memory | low | 启用 | 搜索 Agent 长期记忆 |
| `memory_get` | Memory 读取 | memory | low | 启用 | 读取指定 memory 内容 |
| `read_image` | 图片理解 | media | medium | 启用 | 分析图片内容 |
| `read_document` | 文档理解 | media | medium | 启用 | 分析 PDF、Office、CSV 等文档 |
| `create_image` | 图片生成 | media | medium | 停用 | 根据文本提示生成图片 |
| `tts` | 文本转语音 | media | medium | 停用 | 将文本转换成语音文件 |
| `shell_exec` | Shell 命令 | runtime | critical | 停用 | 本机 shell；默认 cwd 与 **工作区根**一致；需确认 |
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

详情抽屉分为 5 个 Tab（**已实现**，见 `ToolDetailContent.vue`）：

| Tab | 内容 |
|-----|------|
| 概览 | 描述、分类、风险、来源、策略 chips、在线测试 |
| 参数 | `parameters_schema_json` 只读预览 |
| 配置 | `config_schema_json` 渲染的可编辑表单；`PUT /v1/tools/{id}/config` 保存 |
| Agent | allow / deny 覆盖、编辑覆盖弹窗 |
| 调用 | 最近 20 条调用记录、跳转完整记录 |

日常改 API Key / 超时：**详情「配置」Tab**，不必打开完整编辑弹窗。

### 5.5 编辑弹窗 UX（P0–P3）

实现位置：`web/src/components/tools/ToolEditorForm.vue` 及 `editor/*` 子组件；文案源 `web/src/features/tools/toolEditorCopy.ts`。

#### 5.5.1 信息架构（4 Tab）

| Tab | 内容 | 目标用户 |
|-----|------|----------|
| **基础** | Key、名称、描述、分类、来源、风险；新建时可选模板 | 所有人 |
| **运行策略** | 启用 / 只读 / 需确认 / 流式 / 并发（卡片 + 说明） | 运维 |
| **参数与配置** | Schema 字段构建器 + 配置可视化表单 | 注册 external Tool |
| **高级** | 默认配置、metadata、Raw JSON；配置 diff | 开发者 |

顶栏 **? 帮助** 打开侧栏抽屉（配置分层、字段词典）。

#### 5.5.2 运行策略开关语义

| 开关 | 含义 | 与 Agent 设置的区别 |
|------|------|---------------------|
| 全局启用 | 目录级启停 | 列表 toggle 同源 |
| 需确认 | 调用前用户确认 | ≠ Agent 覆盖「需确认」可叠加 |
| 流式 | 工具支持 StreamableCall | 实际流式还取决于 Agent `tools_streaming_enabled` |
| 并发 | 目录标记可并行 | 实际并行取决于 Agent `tools_parallel_enabled` + allow |

**builtin / 只读工具**：需确认、流式、并发由 `syncBuiltinToolsFromRegistry` 维护，UI 显示锁图标与警告；重启可能恢复 registry 默认值。

#### 5.5.3 JSON 字段说明

| 字段 | 作用 | 格式 |
|------|------|------|
| `parameters_schema_json` | LLM 可见调用参数 | JSON Schema object |
| `result_schema_json` | 返回结构文档（可选） | JSON Schema object |
| `config_schema_json` | 管理员配置项定义 | JSON Schema object |
| `config_json` | 当前配置值 | 符合 config_schema 的 JSON object |
| `default_config_json` | 出厂默认 | 同 config_json |
| `metadata_json` | OpenAPI URL、MCP 信息等 | JSON object |

#### 5.5.4 新建模板

| 模板 | 预填 |
|------|------|
| 空白 Tool | 默认 external / custom |
| REST 查询 | query 参数 + base_url / timeout 配置 Schema |
| OpenAPI | metadata.openapi_spec_url |

#### 5.5.5 分工（减少困惑）

| 操作 | 入口 |
|------|------|
| 启用 / 停用 | 列表 toggle |
| 改 API Key、超时 | 详情「配置」Tab |
| 改 Schema / 注册新 Tool | 编辑弹窗 |
| Agent 策略 | Agent 设置 → 平台工具策略 |

### 5.6 后续迭代（业务评审 2026-05-28）

> 背景：§5.5 编辑 UX（P0–P3）已落地；以下为业务评审识别的缺口与优先级。**实现状态**随 PR 更新。

#### 5.6.1 P1 — 降低语义误解

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| UX-01 | 策略 chip 降噪 | 详情 meta chip 改为「标记：需确认 / 流式 / 可并行」并附 tooltip，强调目录标记 ≠ 运行时必然生效 | ✅ |
| UX-02 | Agent Tab 生效摘要 | 汇总各 Agent 对该 Tool 的 effective_state（allowed / denied）及 override 数量；链到 Agent 能力 Tab | ✅ |
| UX-03 | Schema Builder 边界 | 字段视图顶部说明：仅支持扁平 `object.properties`；嵌套/array 请用 JSON 模式 | ✅ |
| UX-04 | 保存校验体验 | 编辑弹窗 JSON 校验失败自动跳转对应 Tab；详情「配置」保存前本地 `JSON.parse` | ✅ |

#### 5.6.2 P2 — 闭环与硬约束

| ID | 项 | 说明 | 状态 |
|----|-----|------|------|
| UX-05 | 新建后测试引导 | external Tool 首次保存成功后提示「打开详情 → 在线测试」 | ✅ |
| UX-06 | builtin 策略后端只读 | `syncBuiltinToolsFromRegistry` 覆盖字段在后端拒绝写入，避免 UI「假成功」 | 待做 |
| UX-07 | 工具调用审计 | §0.1 后续项；与 Tools 页「审计日志」路由联动 | 待做 |

#### 5.6.3 分工不变（§5.5.5 延续）

| 用户问题 | 正确入口 |
|----------|----------|
| 「开了流式/并行为什么没效果？」 | Agent 设置 → 能力 Tab（`tools_streaming_enabled` / `tools_parallel_enabled`） |
| 「哪些 Agent 能用这个 Tool？」 | 详情 → Agent Tab 生效摘要 + 覆盖列表 |
| 「注册完怎么验证？」 | 详情 → 概览 → 在线测试（新建后 UX-05 引导） |

#### 5.6.4 实现位置

| 能力 | 路径 |
|------|------|
| chip 文案 / tooltip | `web/src/features/tools/toolEditorCopy.ts` |
| Agent 生效汇总 | `web/src/features/tools/toolAgentBindingSummary.ts` |
| JSON Tab 映射 | `web/src/components/tools/toolUi.ts` |
| 详情 Agent 摘要 UI | `web/src/components/tools/ToolDetailContent.vue` |
| Schema 边界 banner | `web/src/components/tools/editor/ToolSchemaBuilder.vue` |

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
| **片段级文件编辑** | ✅ **P1** | Phase 4 已实现；[changelog](../changelog/2026-05-22-Tools-Phase4-Fragment-Edit.md) |
| 工具在线测试 | P3 | 自定义工具可在配置时在线测试 |
| 工具调用审计日志 | P3 | `tool_invocation_audit` 表 + 查询 API |
| 工作区搜索增强 | P1 | `workspace_search` 字面检索工具（P0-WS） |
| 代码沙箱执行 | P2 | E2B / Jupyter / Container 沙箱 |
| 多渠道通知 | P2 | 统一通知接口（邮件/IM/Webhook） |
| 数据库查询工具 | P3 | 安全 SQL 查询（只读，白名单表） |
| 工作流编排工具 | P3 | 与 Graph Workflow 模块对齐 |
| **工具依赖图管理** | **P2** | **BabyAGI 启发——`ToolRegistration` 增加 Dependencies 字段，Assemble 拓扑排序，前端依赖图可视化** |
| **工具自构建与自进化** | **P2** | **BabyAGI 启发——Agent 运行时动态工具生成 + 持久化为 Skill + 安全审查** |

### 10.1 BabyAGI 启发：工具依赖图与自构建

> 来源：BabyAGI functionz 框架（GitHub 22k+ stars），竞品分析差距 #8
> 对应需求：`docs/competitive-gap-requirements-2026-05-31.md` P2-8/P2-9

BabyAGI 的 `functionz` 框架以图结构追踪函数间的 import/依赖/密钥关系，并实现了自构建能力（`process_user_input` + `self_build`）。这两个设计思想对 Tools 模块有直接启发：

**依赖图管理**（P2-9）：
- 当前 `ToolRegistration` 已有 `Category/Tags/RiskLevel`，但无 `Dependencies` 字段
- 工具间依赖关系是隐式的（如 `email` 依赖 `httpfetch` 获取附件），`Assemble()` 按顺序装配但不保证依赖顺序
- 借鉴 BabyAGI 的做法：`ToolRegistration` 增加 `Dependencies []string` 字段，`Assemble()` 时自动拓扑排序
- 前端增加工具依赖图可视化视图（类似 BabyAGI functionz Dashboard）

**工具自构建**（P2-8）：
- 当前工具通过 `Registry()` 静态注册 + `Assemble()` 装配，运行时工具集固定
- 借鉴 BabyAGI 的 `process_user_input`：Agent 运行时检测到工具缺口后，通过 LLM 生成新工具代码（`function.NewFunctionTool[I, O]`），注册到 `Registry` 并持久化为 Skill
- 动态生成的工具必须经过风险分级 + 人工审批（不可自动启用），与 Skill 自创建（P2-2）共享审批流程


---

## 子模块：Tools Fragment Edit

> **版本**：1.1 | **状态**：✅ 已实现（2026-05-22）
> **所属模块**：**Tools（23）** — `filesystem` 分类运行时能力增强
> **设计**：[23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) · **开发计划**：[23-tools-development.md §Phase 4](./23-tools-development.md#phase-4片段级文件编辑p1)
> **父文档**：[23 tools.md](./23%20tools.md) · [23 tools.design.md](./23%20tools.design.md)

---

## 0. 模块归属

| 维度 | 归属 |
|------|------|
| **产品模块** | Tools 工具管理（编号 23） |
| **工具分类** | `filesystem` |
| **框架实现** | `pkg/trpc-agent-go/tool/file`（trpc-agent-go 框架层，真相源） |
| **项目桥接** | `internal/tools`（Registry / Assemble / 别名）、`internal/data`（catalog seed）、`internal/agent`（Prompt 工作流提示） |
| **不涉及** | Proto RPC 变更、新数据库表、前端新页面（仅 catalog 展示同步） |

与 Agent 创建/设置（2/5）、Channel、Memory 等模块无直接耦合；生效路径为 **Agent 工具装配 → 运行时 ToolSet**。

---

## 1. 背景与问题

### 1.1 现状

默认 `file` ToolSet 提供：

| 工具 | 行为 | 问题 |
|------|------|------|
| `save_file` | 整文件覆盖写入 | 大文件 token 高、易截断、误改无关行 |
| `replace_content` | 单次精确 `old_string` → `new_string` | 一处修改一次调用；无会话缓存；整文件读写 |

Agent 系统提示已引导 `search_content → read_file → replace_content/save_file`，但缺少 **Cursor 式片段编辑**：只传变更块、单文件多处修改一次提交、同会话减少重复读盘。

### 1.2 目标

| 目标 | 说明 |
|------|------|
| **降 token** | 模型只输出变更片段，不传整文件 |
| **降往返** | 单文件 N 处修改合并为 1 次工具调用 |
| **提速** | 同会话内 read/edit 命中内存缓存，磁盘 1 读 1 写 |
| **可诊断** | 匹配失败返回行号、上下文 snippet，便于模型 self-heal |

### 1.3 非目标（本期不做）

- 跨文件原子事务（多文件各自独立提交）
- 在线 diff 可视化 UI
- 替代 `claudecode` ToolSet（Bash / NotebookEdit 等仍走 claudecode）
- 二进制文件、`.ipynb` 的片段编辑
- 基于 LSP / AST 的结构化重构

---

## 2. 用户故事

### US-FE-1：Agent 片段编辑大文件

**作为** 使用 coding Agent 的开发者，**我希望** Agent 修改 50KB+ 源文件时只提交变更片段，**以便** 响应更快且不易因 token 截断写坏文件。

**验收**：

- Agent 可通过 `diff_edit` 一次提交同一文件的多个 `search`/`replace` 块
- 不使用 `save_file` 覆盖已有大文件（工具描述与 Prompt 明确约束）
- 单次编辑工具参数体积显著小于整文件（见 §5 量化指标）

### US-FE-2：Agent 应用 unified diff

**作为** Agent，**我希望** 在已生成标准 diff 时直接应用，**以便** 不必把 diff 手工转成 search/replace 块。

**验收**：

- Agent 可通过 `patch_file` 传入 unified diff 或结构化 hunk 列表
- 任一 hunk 与磁盘内容不一致时整次失败且不落盘
- 失败响应包含 hunk 索引与 expected/actual 行摘要

### US-FE-3：同会话连续编辑

**作为** Agent，**我希望** 对刚读过的文件再次编辑时不必重复读盘，**以便** 多轮修改更流畅。

**验收**：

- 同 invocation 内：`read_file` 后 `diff_edit`/`patch_file` 优先使用会话缓存
- 写盘后缓存更新，后续编辑不再额外 `ReadFile`
- 外部进程修改文件（mtime 变化）时拒绝静默覆盖，提示 re-read

### US-FE-4：平台管理员识别新工具

**作为** 平台使用者，**我希望** 在 Tools  catalog 中看到 `diff_edit` / `patch_file` 及风险级别，**以便** 通过 Agent allow/deny 控制暴露范围。

**验收**：

- `builtin_tools_seed` 含两条 catalog 记录，分类 `filesystem`，风险 `medium`
- Effective Tools / Agent 工具矩阵包含新 key
- 活动流展示中文标签（如「片段编辑」「应用补丁」）

---

## 3. 功能规格

### 3.1 新增工具：`diff_edit`

| 项 | 规格 |
|----|------|
| **用途** | 对已有文本文件施加 1～N 处片段替换 |
| **必填参数** | `file_name`；`edits[]`（每项含 `search`、`replace`） |
| **可选参数** | `replace_all`（per-edit）；`expected_mtime_ms`（乐观并发） |
| **默认策略** | 每处 `search` 须唯一匹配，否则报错 |
| **原子性** | 所有 edit 在内存校验通过后一次写盘；任一失败则不写 |
| **新建文件** | 仅当 `search` 为空且目标不存在或为空文件时允许 |
| **默认启用** | 是（与 `read_file` 同属 filesystem profile） |

### 3.2 新增工具：`patch_file`

| 项 | 规格 |
|----|------|
| **用途** | 应用 unified diff 或结构化 hunk 列表 |
| **输入模式** | `patch`（字符串）与 `hunks[]`（结构化）二选一 |
| **必填参数** | `file_name`；`patch` 或 `hunks` |
| **可选参数** | `expected_mtime_ms` |
| **校验** | hunk 的删除行须与当前文件逐行一致（含 context 行） |
| **原子性** | 同 `diff_edit` |
| **默认启用** | 是 |

### 3.3 会话文件缓存（Fast Path）

| 项 | 规格 |
|----|------|
| **范围** | 单次 Agent invocation 内，按绝对路径缓存文本内容与 mtime |
| **写入时机** | `read_file` 成功后；`diff_edit` / `patch_file` / `save_file` 写盘后 |
| **失效** | 磁盘 mtime 与缓存不一致；invocation 结束 |
| **不替代** | Tool 结果缓存（`metadata_json.cache_enabled`，只读幂等工具） |

### 3.4 与现有工具的分工

| 场景 | 推荐工具 |
|------|----------|
| 新建文件 | `save_file` |
| 极小文件全量重写（如 <100 行配置） | `save_file` |
| 修改已有文件（默认） | `diff_edit` |
| 已有 unified diff | `patch_file` |
| 单次简单替换（兼容） | `replace_content`（`edit_file` 别名） |

### 3.5 编辑工作流（Prompt 约束）

推荐顺序：

```
search_content（定位）
  → read_file（大文件用 start_line / num_lines）
  → diff_edit（默认）或 patch_file（已有 diff）
  → 失败则 re-read 并重试
```

禁止：用 `save_file` 修改已有中大型源文件。

---

## 4. 产品决策

| 决策项 | 默认值 |
|--------|--------|
| 风险级别 | `medium`（与 `save_file` / `replace_content` 一致） |
| 二次确认 | 否（除非 Agent Override 或 profile 另行要求） |
| `edit_file` 别名 | Phase 1 仍指向 `replace_content`；Phase 2 可选迁移至 `diff_edit` |
| 并行执行 | 支持（`SupportsConcurrency: true`，与 file ToolSet 一致） |
| 大文件策略 | 先 `read_file` 分段；Phase 2 可选行区间 patch（>1MB） |
| claudecode | 不合并；需要 Bash/Notebook 时继续启用 claudecode ToolSet |

---

## 5. 验收标准

### 5.1 功能

- [x] `diff_edit` 可在单次调用中应用 ≥2 处非连续替换且原子提交
- [x] `patch_file` 可应用标准 unified diff；hunk mismatch 时零副作用
- [x] 同 invocation 内第二次编辑同一文件不触发额外磁盘读（mtime 未变）
- [x] 外部修改文件后编辑被拒绝并提示 re-read（`expected_mtime_ms` / cache mtime）
- [x] `read_file` 响应含 `mtime_ms`，供编辑工具乐观锁
- [x] catalog、Effective Tools、testexec 映射、Activity 中文标签齐全
- [x] `replace_content` / `save_file` 保持可用，无破坏性变更

### 5.2 性能（量化）

| 指标 | 目标 |
|------|------|
| 单文件 3 处修改 tool 调用数 | ≤2（1 read + 1 diff_edit） |
| 50KB 文件编辑 LLM 输出 | ≪ 50KB（片段级，通常 <2KB） |
| 单次工具执行（I/O） | ≤10ms 量级（1 读 + 内存 patch + 1 写，不含 LLM） |

### 5.3 安全

- [x] 路径校验与现有 `file` ToolSet 一致（禁止 `..`、绝对路径）
- [x] 拒绝二进制与 `.ipynb`
- [x] 单次 patch / search / replace 体积上限（见设计文档常量）

---

## 6. 与相关文档边界

| 文档 | 本文关系 |
|------|----------|
| [23 tools.md](./23%20tools.md) | 父需求；catalog 总表由父文档索引，细节在本文 |
| [23 tools-fragment-edit.design.md](./23%20tools-fragment-edit.design.md) | 技术方案、Schema、分层、代码锚点 |
| [23-tools-development.md](./23-tools-development.md) | 任务拆分、Phase、实现差距（**进度真相**） |
| [guides/trpc-agent-go-framework.md](../guides/trpc-agent-go-framework.md) | 框架 Tool / ToolSet 通用约定 |
| [guides/AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) | 分层红线；实现不得违反 biz 不 import trpc-agent-go |

---

*文档版本：1.0 — 片段级文件编辑产品需求；实现状态以 [23-tools-development.md](./23-tools-development.md) 为准。*
