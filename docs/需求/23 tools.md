# Tools 管理（Quasar UI + ADK Tool Registry + 前后端契约）

本文档定义 **Tools 管理** 的产品需求、前端页面结构、数据库表设计、调用参数记录与后端接入方案。设计目标是对齐当前 Aranea 系统结构：`agent_runtime_settings` 已包含 `tools_enabled`、`tools_profile`、`tools_allow_json`、`tools_deny_json`、`tools_concurrent_allow_json` 等 Agent 级工具策略；ADK Runner 当前已接入 `PluginConfig`，但 `llmagent.Config` 尚未显式挂载 Tool 列表，因此本期需要补齐 **工具注册表、Agent 可用工具解析、ADK Tool 适配、工具调用审计** 四个环节。

Tool 与 Skill / Plugin 的边界：

| 概念 | 定位 | 管理重点 |
|------|------|----------|
| **Tool** | Agent 可调用的具体能力，例如文件、搜索、MCP、媒体、业务 API | 启用、授权、参数 schema、调用记录、风险控制 |
| **Skill** | Agent 使用某类能力的说明、知识和操作规范 | 上传、验证、冲突、版本、使用追踪 |
| **Plugin** | ADK Runtime 回调中间件，拦截模型、工具、事件链路 | 启用、排序、绑定、运行日志 |

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| Tool 列表 | 是 | 展示内置 Tool、MCP Tool、系统注册 Tool 的启用状态、分类、风险级别、schema、最近调用 |
| Tool 详情 | 是 | 展示工具描述、参数 schema、返回结构、调用示例、权限绑定、最近运行 |
| 启用 / 停用 | 是 | 支持全局启停；Agent 级可通过 allow / deny 覆盖 |
| Agent 绑定 | 是 | 基于现有 `agent_runtime_settings` 的 allow / deny / profile 管理 |
| 参数配置 | 是 | Tool 自身配置与 Agent 覆盖配置分开管理，例如 web_search provider、web_fetch allowlist |
| 调用记录 | 是 | 记录 tool call 参数摘要、参数 hash、结果摘要、耗时、状态、错误 |
| 参数详情 | 是，权限控制 | 管理员可查看脱敏后参数；敏感字段默认不明文落库 |
| 使用统计 | 是 | 调用次数、成功率、平均耗时、最近调用、按 Agent 分布 |
| 动态上传 Tool 代码 | 否 | 不允许前端上传任意 Go / JS 代码作为 Tool |
| ADK Tool 真正执行 | 是 | 后端将启用 Tool 注入 `llmagent.Config.Tools`，并通过 ADK `BeforeTool` / `AfterTool` 记录 |
| MCP Tool 发现 | 后续增强 | 本期先预留 `source=mcp`，可先展示已注册 MCP 工具 |

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 路由 | `/tools` 为 Tools 管理页；`/tools/runs` 为调用记录页 |
| 管理对象 | 内置 Tool + 后端注册 Tool + 可发现 MCP Tool |
| 工具来源 | `builtin` / `mcp` / `system` / `external` |
| 工具执行层级 | ADK `llmagent.Config.Tools` 注入，执行前后由 Plugin / wrapper 记录 |
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
| 启用 / 停用 Tool | 允许；高风险工具需要二次确认 |
| 修改 Tool 全局配置 | 允许；配置必须按后端 schema 校验 |
| 修改 Agent 绑定 | 允许；写入 `agent_runtime_settings` 或 `tool_agent_overrides` |
| 查看调用记录 | 允许；默认展示脱敏摘要 |
| 查看参数详情 | 允许查看脱敏 preview、hash、类型、敏感标记；不展示明文 secret |
| 重放 Tool 调用 | 后续迭代；本期不提供，避免误触发写入 / 外部发送 |

安全控制不依赖“角色”实现，而是由后端按工具风险、字段敏感度和执行环境控制：

- 高风险工具启用和执行前需要显式确认。
- 参数记录默认脱敏，只存 preview 和 hash。
- schema 外字段被拒绝。
- 系统注入字段（如 `agent_id`、`session_id`、`workspace_id`）不允许模型或前端覆盖。

---

## 1. 与 ADK 框架的接入方式

### 1.1 当前状态

当前 `runnerRuntimeBackend.run()` 已创建 ADK Runner：

```go
runner.New(runner.Config{
  AppName:           "aranea",
  Agent:             rootAgent,
  SessionService:    session.InMemoryService(),
  PluginConfig:      runner.PluginConfig{Plugins: plugins},
  AutoCreateSession: true,
})
```

`buildAgent()` 当前只设置 `Name`、`Description`、`Instruction`、`Model`，尚未向 `llmagent.Config` 注入工具。因此 Tools 管理落地需要新增：

1. 后端 Tool Registry：维护 Tool 定义、schema、配置、风险级别。
2. Agent Tool Resolver：根据全局启用、Agent profile、allow / deny 计算本次 Run 的可用工具。
3. **框架 Tool Adapter**：把系统内部 Tool 适配为 **`pkg/trpc-agent-go` / `tool.Tool`**（具体 import 以 `go.mod` 为准）。
4. Tool Audit Recorder：记录调用参数、结果、耗时和错误。

### 1.2 执行链路

```text
用户消息
  -> ChatService / RuntimeAdapter
  -> 读取 Agent + AgentRuntimeSettings
  -> ToolResolver 计算 allowed tools
  -> buildAgent 注入框架 tools
  -> 框架 model 选择 function call
  -> Tool Adapter 执行实际工具
  -> BeforeTool / AfterTool / OnToolError Plugin 记录调用
  -> tool_invocations / tool_invocation_params 落库
  -> 前端 Tools Runs 展示
```

### 1.3 Tool 配置分层

