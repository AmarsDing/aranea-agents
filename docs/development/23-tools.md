# Tools 工具管理 — 产品需求

> **版本**：5.1 | **状态**：核心已实现；片段编辑 catalog 已就绪、运行时工具待补；工作区统一已实现
> **设计**：[23-tools.design.md](./23-tools.design.md) · **开发计划**：[23-tools.development.md](./23-tools.development.md)

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| Tool 列表 | 已实现 | 展示内置 Tool、MCP Tool、系统注册 Tool 的启用状态、分类、风险级别、schema、最近调用 |
| Tool 详情 | 已实现 | 展示工具描述、参数 schema、返回结构、配置、Agent 覆盖、最近运行 |
| 启用 / 停用 | 已实现 | 支持全局启停；Agent 级可通过 allow / deny 覆盖；高风险工具需二次确认 |
| Agent 绑定 | 已实现 | 基于现有 `agent_runtime_settings` 的 allow / deny / profile 管理 |
| 参数配置 | 已实现 | Tool 自身配置与 Agent 覆盖配置分开管理 |
| 调用记录 | 已实现 | 记录 tool call 参数摘要、参数 hash、结果摘要、耗时、状态、错误 |
| 参数详情 | 已实现 | 管理员可查看脱敏后参数；敏感字段默认不明文落库 |
| 使用统计 | 已实现 | 调用次数、成功率、平均耗时、最近调用、按 Agent 分布 |
| Agent 工具覆盖 | 已实现 | Agent 级覆盖表支持单 Tool 粒度的 config override / allow / deny |
| Tool 配置更新 | 已实现 | 独立更新工具配置 |
| 动态上传 Tool 代码 | 否 | 不允许前端上传任意 Go / JS 代码作为 Tool |
| MCP Tool 发现 | 后续增强 | 已注册 MCP 工具可展示；MCP Broker 运行时发现已实现 |
| 工具在线测试 | 已实现 | TestTool RPC + 工具详情「在线测试」 |
| 工具调用审计日志 | 已实现 | 审计表 + ListToolInvocationAudits API；前端审计页已实现 |
| 片段级文件编辑 | 已实现 | `diff_edit` / `patch_file` catalog 种子 + 运行时工具均已实现，种子启用 |
| 工具工作区统一 | 已实现 | file / shell / claude_code 共用 `workspace_root`；详见设计文档 §7.8 |

> 进度详情与任务状态见 [23-tools.development.md](./23-tools.development.md)。

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 路由 | `/tools` 为 Tools 管理页；`/tools/runs` 为调用记录页；`/tools/audits` 为审计页 |
| 管理对象 | 内置 Tool + 后端注册 Tool + 可发现 MCP Tool |
| 工具来源 | `builtin` / `mcp` / `system` / `external` |
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
| 启用 / 停用 Tool | 允许；高风险工具需要二次确认（`confirm_intent` = `I_UNDERSTAND_RISK`） |
| 修改 Tool 全局配置 | 允许；配置必须按后端 schema 校验 |
| 修改 Agent 绑定 | 允许；写入 `agent_runtime_settings` 或 Agent 级覆盖表 |
| 查看调用记录 | 允许；默认展示脱敏摘要 |
| 查看参数详情 | 允许查看脱敏 preview、hash、类型、敏感标记；不展示明文 secret |
| 重放 Tool 调用 | 后续迭代；本期不提供，避免误触发写入 / 外部发送 |

安全控制不依赖"角色"实现，而是由后端按工具风险、字段敏感度和执行环境控制：

- 高风险工具启用和执行前需要显式确认（`confirm_intent` 字段，值为 `I_UNDERSTAND_RISK`）。
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
| Agent Tool 覆盖 | Agent 级覆盖表 | 单 Tool 粒度的 Agent 级 config override / allow / deny | 高 |
| 单次调用上下文 | Runtime context | session、user、workspace、request_id 等系统注入参数 | 最高 |

模型只能控制 `parameters_schema_json` 中声明为可见的字段。`tenant_id`、`agent_id`、`workspace_id`、`session_id`、`request_id` 这类系统字段由后端注入，不进入模型可写参数。

### 2.1 工具工作区（Workspace Root）

Agent 运行时对「需要目录」的工具共用 **单一工作区根** `workspace_root`（Cursor 式：Agent 在用户选定的项目目录内读写文件、跑 shell）。