| 层级 | 来源 | 说明 | 优先级 |
|------|------|------|--------|
| 系统默认 | 代码内置 registry | Tool 名称、描述、参数 schema、默认风险级别 | 低 |
| 全局配置 | `tools.config_json` | 管理员在 Tools 页配置，例如默认超时、provider chain | 中 |
| Agent 策略 | `agent_runtime_settings` | `tools_profile`、`tools_allow_json`、`tools_deny_json` | 高 |
| 单次调用上下文 | Runtime context | session、user、workspace、request_id 等系统注入参数 | 最高 |

模型只能控制 `parameters_schema_json` 中声明为可见的字段。`tenant_id`、`agent_id`、`workspace_id`、`session_id`、`request_id` 这类系统字段由后端注入，不进入模型可写参数。

---

## 2. Tool 分类与首批内置工具

### 2.1 分类

| 分类 | 示例 | 风险 |
|------|------|------|
| `filesystem` | `read_file`、`write_file`、`list_files`、`edit_file` | 读低风险，写中高风险 |
| `runtime` | `shell_exec` | 高风险 |
| `web` | `web_search`、`web_fetch` | 中风险 |
| `memory` | `memory_search`、`memory_get` | 低中风险 |
| `skill` | `skill_search`、`use_skill` | 低中风险 |
| `media` | `read_image`、`read_document`、`create_image`、`tts` | 成本与隐私风险 |
| `session` | `session_status`、`sessions_history` | 隐私风险 |
| `messaging` | `send_message`、`send_file` | 高风险 |
| `mcp` | 外部 MCP 工具 | 视工具能力动态判定 |
| `system` | `datetime`、`heartbeat` | 低风险 |

### 2.2 首批建议实现

| Tool Key | 名称 | 说明 | 默认 |
|----------|------|------|------|
| `datetime` | 当前时间 | 返回当前时间、时区、日期格式 | 启用 |
| `web_search` | Web 搜索 | 搜索实时信息，返回标题、URL、摘要 | 启用 |
| `web_fetch` | Web 抓取 | 抓取 URL 并提取文本 / Markdown | 启用 |
| `read_file` | 读取文件 | 读取工作区允许路径内文件 | 启用 |
| `write_file` | 写入文件 | 写入工作区文件 | 启用但中风险 |
| `list_files` | 文件列表 | 列出目录内容 | 启用 |
| `edit_file` | 编辑文件 | 精确替换文件片段 | 启用但中风险 |
| `skill_search` | Skill 搜索 | 搜索可用 Skill | 启用 |
| `use_skill` | 使用 Skill | 标记本次使用某个 Skill，便于追踪 | 启用 |
| `memory_search` | Memory 搜索 | 搜索长期记忆 | 依赖 memory 开关 |
| `memory_get` | Memory 读取 | 读取指定 memory 内容 | 依赖 memory 开关 |
| `read_image` | 图片理解 | 分析图片附件 | 依赖视觉模型 |
| `read_document` | 文档理解 | 分析 PDF / Office / CSV 等 | 依赖文档模型 |
| `create_image` | 图片生成 | 生成图片 | 默认停用或需配置 |
| `tts` | 文本转语音 | 生成语音文件 | 依赖 TTS provider |
| `mcp_tool_search` | MCP 工具搜索 | 在 MCP 工具过多时搜索可用外部工具 | 后续 |

---

## 3. 信息架构与路由

| 路由 | 页面 | 说明 |
|------|------|------|
| `/tools` | Tools 管理 | 列表、启停、分类筛选、配置入口、Agent 绑定入口 |
| `/tools/:id` | Tool 详情 | schema、配置、调用示例、Agent 覆盖、最近调用 |
| `/tools/runs` | Tool 调用记录 | 按 Tool / Agent / Session / 状态 / 时间筛选调用明细 |
| `/agents/:id/tools` | Agent 工具配置 | 嵌入 Agent 设置页，编辑 profile、allow、deny、并发许可 |

侧栏放在管理类导航下，文案为「Tools 管理」和「Tool 调用记录」。若后续将 Agent 工具配置放入 Agent 详情页，高频入口仍应从 Tools 详情页提供「查看使用该工具的 Agent」跳转。

---

## 4. Tools 管理页

### 4.1 页面结构

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

### 4.2 表格列

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

### 4.3 启用 / 停用行为

1. 用户切换 `QToggle`。
2. 前端进入行级 loading，不立即确认最终状态。
3. 调用 `PATCH /tools/:id/enabled`。
4. 成功后更新该行和概览卡片。
5. 失败后恢复原状态并提示错误。

高风险工具启用时必须二次确认：

- 弹窗展示工具名、风险级别、影响范围。
- 若工具属于 `runtime`、`messaging`、`external`，要求输入工具 key 确认。
- 后端仍需做权限校验，不能只依赖前端确认。

### 4.4 详情抽屉

详情抽屉分为 5 个 Tab：

| Tab | 内容 |
|-----|------|
| 概览 | 描述、分类、风险、来源、启用状态、依赖能力 |
| 参数 | `parameters_schema_json` 渲染的只读 schema、必填字段、示例参数 |
| 配置 | `config_schema_json` 渲染的可编辑表单；敏感字段仅显示已配置 |
| Agent | 使用该工具的 Agent、allow / deny 覆盖、profile 命中 |
| 调用 | 最近 20 条调用记录、错误摘要、跳转完整记录 |

### 4.5 Quasar 映射

| 区域 | Quasar 组件 / 说明 |
|------|---------------------|
| 页面骨架 | `QPage` + `QCard` + `q-pa-md` |
| 概览卡片 | `QCard` + `QIcon` + `QBadge` |
| 筛选 | `QInput`、`QSelect`、`QBtnToggle`、`QChip` |
| 表格 | `QTable` + 服务端分页 `@request` |
| 风险提示 | `QBanner` / `QDialog`，高风险启用时二次确认 |
| 详情 | 桌面端右侧 `QDrawer`，窄屏用 maximized `QDialog` |
| Schema 展示 | `QExpansionItem` + 代码块；配置表单按 schema 动态渲染 |
| 调用摘要 | `QTimeline` 或 `QTable`，错误状态用 `negative` |

---

## 5. Agent 工具配置

当前系统已有 `agent_runtime_settings` 工具字段，建议继续复用，避免重复建一套 Agent 工具策略表。

### 5.1 配置项

| 字段 | 来源 | 说明 |
|------|------|------|
| `tools_enabled` | `agent_runtime_settings` | Agent 是否启用工具调用 |
| `tools_profile` | `agent_runtime_settings` | `minimal` / `coding` / `research` / `full` 等预设 |
| `tools_allow_json` | `agent_runtime_settings` | Agent 明确允许的工具 key 或 group |
| `tools_deny_json` | `agent_runtime_settings` | Agent 明确禁止的工具 key 或 group |
| `tools_concurrent_allow_json` | `agent_runtime_settings` | 允许并发执行的工具 key |
| `tools_tool_call_prefix` | `agent_runtime_settings` | 兼容部分模型 tool call 前缀 |

### 5.2 Agent 页 UI

在 Agent 详情页新增「Tools」Tab：

| 区域 | 需求 |
|------|------|
| 总开关 | `tools_enabled` |
| Profile | `QSelect`，展示 profile 说明和包含的工具组 |
| Allow | `QSelect multiple`，选择工具或 `group:filesystem` |
| Deny | `QSelect multiple`，选择工具或工具组 |
| 并发许可 | `QSelect multiple`，只允许选择幂等 / 读类工具 |
| 预览 | 展示最终可用工具列表，按 ToolResolver 的真实结果返回 |

前端不应自行计算最终工具列表，只展示后端 `GET /agents/:id/tools/effective` 返回的结果，避免与后端策略不一致。

---

## 6. 数据模型

### 6.1 Tool

```ts
type Tool = {
  id: string;
  key: string;
  display_name: string;
  description: string;
  category: ToolCategory;
  source: "builtin" | "mcp" | "system" | "external";
  risk_level: "low" | "medium" | "high" | "critical";
  enabled: boolean;
  readonly: boolean;
  requires_confirmation: boolean;
  supports_streaming: boolean;
  supports_concurrency: boolean;
  parameters_schema: Record<string, unknown>;
  result_schema?: Record<string, unknown>;
  config_schema: Record<string, unknown>;
  config_json: Record<string, unknown>;
  default_config_json: Record<string, unknown>;
  metadata_json: Record<string, unknown>;
  invoke_count: number;
  success_count: number;
  failure_count: number;
  avg_duration_ms?: number;
  p95_duration_ms?: number;
  last_invoked_at?: string;
  last_status?: "success" | "error" | "blocked" | "cancelled";
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  permissions: ToolPermissions;
};
```

### 6.2 ToolInvocation

```ts
type ToolInvocation = {
  id: string;
  request_id: string;
  invocation_id: string;
  tool_id: string;
  tool_key: string;
  agent_id: string;
  agent_key: string;
  session_id: string;
  message_id?: string;
  user_id?: string;
  source: "adk" | "manual" | "system";
  status: "success" | "error" | "blocked" | "cancelled";
  started_at: string;
  ended_at?: string;
  duration_ms: number;
  input_preview: string;
  input_hash: string;
  output_preview: string;
  output_hash: string;
  error_code?: string;
  error_message?: string;
  redaction_applied: boolean;
  metadata_json: Record<string, unknown>;
};
```

### 6.3 ToolInvocationParam

```ts
type ToolInvocationParam = {
  id: string;
  invocation_id: string;
  tool_key: string;
  param_name: string;
  param_type: "string" | "number" | "boolean" | "array" | "object" | "null";
  value_preview: string;
  value_hash: string;
  value_size_bytes: number;
  is_required: boolean;
  is_sensitive: boolean;
  redaction_reason?: string;
  created_at: string;
};
```

参数记录原则：

- `value_preview` 最多 500 字符，超过截断。
- `api_key`、`token`、`password`、`secret`、`authorization`、`cookie` 等字段默认标记敏感。
- 对象和数组只记录结构摘要，例如 `{"query":"...","limit":10}`，大对象不完整落库。
- `value_hash` 用于排查“是否同一参数重复调用”，不用于还原原文。
- 完整参数仅保存在内存执行上下文，不写入普通业务库。

---

## 7. 数据库表设计

以下以当前 SQLite 风格设计：主键 `TEXT`，布尔用 `INTEGER`，JSON 用 `TEXT`，时间用 ISO 字符串。

### 7.1 `tools`

```sql
CREATE TABLE IF NOT EXISTS tools (
  id TEXT PRIMARY KEY,
  tool_key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'system',
  source TEXT NOT NULL DEFAULT 'builtin',
  risk_level TEXT NOT NULL DEFAULT 'low',
  enabled INTEGER NOT NULL DEFAULT 1,
  readonly INTEGER NOT NULL DEFAULT 0,
  requires_confirmation INTEGER NOT NULL DEFAULT 0,
  supports_streaming INTEGER NOT NULL DEFAULT 0,
  supports_concurrency INTEGER NOT NULL DEFAULT 0,
  parameters_schema_json TEXT NOT NULL DEFAULT '{}',
  result_schema_json TEXT NOT NULL DEFAULT '{}',
  config_schema_json TEXT NOT NULL DEFAULT '{}',
  config_json TEXT NOT NULL DEFAULT '{}',
  default_config_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category);
CREATE INDEX IF NOT EXISTS idx_tools_source ON tools(source);
CREATE INDEX IF NOT EXISTS idx_tools_enabled ON tools(enabled);
CREATE INDEX IF NOT EXISTS idx_tools_risk_level ON tools(risk_level);
```