| 决策项 | 默认值 |
|--------|--------|
| 工作区含义 | 绝对路径目录；file 工具路径均相对或受限于此根 |
| file 工具 | 严格限制在工作区内 |
| `shell_exec` | 默认 cwd = 工作区根；调用参数 `workdir` 可指定子目录或（在 OS 权限内）绝对路径 |
| `claude_code` | 未单独配置时与工作区根相同 |
| 不需工作区的工具 | web 搜索/抓取、email、todo、MCP、memory、knowledge、spirit、browser(MCP 桥接) 等 |
| `workspace_exec` | 依赖 CodeExecutor 工作区，与 file/shell 根目录 **可能不同**；不替代日常 `shell_exec` |

**验收**：同一 turn 内 `save_file` 写入的文件，紧随其后的 `exec_command` 可在工作区内访问；shell 不再落在 Server 进程当前目录。

> 工作区解析链、装配改动、工具×目录矩阵等技术设计见 [23-tools.design.md §7.8](./23-tools.design.md#78-工具工作区统一phase-5)。

---

## 3. Tool 分类与内置工具

### 3.1 分类

| 分类 | 示例 | 风险 |
|------|------|------|
| `system` | `datetime`、`todo_write` | 低风险 |
| `filesystem` | `read_file`、`save_file`、`list_file`、`replace_content`、`search_content`、`diff_edit`、`patch_file` | 读低风险，写中高风险 |
| `web` | `web_research`、`web_fetch`、`duckduckgo_search`、`gemini_web_fetch`、`google_search`、`arxiv_search`、`wikipedia_search` | 中风险 |
| `search` | 同 web 分类搜索工具 | 低中风险 |
| `memory` | `memory_search`、`memory_get`、`working_memory.*` | 低中风险 |
| `skill` | `skill_search`、`use_skill` | 低中风险 |
| `media` | `read_image`、`read_document`、`create_image`、`tts`、`read_spreadsheet` | 成本与隐私风险 |
| `session` | `await_user_reply` | 低风险 |
| `runtime` | `shell_exec`、`claude_code`、`workspace_exec` | 高风险 |
| `messaging` | `send_email` | 高风险 |
| `integration` | `mcp_tool_set`、`mcp_broker`、`openapi`、`kanban`、`call_agent`、`knowledge_search`、`knowledge_reflect` | 中高风险 |
| `composition` | `call_agent`（Agent-as-Tool） | 中风险 |
| `knowledge` | `knowledge_search` | 低风险 |
| `browser` | `browser` | critical |
| `spirit` | `plan_and_execute`、`cancel_orchestration`、`synthesize_results`、`build_orchestration_graph` | 低中风险 |

### 3.2 内置工具 catalog

> 完整 catalog 与种子同步见设计文档 §4.3 与开发计划代码锚点。下表为用户视角的工具清单。

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
| `diff_edit` | 片段编辑 | filesystem | medium | 启用 | 多片段 SEARCH/REPLACE，原子提交 |
| `patch_file` | 应用补丁 | filesystem | medium | 启用 | unified diff / 结构化 hunk |
| `skill_search` | Skill 搜索 | skill | low | 启用 | 搜索当前系统可用 Skill |
| `use_skill` | 使用 Skill | skill | low | 启用 | 标记本次运行使用某个 Skill |
| `memory_search` | Memory 搜索 | memory | low | 启用 | 搜索 Agent 长期记忆 |
| `memory_get` | Memory 读取 | memory | low | 启用 | 读取指定 memory 内容 |
| `read_image` | 图片理解 | media | medium | 停用 | 分析图片内容 |
| `read_document` | 文档理解 | media | medium | 启用 | 分析 PDF、Office、CSV 等文档 |
| `read_spreadsheet` | 表格读取 | media | medium | 启用 | 读取 XLSX、CSV 等表格文件 |
| `create_image` | 图片生成 | media | medium | 停用 | 根据文本提示生成图片 |
| `tts` | 文本转语音 | media | medium | 停用 | 将文本转换成语音文件 |
| `shell_exec` | Shell 命令 | runtime | critical | 停用 | 本机 shell；默认 cwd 与工作区根一致；需确认 |
| `send_email` | 邮件发送 | messaging | high | 停用 | 发送电子邮件，需确认 |
| `todo_write` | 待办管理 | system | low | 启用 | 管理待办事项列表 |
| `await_user_reply` | 等待回复 | session | low | 启用 | 暂停执行并等待用户回复 |
| `claude_code` | Claude Code | runtime | high | 停用 | 代码编辑和执行，需确认 |
| `workspace_exec` | 工作区执行 | runtime | high | 停用 | 在工作区中执行命令，需确认 |
| `working_memory_read` | 工作记忆读取 | memory | low | 启用 | 读取当前任务的结构化工作记忆 |
| `working_memory_list` | 工作记忆列表 | memory | low | 启用 | 列出当前任务下所有可见字段 |
| `working_memory_write` | 工作记忆写入 | memory | low | 启用 | 向当前任务写入或更新字段 |
| `working_memory_patch` | 工作记忆批量补丁 | memory | low | 启用 | 一次写入多个字段 |
| `working_memory_delete` | 工作记忆删除 | memory | low | 启用 | 删除当前任务下的一个字段 |
| `browser` | 浏览器自动化 | browser | critical | 停用 | Playwright MCP 桥接；需确认 |
| `model_registry_sync` | 模型目录同步 | system | medium | 停用 | 同步模型注册表信息 |
| `read_tool_result` | 读取工具结果 | system | low | 启用 | 读取延迟工具的执行结果（deferred 通道） |
| `kanban` | Kanban 任务板 | integration | medium | 启用 | Graph 任务看板工具集 |
| `call_agent` | 调用 Agent | integration | medium | 启用 | Agent-as-Tool 委托 |
| `knowledge_search` | 知识库搜索 | integration | low | 启用 | 在指定知识库集合中检索 |
| `knowledge_reflect` | 知识库反思 | integration | low | 停用 | 跨知识库检索并评估结果质量 |
| `mcp_tool_set` | MCP 工具集 | integration | medium | 停用 | 挂载已配置的 MCP Server 工具 |
| `mcp_broker` | MCP Broker | integration | medium | 停用 | 运行时 MCP 发现与调用 |
| `plan_and_execute` | 规划并执行 | spirit | low | 启用 | 编排式计划与执行 |
| `cancel_orchestration` | 取消编排 | spirit | medium | 启用 | 取消正在运行的编排 |
| `synthesize_results` | 合成团队结果 | spirit | low | 启用 | 将所有已完成团队的执行结果合成为综合报告 |
| `build_orchestration_graph` | 构建编排图 | spirit | low | 启用 | 构建 DAG 编排图，定义子任务依赖关系 |

> `subagents_spawn/list/get/cancel`、`message` 已入种子表（默认停用），上表从略；框架注册细节见设计文档 §7.2。

---

## 4. 信息架构与路由

| 路由 | 页面 | 说明 |
|------|------|------|
| `/tools` | Tools 管理 | 列表、启停、分类筛选、配置入口、Agent 绑定入口 |
| `/tools/:id` | Tool 详情 | schema、配置、调用示例、Agent 覆盖、最近调用 |
| `/tools/runs` | Tool 调用记录 | 按 Tool / Agent / Session / 状态 / 时间筛选调用明细 |
| `/tools/audits` | Tool 调用审计 | 按 Tool / Agent / User / Session / 状态 / 时间筛选审计记录 |
| `/agents/:id/tools` | Agent 工具配置 | 嵌入 Agent 设置页，编辑 profile、allow、deny、并发许可 |

侧栏放在管理类导航下，文案为「Tools 管理」「Tool 调用记录」「Tool 调用审计」。

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
| 主表 | 服务端分页，支持按调用量、失败率、最近调用排序 |
| 详情抽屉 | 点击行打开右侧抽屉查看配置与 schema |

搜索行为：

- 搜索字段匹配 `key`、`display_name`、`description`、`category`。
- 输入框使用 `debounce="300"`。
- 搜索、筛选、每页条数变化后重置到第 1 页。

### 5.2 表格列

| 列 | 字段 | UI 与交互 |
|----|------|-----------|
| 名称 | `display_name`、`key` | 主行展示名称，副行展示 key；内置工具带 `builtin` chip |
| 分类 | `category` | chip，按分类上色 |
| 来源 | `source` | `builtin` / `mcp` / `system` / `external` |
| 风险 | `risk_level` | `low` 灰绿，`medium` 黄，`high` 橙，`critical` 红 |
| 启用 | `enabled` | Toggle dense；无权限或内置锁定时禁用 |
| Agent 覆盖 | `agent_override_count` | 展示有 allow / deny 覆盖的 Agent 数 |
| 使用频率 | `invoke_count`、`invoke_count_24h` | 总调用 + 近 24 小时调用 |
| 成功率 | `success_count`、`failure_count` | 展示成功率，低于阈值标红 |
| 耗时 | `avg_duration_ms`、`p95_duration_ms` | 平均与 P95 |
| 最近调用 | `last_invoked_at`、`last_status` | 无调用展示「未调用」 |
| 操作 | `permissions` | 详情、配置、绑定 Agent、查看调用 |

### 5.3 启用 / 停用行为

1. 用户切换 Toggle。
2. 前端进入行级 loading，不立即确认最终状态。
3. 调用 `PATCH /v1/tools/{id}/enabled`。
4. 成功后更新该行和概览卡片。
5. 失败后恢复原状态并提示错误。

高风险工具启用时必须二次确认：

- 弹窗展示工具名、风险级别、影响范围。
- 若工具属于 `runtime`、`messaging`、`external`，要求输入 `I_UNDERSTAND_RISK` 确认（`confirm_intent` 字段）。
- 后端仍需做权限校验，不能只依赖前端确认。

### 5.4 详情抽屉

详情抽屉分为 5 个 Tab：

| Tab | 内容 |
|-----|------|
| 概览 | 描述、分类、风险、来源、策略 chips、在线测试 |
| 参数 | `parameters_schema_json` 只读预览 |
| 配置 | `config_schema_json` 渲染的可编辑表单；`PUT /v1/tools/{id}/config` 保存 |
| Agent | allow / deny 覆盖、编辑覆盖弹窗、生效摘要 |
| 调用 | 最近 20 条调用记录、跳转完整记录 |

日常改 API Key / 超时：**详情「配置」Tab**，不必打开完整编辑弹窗。

> 详情抽屉组件设计与 Tab 实现见设计文档 §8.4。

### 5.5 编辑弹窗 UX（P0–P3）

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

**builtin / 只读工具**：需确认、流式、并发由后端同步维护，UI 显示锁图标与警告；重启可能恢复 registry 默认值。

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

#### 5.6.1 P1 — 降低语义误解

| ID | 项 | 说明 |
|----|-----|------|
| UX-01 | 策略 chip 降噪 | 详情 meta chip 改为「标记：需确认 / 流式 / 可并行」并附 tooltip，强调目录标记 ≠ 运行时必然生效 |
| UX-02 | Agent Tab 生效摘要 | 汇总各 Agent 对该 Tool 的 effective_state（allowed / denied）及 override 数量；链到 Agent 能力 Tab |
| UX-03 | Schema Builder 边界 | 字段视图顶部说明：仅支持扁平 `object.properties`；嵌套/array 请用 JSON 模式 |
| UX-04 | 保存校验体验 | 编辑弹窗 JSON 校验失败自动跳转对应 Tab；详情「配置」保存前本地 `JSON.parse` |

#### 5.6.2 P2 — 闭环与硬约束

| ID | 项 | 说明 |
|----|-----|------|
| UX-05 | 新建后测试引导 | external Tool 首次保存成功后提示「打开详情 → 在线测试」 |
| UX-06 | builtin 策略后端只读 | 后端同步覆盖字段时拒绝写入，避免 UI「假成功」 |
| UX-07 | 工具调用审计 | 与 Tools 页「审计日志」路由联动 |

#### 5.6.3 分工不变（§5.5.5 延续）

| 用户问题 | 正确入口 |
|----------|----------|
| 「开了流式/并行为什么没效果？」 | Agent 设置 → 能力 Tab（`tools_streaming_enabled` / `tools_parallel_enabled`） |
| 「哪些 Agent 能用这个 Tool？」 | 详情 → Agent Tab 生效摘要 + 覆盖列表 |
| 「注册完怎么验证？」 | 详情 → 概览 → 在线测试（新建后 UX-05 引导） |

> UX 项的实现位置与状态见开发计划。

---

## 6. Agent 工具配置

当前系统已有 `agent_runtime_settings` 工具字段，继续复用，避免重复建一套 Agent 工具策略表。

### 6.1 用户视角配置项

| 配置 | 说明 |
|------|------|
| 工具总开关 | Agent 是否启用工具调用 |
| Profile 预设 | `chat_only` / `read_only` / `coding` / `research` / `full` / `minimal` / `safe` / `system_admin` / `spirit` |
| Allow 列表 | Agent 明确允许的工具 key 或 `group:filesystem` |
| Deny 列表 | Agent 明确禁止的工具 key 或工具组 |
| 并发许可 | 允许并发执行的工具 key |
| 并行开关 | 是否允许并行工具调用 |
| 流式开关 | 是否启用流式工具 |
| 重试策略 | 是否启用重试、最大次数、初始间隔、退避因子、最大间隔、抖动 |

> 字段名与数据模型见设计文档 §3.1 与 §6.1。

### 6.2 Profile 预设

| Profile | 包含工具组 |
|---------|-----------|
| `chat_only` | 无工具 |
| `read_only` | `datetime`、`read_file`、`read_multiple_files`、`list_file`、`search_file`、`search_content`、`todo_write` |
| `coding` | `group:filesystem`、`group:web`、`group:skill`、`group:session`、`datetime` |
| `research` | 搜索 + 文件读取 + skill + memory + `datetime` |
| `full` | 全部工具组 |
| `minimal` | 无工具（最简模式） |
| `safe` | `datetime`、`read_file`、`read_multiple_files`、`list_file`、`search_file`、`search_content`、`todo_write` |
| `system_admin` | `group:cli_admin`、`web_fetch`、`datetime` |
| `spirit` | `plan_and_execute`、`cancel_orchestration`、`synthesize_results`、`build_orchestration_graph`、`memory_search`、`group:subagent`、`shell_exec`、`datetime` |

### 6.3 Agent 页 UI

在 Agent 详情页新增「Tools」Tab：

| 区域 | 需求 |
|------|------|
| 总开关 | 工具总开关 |
| Profile | 下拉选择，展示 profile 说明和包含的工具组 |
| Allow | 多选，选择工具或 `group:filesystem` |
| Deny | 多选，选择工具或工具组 |
| 并发许可 | 多选，只允许选择幂等 / 读类工具 |
| 预览 | 展示最终可用工具列表，按后端 `GET /v1/agents/{id}/tools/effective` 返回的结果 |

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

## 8. Tool 调用审计页

### 8.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Tool 调用审计」 |
| 筛选 | Tool、Agent、User、Session、状态、时间范围 |
| 主表 | 服务端分页 |

### 8.2 表格列

| 列 | 字段 | 说明 |
|----|------|------|
| 时间 | `created_at` | 本地化显示 |
| Tool | `tool_key` | |
| Agent | `agent_id` | |
| User | `user_id` | |
| Session | `session_id` | |
| 动作 | `action` | 如 `tool.call` |
| 结果摘要 | `result_summary` | 单行截断 |
| 状态 | `status` | success / error / blocked |

> 审计日志默认保留 90 天，由后端定时清理。

---

## 9. 与现有模块的关系

| 模块 | 关系 |
|------|------|
| Skill | `skill_search` / `use_skill` 的调用记录可关联 Skill 运行记录 |
| Plugin | Plugin 负责运行链路回调；Tool Callbacks / Adapter 负责权威调用记录 |
| Monitor | Tool 调用可同步发实时事件，Monitor 页面展示 `tool.call` / `tool.result` |
| Agent 设置 | 复用 `agent_runtime_settings` 的工具策略字段 |
| Knowledge | `knowledge_search` 工具与知识库模块联动 |
| MCP | MCP ToolSet / Broker 通过 Agent 级覆盖和 Agent 设置管理 |

---

## 10. 前端状态与交互细节

### 10.1 状态展示

| 状态 | UI |
|------|----|
| 首次加载 | Skeleton 或 InnerLoading |
| 空列表 | Banner +「同步内置工具」 |
| 搜索无结果 | 文案「没有匹配的 Tool」+「重置筛选」 |
| 请求失败 | Banner negative +「重试」 |
| 无权限 | 按钮禁用，Tooltip 展示原因 |
| 高风险开启 | Dialog 二次确认 |

### 10.2 可访问性

- 所有 icon-only 按钮必须有 `aria-label`。
- Toggle 需要明确 label，不只依赖颜色表达启用状态。
- 风险级别除颜色外，还必须显示文字。
- 详情抽屉打开后焦点进入标题，关闭后回到触发按钮。
- 调用记录错误信息使用可复制文本，不只放 tooltip。

---

## 11. 后续需求

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 工作区搜索增强 | P1 | `workspace_search` 字面检索工具 |
| 代码沙箱执行 | P2 | E2B / Jupyter / Container 沙箱 |
| 多渠道通知 | P2 | 统一通知接口（邮件/IM/Webhook） |
| 数据库查询工具 | P3 | 安全 SQL 查询（只读，白名单表） |
| 工作流编排工具 | P3 | 与 Graph Workflow 模块对齐 |
| **工具依赖图管理** | **P2** | `ToolRegistration` 增加 Dependencies 字段，Assemble 拓扑排序，前端依赖图可视化 |
| **工具自构建与自进化** | **P2** | Agent 运行时动态工具生成 + 持久化为 Skill + 安全审查 |

### 11.1 BabyAGI 启发：工具依赖图与自构建

BabyAGI 的 `functionz` 框架以图结构追踪函数间的 import/依赖/密钥关系，并实现了自构建能力。这两个设计思想对 Tools 模块有直接启发：

**依赖图管理**（P2）：
- 当前 `ToolRegistration` 已有 `Category/Tags/RiskLevel`，但无 `Dependencies` 字段
- 工具间依赖关系是隐式的（如 `email` 依赖 `httpfetch` 获取附件），`Assemble()` 按顺序装配但不保证依赖顺序
- 借鉴 BabyAGI 的做法：`ToolRegistration` 增加 `Dependencies []string` 字段，`Assemble()` 时自动拓扑排序
- 前端增加工具依赖图可视化视图

**工具自构建**（P2）：
- 当前工具通过 `Registry()` 静态注册 + `Assemble()` 装配，运行时工具集固定
- 借鉴 BabyAGI 的 `process_user_input`：Agent 运行时检测到工具缺口后，通过 LLM 生成新工具代码，注册到 `Registry` 并持久化为 Skill
- 动态生成的工具必须经过风险分级 + 人工审批（不可自动启用），与 Skill 自创建共享审批流程

---

## 子模块：Tools Fragment Edit

> **版本**：1.2 | **所属模块**：**Tools（23）** — `filesystem` 分类运行时能力增强
> **设计**：[23-tools.design.md §十三](./23-tools.design.md#十三片段级文件编辑扩展) · **开发计划**：[23-tools.development.md §Phase 4](./23-tools.development.md#phase-4片段级文件编辑p1)

---

## 0. 模块归属

| 维度 | 归属 |
|------|------|
| **产品模块** | Tools 工具管理（编号 23） |
| **工具分类** | `filesystem` |
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

**作为** 平台使用者，**我希望** 在 Tools catalog 中看到 `diff_edit` / `patch_file` 及风险级别，**以便** 通过 Agent allow/deny 控制暴露范围。

**验收**：

- catalog 含两条记录，分类 `filesystem`，风险 `medium`
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
| `edit_file` 别名 | 指向 `diff_edit`（policy 与 runtime alias 已同步） |
| 并行执行 | 支持（与 file ToolSet 一致） |
| 大文件策略 | 先 `read_file` 分段；后续可选行区间 patch（>1MB） |
| claudecode | 不合并；需要 Bash/Notebook 时继续启用 claudecode ToolSet |

---

## 5. 验收标准

### 5.1 功能

- `diff_edit` 可在单次调用中应用 ≥2 处非连续替换且原子提交
- `patch_file` 可应用标准 unified diff；hunk mismatch 时零副作用
- 同 invocation 内第二次编辑同一文件不触发额外磁盘读（mtime 未变）
- 外部修改文件后编辑被拒绝并提示 re-read（`expected_mtime_ms` / cache mtime）
- `read_file` 响应含 `mtime_ms`，供编辑工具乐观锁
- catalog、Effective Tools、testexec 映射、Activity 中文标签齐全
- `replace_content` / `save_file` 保持可用，无破坏性变更

### 5.2 性能（量化）

| 指标 | 目标 |
|------|------|
| 单文件 3 处修改 tool 调用数 | ≤2（1 read + 1 diff_edit） |
| 50KB 文件编辑 LLM 输出 | ≪ 50KB（片段级，通常 <2KB） |
| 单次工具执行（I/O） | ≤10ms 量级（1 读 + 内存 patch + 1 写，不含 LLM） |

### 5.3 安全

- 路径校验与现有 `file` ToolSet 一致（禁止 `..`、绝对路径）
- 拒绝二进制与 `.ipynb`
- 单次 patch / search / replace 体积上限（见设计文档常量）

---

## 6. 与相关文档边界

| 文档 | 本文关系 |
|------|----------|
| [23-tools.md](./23-tools.md) | 父需求；catalog 总表由父文档索引，细节在本文 |
| [23-tools.design.md §十三](./23-tools.design.md#十三片段级文件编辑扩展) | 技术方案、Schema、分层、代码锚点 |
| [23-tools.development.md §Phase 4](./23-tools.development.md#phase-4片段级文件编辑p1) | 任务拆分、Phase、实现差距（进度真相） |

---

*文档版本：2.0 — 片段级文件编辑产品需求；实现状态以 [23-tools.development.md](./23-tools.development.md) 为准。*