### 7.2 `tool_agent_overrides`

用于存储 Tool 粒度的 Agent 覆盖配置。全局 allow / deny 仍保留在 `agent_runtime_settings`，此表只处理单个 Tool 的 config override 或确认策略。

```sql
CREATE TABLE IF NOT EXISTS tool_agent_overrides (
  id TEXT PRIMARY KEY,
  tool_id TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  mode TEXT NOT NULL DEFAULT 'inherit',
  config_override_json TEXT NOT NULL DEFAULT '{}',
  requires_confirmation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(tool_key, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_agent ON tool_agent_overrides(agent_id);
CREATE INDEX IF NOT EXISTS idx_tool_agent_overrides_tool ON tool_agent_overrides(tool_key);
```

`mode` 枚举：

| 值 | 说明 |
|----|------|
| `inherit` | 继承全局工具启用和 Agent profile |
| `allow` | 对该 Agent 明确允许 |
| `deny` | 对该 Agent 明确禁止 |
| `config_only` | 只覆盖配置，不改变允许关系 |

### 7.3 `tool_invocations`

```sql
CREATE TABLE IF NOT EXISTS tool_invocations (
  id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL DEFAULT '',
  invocation_id TEXT NOT NULL DEFAULT '',
  tool_id TEXT NOT NULL DEFAULT '',
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  agent_key TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'adk',
  status TEXT NOT NULL DEFAULT 'success',
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  input_preview TEXT NOT NULL DEFAULT '',
  input_hash TEXT NOT NULL DEFAULT '',
  output_preview TEXT NOT NULL DEFAULT '',
  output_hash TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  redaction_applied INTEGER NOT NULL DEFAULT 1,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_tool_time ON tool_invocations(tool_key, started_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_agent_time ON tool_invocations(agent_id, started_at);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_session ON tool_invocations(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_status ON tool_invocations(status);
```

### 7.4 `tool_invocation_params`

```sql
CREATE TABLE IF NOT EXISTS tool_invocation_params (
  id TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  param_name TEXT NOT NULL,
  param_type TEXT NOT NULL DEFAULT 'string',
  value_preview TEXT NOT NULL DEFAULT '',
  value_hash TEXT NOT NULL DEFAULT '',
  value_size_bytes INTEGER NOT NULL DEFAULT 0,
  is_required INTEGER NOT NULL DEFAULT 0,
  is_sensitive INTEGER NOT NULL DEFAULT 0,
  redaction_reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_invocation ON tool_invocation_params(invocation_id);
CREATE INDEX IF NOT EXISTS idx_tool_invocation_params_tool_param ON tool_invocation_params(tool_key, param_name);
```

### 7.5 `tool_usage_daily`

用于列表页和趋势图快速聚合，避免每次扫调用明细。

```sql
CREATE TABLE IF NOT EXISTS tool_usage_daily (
  id TEXT PRIMARY KEY,
  date_key TEXT NOT NULL,
  tool_key TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  call_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failure_count INTEGER NOT NULL DEFAULT 0,
  blocked_count INTEGER NOT NULL DEFAULT 0,
  total_duration_ms INTEGER NOT NULL DEFAULT 0,
  avg_duration_ms REAL NOT NULL DEFAULT 0,
  p95_duration_ms REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(date_key, tool_key, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_tool_date ON tool_usage_daily(tool_key, date_key);
CREATE INDEX IF NOT EXISTS idx_tool_usage_daily_agent_date ON tool_usage_daily(agent_id, date_key);
```

---

## 8. API 契约

### 8.1 Tool 列表

`GET /api/v1/tools`

Query：

| 参数 | 类型 | 说明 |
|------|------|------|
| `search` | string | 名称、key、描述 |
| `category` | string | 分类 |
| `source` | string | `builtin` / `mcp` / `system` / `external` |
| `risk_level` | string | 风险级别 |
| `enabled` | boolean | 启用状态 |
| `status` | string | `healthy` / `error` / `unused` 可选 |
| `page` | number | 页码，从 1 开始 |
| `page_size` | number | 每页数量 |
| `sort` | string | `last_invoked_at` / `invoke_count` / `failure_rate` |

Response：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 0,
  "summary": {
    "total_tools": 16,
    "enabled_tools": 12,
    "high_risk_enabled": 2,
    "calls_24h": 128,
    "failure_rate_24h": 0.031
  }
}
```

### 8.2 Tool 详情

`GET /api/v1/tools/:id`

返回完整 `Tool`，包含 schema、配置、统计和权限。

### 8.3 启用 / 停用

`PATCH /api/v1/tools/:id/enabled`

Request：

```json
{
  "enabled": true,
  "confirm_key": "shell_exec"
}
```

Response：返回更新后的 `Tool`。

### 8.4 更新配置

`PUT /api/v1/tools/:id/config`

Request：

```json
{
  "config_json": {
    "timeout_ms": 30000,
    "max_result_chars": 12000,
    "redact_sensitive": true
  }
}
```

后端必须使用 `config_schema_json` 校验类型、枚举、范围和敏感字段，不允许前端提交 schema 外字段。

### 8.5 Agent 工具有效列表

`GET /api/v1/agents/:id/tools/effective`

Response：

```json
{
  "tools_enabled": true,
  "profile": "coding",
  "allow": ["group:filesystem", "web_search"],
  "deny": ["shell_exec"],
  "items": [
    {
      "tool_key": "read_file",
      "display_name": "读取文件",
      "category": "filesystem",
      "source": "builtin",
      "enabled": true,
      "effective_state": "allowed",
      "reason": "profile:coding"
    }
  ]
}
```

### 8.6 更新 Agent 工具策略

`PUT /api/v1/agents/:id/tools/policy`

Request：

```json
{
  "tools_enabled": true,
  "tools_profile": "coding",
  "tools_allow": ["group:filesystem", "web_search", "web_fetch"],
  "tools_deny": ["shell_exec"],
  "tools_concurrent_allow": ["web_search", "web_fetch", "read_file"]
}
```

后端写入 `agent_runtime_settings` 对应字段，数组字段序列化为 JSON。

### 8.7 Tool Agent 覆盖

`GET /api/v1/tools/:id/agent-overrides`

`PUT /api/v1/tools/:id/agent-overrides`

Request：

```json
{
  "items": [
    {
      "agent_id": "agent_xxx",
      "mode": "config_only",
      "enabled": true,
      "requires_confirmation": false,
      "config_override_json": {
        "max_result_chars": 8000
      }
    }
  ]
}
```

### 8.8 调用记录

`GET /api/v1/tools/runs`

Query：

| 参数 | 类型 | 说明 |
|------|------|------|
| `tool_key` | string | Tool key |
| `agent_id` | string | Agent |
| `session_id` | string | 会话 |
| `status` | string | `success` / `error` / `blocked` / `cancelled` |
| `from` / `to` | string | ISO 时间 |
| `has_error` | boolean | 仅看错误 |
| `page` | number | 页码 |
| `page_size` | number | 每页数量 |

Response：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 0
}
```

### 8.9 调用参数详情

`GET /api/v1/tools/runs/:id/params`

返回脱敏后的 `ToolInvocationParam[]`。后端根据权限决定是否返回 `value_preview`，只读用户只能看到字段名、类型、是否敏感和 hash。

---

## 9. 后端功能设计

### 9.1 新增模块

| 模块 | 建议文件 | 职责 |
|------|----------|------|
| Domain | `internal/domain/models.go` | 新增 `Tool`、`ToolInvocation`、`ToolInvocationParam` |
| Repository | `internal/repository/sqlite.go` | CRUD、分页、统计、调用记录落库 |
| Service | `internal/service/tool_service.go` | 业务校验、schema 校验、Agent 策略解析 |
| Runtime | `internal/runtime/adk_builtin_tools.go` | 内置工具定义与 ADK adapter |
| Runtime | `internal/runtime/adk_tool_recorder.go` | 调用前后记录、参数脱敏 |
| Transport | `internal/transport/tool.go` | `/api/v1/tools` REST API |

### 9.2 Tool Registry

后端启动时执行 `SeedBuiltinTools()`：

1. 从代码 registry 获取内置工具定义。
2. `INSERT ... ON CONFLICT(tool_key) DO UPDATE`，保留用户修改的 `enabled` 和 `config_json`。
3. 更新描述、schema、分类、风险级别等代码权威字段。
4. 对已删除但代码仍存在的内置工具恢复 `deleted_at=''`。

内置定义建议结构：

```go
type BuiltinToolDefinition struct {
  Key              string
  DisplayName      string
  Description      string
  Category         string
  Source           string
  RiskLevel        string
  Readonly         bool
  RequiresConfirm  bool
  ParametersSchema map[string]any
  ResultSchema     map[string]any
  ConfigSchema     map[string]any
  DefaultConfig    map[string]any
  Factory          func(deps ToolDeps) (tool.Tool, error)
}
```

### 9.3 ToolResolver

`ToolResolver.Resolve(ctx, agent)` 输出本次 ADK Run 可用工具：

1. 如果 `tools_enabled=false`，返回空列表。
2. 查询全局启用的 tools。
3. 根据 `tools_profile` 展开工具组。
4. 应用 `tools_allow_json` 增加或限制工具。
5. 应用 `tools_deny_json` 移除工具。
6. 应用 `tool_agent_overrides` 的 `allow` / `deny` / `config_only`。
7. 过滤依赖不满足的工具，例如无 TTS provider 时不注入 `tts`。
8. 构造 ADK tool 列表并返回 debug reason，用于前端预览。

### 9.4 ADK Tool Adapter

每个系统 Tool 适配 ADK `tool.Tool`：

```go
type ManagedADKTool struct {
  Definition ToolDefinition
  Executor   ToolExecutor
  Recorder   ToolRecorder
}
```

执行流程：

1. 接收 ADK 传入参数。
2. 使用 `parameters_schema_json` 校验参数。
3. 注入系统上下文参数。
4. 写入 `tool_invocations` started 状态。
5. 执行实际工具。
6. 写入参数摘要、结果摘要、耗时、状态。
7. 返回 ADK 需要的 map / content。

### 9.5 调用记录与 Plugin 的关系

推荐两层记录：

| 层 | 作用 |
|----|------|
| Tool Adapter Recorder | 权威记录每次工具执行，覆盖所有 Tool |
| ADK Plugin `runtime_audit` | 记录运行链路事件，辅助排查 Tool 前后上下文 |

不要只依赖 Plugin 记录 Tool 调用，因为部分工具可能在 adapter 内部失败，未完整进入 after callback。Adapter recorder 能保证 started / ended 成对或可恢复。

### 9.6 参数脱敏

后端提供统一函数：

```go
func RedactToolArgs(toolKey string, schema map[string]any, args map[string]any) RedactedArgs
```

敏感判定来源：

1. schema 中字段 `x-sensitive: true`。
2. 字段名命中敏感关键字：`key`、`token`、`secret`、`password`、`authorization`、`cookie`。
3. 值命中密钥正则，例如 Bearer token、AK/SK、私钥片段。
4. Tool 风险级别为 high / critical 时默认缩短 preview。

---

## 10. Tool 调用记录页

### 10.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Tool 调用记录」 |
| 右上 | 刷新、导出 CSV（后续）、清理策略入口（后续） |
| 筛选 | Tool、Agent、Session、状态、时间范围、仅看错误 |
| 主表 | 服务端分页 |
| 详情弹窗 | 参数、输出、错误、metadata |

### 10.2 表格列

| 列 | 字段 | 说明 |
|----|------|------|
| 时间 | `started_at` | 本地化显示 |
| Tool | `tool_key`、`display_name` | 标签 + 名称 |
| Agent | `agent_display_name` / `agent_key` | 可点击跳转 |
| Session | `session_id` | 可复制，后续跳转会话 |
| 状态 | `status` | 成功、错误、阻断、取消 |
| 耗时 | `duration_ms` | 超过 P95 标记 |
| 参数摘要 | `input_preview` | 单行截断 |
| 结果摘要 | `output_preview` | 单行截断 |
| 错误 | `error_message` | 错误时展示 |
| 操作 | - | 查看详情 |

### 10.3 详情弹窗

| Tab | 内容 |
|-----|------|
| 参数 | 参数名、类型、preview、hash、是否敏感 |
| 输出 | `output_preview`、`output_hash` |
| 错误 | `error_code`、`error_message`、metadata |
| 上下文 | request_id、invocation_id、session_id、message_id、agent_id |

详情弹窗必须明确提示：「参数已脱敏，hash 仅用于排查重复调用，不能还原原文。」

---

## 11. 前端状态与交互细节

### 11.1 状态展示

| 状态 | UI |
|------|----|
| 首次加载 | `QInnerLoading` 或 `QSkeleton` |
| 空列表 | `QBanner` +「同步内置工具」 |
| 搜索无结果 | 文案「没有匹配的 Tool」+「重置筛选」 |
| 请求失败 | `QBanner` negative +「重试」 |
| 无权限 | 按钮禁用，`QTooltip` 展示原因 |
| 高风险开启 | `QDialog` 二次确认 |

### 11.2 可访问性

- 所有 icon-only 按钮必须有 `aria-label`。
- `QToggle` 需要明确 label，不只依赖颜色表达启用状态。
- 风险级别除颜色外，还必须显示文字。
- 详情抽屉打开后焦点进入标题，关闭后回到触发按钮。
- 调用记录错误信息使用可复制文本，不只放 tooltip。

---

## 12. 与现有文档 / 模块的关系

| 模块 | 关系 |
|------|------|
| `20 skill.md` | `skill_search` / `use_skill` 的调用记录可关联 Skill 运行记录 |
| `22 plugin.md` | Plugin 负责运行链路回调；Tool adapter 负责权威调用记录 |
| `18 monitor.md` | Tool 调用可同步发实时事件，Monitor 页面展示 `tool.call` / `tool.result` |
| Agent 设置 | 复用 `agent_runtime_settings` 的工具策略字段 |
| ADK Runner | 在 `buildAgent()` 阶段注入 resolved tools |

---

## 13. 实施拆分建议

### Phase 1：数据和列表

1. 新增 `tools`、`tool_invocations`、`tool_invocation_params`、`tool_usage_daily` 表。
2. 后端 seed 内置 Tool 定义。
3. 实现 `/api/v1/tools` 列表、详情、启停、配置接口。
4. 前端完成 `/tools` 列表和详情抽屉。

### Phase 2：Agent 策略

1. 实现 ToolResolver。
2. 实现 `/api/v1/agents/:id/tools/effective`。
3. Agent 详情页新增 Tools Tab。
4. 支持 profile / allow / deny 保存。

### Phase 3：ADK 执行

1. 新增 ADK Tool Adapter。
2. `buildAgent()` 注入 resolved tools。
3. 实现 Tool Adapter Recorder。
4. 完成 `/tools/runs` 调用记录页。

### Phase 4：治理增强

1. 高风险工具二次确认。
2. 参数敏感字段策略可配置。
3. MCP Tool 接入与搜索。
4. 调用统计日聚合与清理策略。

---

## 14. 运行时实现与演进方向

> 本节整合自 `architecture/agent-skills-tools-mcp-memory.md`、`architecture/trpc-agent-go-implementation-plan.md` 与 `architecture/agent-repo-retrieval-context-engineering.md`，描述 Tools 在 Agent 运行时的装配机制、代码检索增强与后续演进方向。

### 14.1 运行时工具装配

| 步骤 | 位置 | 说明 |
|------|------|------|
| 真相源 | `AgentUsecase.GetEffectiveTools` | 返回 profile、是否启用工具、每条 `tool_key` 的 allow/deny 列表 |
| 映射为框架 Tool | `internal/tools/tools.go` | `ToolsFromAgentEffective` → `registry.ApplyEffectiveAliases` → `registry.ADKToolsFromEnabled` |
| 单 Agent / Team 成员 | `ADKToolsForAgentPolicy` | 在生效工具基础上，若 `SubagentsEnabled` 则追加 `spawn_subagent` |

**具体工具族**：

- **工作区文件**：`read_file`、`list_files`、`write_file`、`edit_file`（沙箱根由 `ARANEA_WORKSPACE_ROOT` 约束）
- **框架内置顺序**：`exit_loop`、`web_search`、`web_fetch`、`load_artifacts`、`load_memory`、`preload_memory`
- **宿主**：`shell_exec`
- **子 Agent**：`spawn_subagent`（策略开启时）

`BuildLLMAgent`（`internal/agent/adk_build.go`）将 `deps.Tools` 与 `deps.Toolsets` 分别传给 `llmagent.Config`，由框架统一暴露给模型。

### 14.2 代码库检索与上下文工程

> 本节详细内容整合自 `architecture/agent-repo-retrieval-context-engineering.md`。

当前工作区 filesystem 工具仅有 `read_file` / `list_files` / `write_file` / `edit_file`，**缺少受预算约束的字面/workspace 级搜索工具**；模型只能反复列目录或直接读大文件摸索。

**核心指标**：

| 指标 | 含义 | 建议度量方式 |
|------|------|----------------|
| Precision@k（工具） | 前 k 次工具调用中，对用户最终任务有直接帮助的比例 | 固定任务脚本 + 人工或 LLM-as-judge 标注 |
| 工具返回 token 体积 | 单次 list/search 的平均 JSON 体积 | 日志里对工具 result 长度采样 |
| 到达结论步数 | 从首轮 user 到「可编译/通过约定测试」的 tool+assistant 轮次 | CI 评测集 |

**混合检索层级**：

| 层级 | 能力 | 落点 |
|------|------|------|
| L0 字面检索 | 子串或安全子集正则、glob、按文件名 | 缺口：P0-WS 工单 |
| L1 Git/变更锚点 | `git status` / `diff`（可选） | 可先通过受控 `shell_exec` 或专用只读工具包装 |
| L2 符号/LSP | 跳转到定义、引用 | 中长期：P1-SYM 工单 |
| L3 语义/向量 | hybrid：向量候选 + 字面重排 | 长期：P2-RAG 工单 |

**设计约束**：

1. **沙箱**：只允许访问 `workspace.ResolvePath` 可解析的路径；禁止把绝对路径或可穿越 `..` 的原始字符串直接交给操作系统 API
2. **输出预算**：任何「扫描类」工具必须有 `max_results`（默认 40～80）、单行/单文件 excerpt 上限、总输出字节上限
3. **DoS**：正则搜索须限复杂度（可选用 RE2、`regexp` 编译失败即拒；或只允许 `Substring`/`FixedString` 首版）
4. **忽略噪音**：可选实现「尊重 `.gitignore`」（若嵌入调用 `rg` 则开箱即有）
5. **双通路一致**：若维护 OpenAI-native 回路，新增工具须同时更新 `WorkspaceOpenAISpecs`/`Invoke*` 中与 ADK 等价的契约
6. **Catalog 一致性**：新增 `tool_key` 必须与 `biz`/`data`/`web` 对齐

**分阶段实施工单**：

#### P0-WS：`workspace_search` 字面检索工具

**目标**：给模型一个不依赖 shell 的、默认 `max_results` 截断的工作区搜索能力（子串或可配置 regex + 可选路径 glob）。

**参数**：`query`（必填）；`mode`：`substring`|`regex`；`path_prefix`（可选）；`glob`（可选文件名 glob）；`max_results`（可选 int，默认 `50`，硬顶 `500`）；`max_matches_per_file`（可选）；`context_lines`（可选，每条匹配前后行数）。

**返回**：`matches`: `[{ "path","line","column","snippet" }]`，若超过预算在顶层加 `truncated: true`。

**后端选型**：
- **首推**：在只读、`PATH` 上存在 `rg` 且沙箱 cwd 定于 `workspace.Root()` 的前提下，封装一次 `rg` 调用：`--follow`、`-S`（smart case）、`-n`/`--column`、`--glob`、`--max-count`、`--max-filesize`、`--json`、硬性 wall-clock 超时（如 10～30s）
- **必选回退**：无 `rg` 或调用失败 → `filepath.WalkDir` + 默认跳过清单 + UTF-8 文本启发式；大仓必须通过 `path_prefix` 收窄
- **不推荐**：默认把字面检索唯一路径绑在 `shell_exec` 上调 `rg`

**接入 registry**：
- `internal/tools/registry/keys.go` 新增常量 `WorkspaceSearch = "workspace_search"`
- `registry/adk_enabled.go`：在 `WorkspaceADKTools` 之后追加该工具
- `builtin_tools_seed.go` 增加一行（`category: filesystem` 或 `filesystem_search`）
- `biz/agent_effective_tools.go`：`toolProfiles` 中至少 `coding`、`research`、`full` 应包含该 key

**验收**：`go test ./internal/tools/workspace_search/...`（含边界：越权路径、`max_results` 截断）；启用该工具的 Agent 在「找字符串 X」任务中，`list_files` 调用次数 ≤ 人工基线。

#### P0-LF：收紧 `list_files` 输出预算

新增可选参数 `max_entries`（默认 `0`=不截断，或默认 `200`）。截断时在结果中加 `truncated: true`。可选 `depth`/`sort`/`dirs_only` 按复杂度迭代。

#### P0-RF：`read_file` 部分读取

增加可选 `offset` + `limit` 或「按行区间」`(start_line,end_line)`，避免全文读入。与 P0-WS 的 snippet 衔接。

#### P0-PROMPT：系统提示与 Runtime cue

在 `RuntimeCapabilityCue` 追加短规则：有 `workspace_search` 时探索顺序 `workspace_search → read_file → edit/write`；无明确关键词才 `list_files` 且每层只列一次；禁止为用工具而用工具。

#### P1-SYM：Go 符号级导航

候选实现：包装 `go doc`/`go list`/`gopls query`（仅只读）；或预生成 `.json` outline。接入为独立 `tool_key`（如 `go_outline`），或作为 `workspace_search` 的特殊 `mode`。

#### P1-BGIDX：工作区后台轻量索引/摘要

文件级 manifest（mtime + hash + lang + LOC）存放在进程内 LRU 或 SQLite/Bolt；Watcher 可选用 fsnotify，或会话首次 `workspace_search` 时 lazy 补齐。

#### P2-RAG：项目向量检索 + hybrid

必须与 `docs/需求/15 memory-L3-semantic.md` 对齐，避免再造孤岛向量库。chunk 边界用目录 + exported API + 文件名；检索流程向量召回 topK → same-file 字面重排 → excerpt。

#### P3-TEAM：Team 运行时黑板

并行分支写入 `visibility=shared` 的 `working_memory` 字段，记录已搜索路径与已知模块，减少重复工作。

**安全与守门**：

- 新检索工具必须为 readonly；与 `shell_exec` 严格分离
- 正则与时间上限防 ReDoS
- 不在工具结果内返回二进制全文
- WalkDir/`rg` 共用默认跳过清单：`.git`、`node_modules`、`vendor`、`dist`、`build`、`.cursor` 等；按后缀：`.exe`、`.png`、`.zip` 等

**代码地图**（改哪里）：

| 主题 | 路径 | 说明 |
|------|------|------|
| 工作区根与路径校验 | `internal/tools/workspace/sandbox.go` | 一切文件访问须落在此 sandbox 内 |
| 工作区四类文件工具 | `internal/tools/registry/workspace.go` | `WorkspaceToolNames` 顺序即挂载顺序 |
| Effective tools → ADK Tool | `internal/tools/tools.go` | 新 builtins 须在 platform catalog + biz policy + registry 挂载三处贯通 |
| 运行时能力提示 | `internal/agent/prompt.go` `RuntimeCapabilityCue` | 追加探索策略 |
| 平台工具种子 | `internal/data/builtin_tools_seed.go` | 新工具要增 `builtinPlatformToolSeeds` 行 |
| Profile 预设 | `internal/biz/agent_effective_tools.go` | `read_only`/`coding` 等是否默认带上新检索工具 |

### 14.3 演进方向

| 方向 | 现状与问题 | 建议 |
|------|------------|------|
| 服务层「唯一桥点」一致性 | `runSingleAgentViaADK` 与 `team.Runner` 各自构造 Runner、BuilderDeps，逻辑相似但分叉维护 | 抽出面向本产品的 Runner 装配助手（位于 `internal/service`），禁止下沉到 `internal/server` |
| Tool vs Toolset 职责 | 平台 `mcp_tool_set` 未进入 `ADKToolsFromEnabled`，模型侧「可调能力」与运营配置脱节 | 保持 builtin/tool 映射在 `internal/tools/registry`；MCP 只走 Toolset，由 effective 策略在 service 组装进 `deps.Toolsets` |
| 回归与熔断 | 对「生效工具 + skill 子集 + MCP」缺少契约测试 | 增加录制/契约测试；对 MCP 与 runtime 报错路径统一用户可见文案 |
| 记忆工具的业务含义 | `load_memory` / `preload_memory` 走框架默认语义，若底层仍是空/in-memory，能力名实不符 | 默认关闭直至后端就绪并在 `RuntimeCapabilityCue` 中如实描述，或 Composite SearchMemory 聚合多后端 |

---

*文档版本：2.1 — 整合运行时装配机制、代码检索增强与演进方向（原 architecture/agent-skills-tools-mcp-memory.md、agent-repo-retrieval-context-engineering.md Tools 部分）。*

---

## 15. trpc-agent-go 对齐需求（M7 Tool 工具体系）

> 本节补充 `plan.md` M7 模块的对齐需求，确保 Tool 工具体系完全复刻 trpc-agent-go `tool` 包能力。

### 15.1 trpc Tool 接口迁移

**trpc 框架**：`tool.Tool` / `tool.CallableTool` / `tool.StreamableTool` 统一工具接口。

**需求**：
- 所有内置工具通过 trpc `tool.Tool` 接口注册
- `tool.FunctionTool` 包装简单函数为 Tool
- `tool.StreamableTool` 支持流式结果返回
- `tool.ToolSet` 管理工具集合

**涉及文件**：`internal/tools/trpc/toolsets.go`

**验收标准**：所有内置工具通过 trpc Tool 接口注册和调用

### 15.2 流式工具

**trpc 框架**：`tool.StreamableTool` 支持流式返回工具结果。

**需求**：
- 长时间运行的工具（如 `web_search`、`shell_exec`）支持流式返回
- 前端可实时显示工具执行进度
- 流式结果通过 SSE 推送

**涉及文件**：`internal/tools/trpc/toolsets.go`、`internal/server/sse.go`

**验收标准**：长时间运行的工具可流式返回结果

### 15.3 工具重试

**trpc 框架**：`tool.WithRetry` 支持工具执行重试。

**需求**：
- 工具配置增加 `max_retries` 和 `retry_delay_ms` 字段
- 执行失败时自动重试
- 重试次数和结果记录在 `tool_invocations` 中

**涉及文件**：`internal/tools/trpc/toolsets.go`

**验收标准**：工具执行失败时自动重试

### 15.4 工具过滤

**trpc 框架**：`tool.WithFilter` 支持工具调用过滤。

**需求**：
- 根据 Agent 的 `tools_allow_json` / `tools_deny_json` 过滤可用工具
- 高风险工具需要确认后执行
- 过滤结果记录在 `tool_invocations` 中

**涉及文件**：`internal/tools/trpc/toolsets.go`、`internal/agent/trpc_build.go`

**验收标准**：工具按 Agent 配置过滤

### 15.5 ToolSet 并行

**trpc 框架**：`tool.ToolSet` 支持并行执行多个工具调用。

**需求**：
- LLM 返回多个 tool_call 时并行执行
- 并行度受 `tools_concurrent_allow_json` 控制
- 并行结果合并后返回

**涉及文件**：`internal/tools/trpc/toolsets.go`

**验收标准**：多个工具调用可并行执行
