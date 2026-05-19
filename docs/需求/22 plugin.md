# Plugin 管理 — 产品需求文档

> **版本**：2026-05-19 | **状态**：CRUD + Runtime 已通，内置插件待完善
> **设计**：[22 plugin.design.md](./22%20plugin.design.md) · **开发计划**：[22-plugin-development.md](./22-plugin-development.md)

---

本文档定义 **Plugin 管理** 的产品需求、前端页面结构与后端契约。Plugin 指 ADK 运行时的回调扩展机制，用于在 Agent 执行过程中插入治理、调试、增强、风控等逻辑。

Plugin 与 Skill / Tool 的边界：

- **Skill**：面向 Agent 的能力、知识、脚本和使用规范。
- **Tool**：Agent 可调用的具体外部能力。
- **Plugin**：运行时拦截器 / 中间件，改变或增强 Agent 执行链路。

当前系统优先管理 **内置 Plugin 的启用、配置、排序和绑定关系**。不支持普通用户上传任意 Go 插件代码。

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| Plugin 列表 | 是 | 展示系统内置插件、启用状态、作用阶段、执行顺序、最近运行状态 |
| Plugin 配置 | 是 | 支持编辑内置插件暴露的安全配置项 |
| 启用 / 停用 | 是 | 支持全局启用、停用 |
| 执行顺序 | 是 | 插件按顺序执行，支持调整排序 |
| Agent 绑定 | 是 | 支持插件全局生效或只绑定到指定 Agent |
| 运行统计 | 是 | 展示调用次数、拦截次数、错误次数、最近执行时间、平均耗时 |
| 运行日志 | MVP 展示摘要 | 仅展示 callback 类型、Agent、结果、耗时、错误摘要 |
| 上传第三方插件代码 | 否 | 不允许上传任意 Go / 动态库插件，避免安全和跨平台问题 |
| 动态插件运行时 | 后续迭代 | 可考虑外部进程 / gRPC / MCP / WASM 插件 |
| 真正注入框架 Runner | 已接入 | Plugin 列表已注入 Agent Runner |

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 路由 | `/plugins` 为 Plugin 管理页；`/plugins/runs` 为 Plugin 运行记录 |
| 管理对象 | 仅管理系统内置插件 |
| 插件执行层级 | 执行层运行时调用，不是编译层自动注入 |
| 插件加载方式 | 编译期内置注册，运行时按数据库配置启用 |
| 插件顺序 | 数字越小越先执行；同顺序按创建时间稳定排序 |
| 默认状态 | 高风险插件默认停用；观测类插件可默认开启开发环境、关闭生产环境 |
| 配置格式 | 后端维护 schema，前端按 schema 渲染表单 |
| 权限 | 只有管理员可启停、排序、修改配置；编辑者可查看；只读用户仅查看摘要 |
| 安全限制 | 禁止前端传入任意代码、任意 callback 函数或动态库路径 |

### 0.3 角色与权限

| 能力 | 管理员 | 编辑者 | 只读用户 |
|------|--------|--------|----------|
| 查看 Plugin 列表 | 是 | 是 | 是 |
| 查看 Plugin 配置 | 是 | 是 | 否，仅摘要 |
| 启用 / 停用 Plugin | 是 | 否 | 否 |
| 修改 Plugin 配置 | 是 | 否 | 否 |
| 调整执行顺序 | 是 | 否 | 否 |
| 绑定 / 解绑 Agent | 是 | 否 | 否 |
| 查看运行日志 | 是 | 是，脱敏摘要 | 否 |
| 查看敏感参数 | 是，需后端授权 | 否 | 否 |

前端根据后端返回的 `permissions` 控制按钮可见性和禁用态，不在本地硬编码角色名。

---

## 1. Plugin 执行原理

### 1.1 基本机制

Plugin 是运行时回调对象。每个 Plugin 实现 `plugin.Plugin` 接口，通过 `Register(r *Registry)` 注册回调到 `plugin.Registry`。

**trpc-agent-go 框架支持的 Callback Points**（7 个）：

| Callback Point | 注册方法 | 触发时机 | 典型用途 |
|----------------|----------|----------|----------|
| `BeforeAgent` | `r.BeforeAgent(cb)` | Agent 执行前 | Agent 权限检查、上下文注入、提前拒绝 |
| `AfterAgent` | `r.AfterAgent(cb)` | Agent 执行后 | 统计、审计、清理 |
| `BeforeModel` | `r.BeforeModel(cb)` | 模型请求前 | 模型路由、脱敏、成本控制、改写请求 |
| `AfterModel` | `r.AfterModel(cb)` | 模型响应后 | 输出审查、记录 token、响应改写 |
| `BeforeTool` | `r.BeforeTool(cb)` | 工具执行前 | 权限检查、参数补充、确认机制、参数改写 |
| `AfterTool` | `r.AfterTool(cb)` | 工具执行后 | 结果审计、统计、结果改写 |
| `OnEvent` | `r.OnEvent(hook)` | 事件经过 Runner 时 | 事件改写、日志、监控 |

> **注意**：框架不提供 `OnUserMessage`、`BeforeRun`、`AfterRun`、`OnModelError`、`OnToolError` 等独立回调点。如需处理模型错误或工具错误，应在 `AfterModel` / `AfterTool` 中检查 `args.Error != nil` 来实现。

### 1.2 回调签名

每个回调点使用 Structured 签名，统一为 `func(ctx, args) (*Result, error)` 模式：

| 回调点 | Args 关键字段 | Result 关键字段 | 拦截方式 |
|--------|--------------|----------------|----------|
| `BeforeAgent` | Agent 信息、Session 信息 | `CustomResponse` 跳过执行；`Context` 传递上下文 | 返回 CustomResponse |
| `AfterAgent` | Agent 响应、执行错误 | `CustomResponse` 替换响应 | 返回 CustomResponse |
| `BeforeModel` | 模型请求内容、参数 | `CustomResponse` 跳过模型调用；`Request` 可修改 | 返回 CustomResponse 或修改 Request |
| `AfterModel` | 模型响应内容、错误 | `CustomResponse` 替换响应 | 返回 CustomResponse |
| `BeforeTool` | 工具名称、调用参数 | `CustomResult` 跳过工具执行；`ModifiedArguments` 改写参数 | 返回 CustomResult 或修改参数 |
| `AfterTool` | 工具执行结果、错误 | `CustomResult` 替换结果；`SkipSummarization` | 返回 CustomResult |
| `OnEvent` | 事件对象、调用上下文 | 改写后的事件 | 返回改写后的事件 |

> **具体 Go 类型签名**参见设计文档 §7.5。

### 1.3 调用顺序

运行时创建会话时注入已启用的插件列表。执行期间由框架在固定节点主动调用。

执行规则：

1. 插件按管理页配置的执行顺序依次执行。
2. `BeforeAgent` / `BeforeModel` / `BeforeTool` 回调返回拦截结果时，默认跳过后续同类回调（可通过配置改变）。
3. 回调返回错误时，默认中断当前执行链路（可通过配置改变）。
4. 插件关闭时按逆序释放资源。

> **具体框架 API 和配置项**参见设计文档 §7.4 和 §12。

### 1.4 第三方扩展边界

本期不支持用户上传任意 Go Plugin。原因：

- Go Plugin 动态库 `.so` 跨平台能力有限，Windows 支持差。
- 动态加载代码存在高安全风险。
- 插件可拦截模型、工具、用户输入，权限很高。
- 插件顺序和错误处理会直接影响 Agent 运行结果。

推荐路线：

| 阶段 | 策略 |
|------|------|
| 短期 | 编译期内置插件 + 数据库配置启用 |
| 中期 | 前端管理配置、顺序、Agent 绑定、日志 |
| 长期 | 如需第三方扩展，优先使用外部进程 / gRPC / MCP / WASM 插件沙箱 |

---

## 2. 内置 Plugin 使用场景

### 2.1 运行日志和审计（runtime_audit）

定位：调试和可观测。

使用场景：

- 开发环境排查 Agent 执行链路。
- 追踪 Agent、模型、工具调用和最终回复。
- 定位某个 Agent 为什么调用了某个工具。
- 比较不同模型或不同 Agent 配置下的行为。

默认策略：

- 开发环境可默认启用。
- 生产环境默认停用，或只记录脱敏摘要。

注册回调点：`BeforeAgent`、`AfterAgent`、`BeforeModel`、`AfterModel`、`BeforeTool`、`AfterTool`、`OnEvent`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `log_model_request` | boolean | true | 是否记录模型请求摘要 |
| `log_model_response` | boolean | true | 是否记录模型响应摘要 |
| `log_tool_args` | boolean | true | 是否记录工具参数摘要 |
| `max_content_length` | number | 500 | 单段日志最大字符数 |
| `redact_sensitive` | boolean | true | 是否脱敏敏感字段 |

### 2.2 工具失败自愈（retry_and_reflect）

定位：工具失败后的自愈和反思重试。

使用场景：

- 工具参数格式错误。
- SQL / 搜索 / 文件操作等工具返回可恢复错误。
- 模型需要根据错误信息修正下一次工具调用。
- Skill 执行失败但错误信息明确。

默认策略：

- 对通用 Agent 可默认启用。
- 对高风险工具只允许生成反思提示，不自动重复危险操作。

注册回调点：`AfterTool`（检查 `args.Error != nil` 判断工具失败）

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_retries` | number | 3 | 单个工具最大反思重试次数 |
| `tracking_scope` | enum | `invocation` | `invocation` / `global` |
| `error_if_retry_exceeded` | boolean | false | 超过次数后是否直接返回原错误 |
| `excluded_tools` | string[] | [] | 不允许重试的工具 |
| `high_risk_tools_need_confirm` | boolean | true | 高风险工具重试前是否需要确认 |

### 2.3 输入输出脱敏（sensitive_data_mask）

定位：模型请求前脱敏，模型响应后审查。

使用场景：

- 隐藏 API Key、Token、数据库连接串。
- 脱敏邮箱、手机号、身份证号等隐私字段。
- 防止模型输出密钥或内部配置。

注册回调点：`BeforeModel`、`AfterModel`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mask_email` | boolean | true | 是否脱敏邮箱 |
| `mask_phone` | boolean | true | 是否脱敏手机号 |
| `mask_secret` | boolean | true | 是否脱敏密钥 |
| `custom_patterns` | object[] | [] | 自定义正则脱敏规则 |
| `block_leak_output` | boolean | true | 输出疑似泄漏时是否阻断 |

### 2.4 高风险操作确认（confirmation_guard）

定位：高风险操作的确认机制。

使用场景：

- 删除文件。
- 执行脚本。
- 修改数据库。
- 调用外部支付 / 邮件 / 发布 API。

注册回调点：`BeforeTool`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `confirm_tools` | string[] | [] | 需要确认的工具 |
| `confirm_patterns` | string[] | [] | 参数命中后需要确认 |
| `timeout_seconds` | number | 300 | 确认超时时间 |
| `default_action` | enum | `reject` | 超时默认行为：`reject` / `allow` |

### 2.5 模型成本控制（cost_guard）

定位：模型调用前的成本和额度控制。

使用场景：

- 超过 token 预算拒绝执行。
- 非管理员不能使用高价模型。
- 单个 Agent 每日调用次数限制。
- 预算不足时自动降级模型。

注册回调点：`BeforeModel`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `daily_token_budget` | number | 0 | 每日 token 预算，0 表示不限制 |
| `max_prompt_tokens` | number | 0 | 单次请求最大 prompt token |
| `blocked_models` | string[] | [] | 禁用模型 |
| `fallback_model` | string | "" | 预算不足时降级模型 |
| `admin_bypass` | boolean | true | 管理员是否绕过限制 |

### 2.6 模型路由（model_router）

定位：模型请求前的动态路由。

使用场景：

- 简单问答走便宜模型。
- 代码生成走强模型。
- 长上下文任务走大上下文模型。
- 中文 / 英文任务使用不同 provider。

注册回调点：`BeforeModel`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `rules` | object[] | [] | 路由规则列表 |
| `default_model` | string | "" | 默认模型 |
| `code_model` | string | "" | 代码任务模型 |
| `long_context_model` | string | "" | 长上下文模型 |
| `fallback_model` | string | "" | 失败回退模型 |

### 2.7 工具权限控制（permission_guard）

定位：工具调用前的权限检查。

使用场景：

- 普通用户不能调用删除类工具。
- 某些 Agent 不能访问数据库、文件系统或外部 API。
- 上传 Skill 后不允许自动执行脚本。
- 高风险工具调用前需要二次确认。

注册回调点：`BeforeTool`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `deny_tools` | string[] | [] | 禁止调用的工具 |
| `confirm_tools` | string[] | [] | 需要确认的工具 |
| `agent_allowlist` | string[] | [] | 允许生效的 Agent |
| `role_rules` | object[] | [] | 基于角色的工具权限规则 |

### 2.8 输出策略检查（output_policy）

定位：模型输出后的策略检查。

使用场景：

- 禁止输出密钥。
- 禁止生成危险命令。
- 禁止直接执行破坏性操作说明。
- 检查是否包含未授权数据。

注册回调点：`AfterModel`、`OnEvent`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `blocked_patterns` | string[] | [] | 禁止输出的模式 |
| `dangerous_command_check` | boolean | true | 是否检查危险命令 |
| `block_on_violation` | boolean | true | 命中策略时是否阻断 |
| `replacement_message` | string | "" | 阻断时返回的说明 |

### 2.9 Skill 调用统计（skill_usage_tracker）

定位：自动统计 Skill 使用频率和质量。

使用场景：

- 记录哪个 Agent 调用了哪个 Skill。
- 统计成功次数、失败次数、耗时。
- 记录最近调用 Agent、时间和输出摘要。
- 支撑 Skill 管理页的使用频率统计。

注册回调点：`BeforeTool`、`AfterTool`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `track_success` | boolean | true | 是否记录成功调用 |
| `track_failure` | boolean | true | 是否记录失败调用 |
| `capture_input_preview` | boolean | true | 是否记录输入摘要 |
| `capture_output_preview` | boolean | true | 是否记录输出摘要 |
| `max_preview_length` | number | 500 | 摘要最大长度 |

### 2.10 第一批内置 Plugin 落地范围

| Plugin | Key | Category | RiskLevel | 默认状态 | Callback Points |
|--------|-----|----------|-----------|----------|-----------------|
| 运行日志和审计 | `runtime_audit` | observability | low | 开发环境建议启用 | BeforeAgent, AfterAgent, BeforeModel, AfterModel, BeforeTool, AfterTool, OnEvent |
| Skill 调用统计 | `skill_usage_tracker` | tracking | low | 可启用 | BeforeTool, AfterTool |
| 工具失败自愈 | `retry_and_reflect` | debug | medium | 可启用 | AfterTool |
| 输入输出脱敏 | `sensitive_data_mask` | guard | medium | 建议启用 | BeforeModel, AfterModel |
| 高风险操作确认 | `confirmation_guard` | guard | high | 默认启用但默认拒绝 | BeforeTool |
| 模型成本控制 | `cost_guard` | guard | medium | 可启用 | BeforeModel |
| 模型路由 | `model_router` | routing | low | 可启用 | BeforeModel |
| 工具权限控制 | `permission_guard` | guard | high | 可启用 | BeforeTool |
| 输出策略检查 | `output_policy` | policy | medium | 可启用 | AfterModel, OnEvent |

管理页启用：

- 后端启动时同步内置插件到数据库 `plugins` 表。
- `/plugins` 页面展示内置插件、启用状态、风险等级、callback 点和 JSON 配置。
- 页面启停后，`plugintrpc.Runtime.Apply()` 热重载生效，下次 Runner 创建时获取最新插件列表。

第一批安全策略：

- `sensitive_data_mask` 默认不会保留原始敏感值，只返回掩码文本。
- `runtime_audit` 默认只记录摘要，摘要长度限制为 500 字符。
- `confirmation_guard` 对高风险工具默认 `reject`，除非后续接入明确的人类确认结果。
- `retry_and_reflect` 不应自动重试高风险工具，高风险工具应先经过确认插件。
- `skill_usage_tracker` 第一阶段可先记录运行时摘要；后续接入持久化后作为 Skill 调用统计的主要来源。
- `cost_guard` 默认对高价模型执行保护；如果配置了 fallback model，则优先降级而不是拒绝。
- `model_router` 只改写 ADK `LLMRequest.Model`，Provider、API Key、base URL 仍复用当前已选 ProviderModel。

第一批验收标准：

- 输入含 `sk-...`、`token=...`、邮箱、手机号时，进入模型前被脱敏。
- 模型输出疑似泄漏密钥时，响应被脱敏或阻断。
- 高风险工具调用在未确认时被阻断，并返回明确原因。
- Skill 工具成功 / 失败时产生统计摘要。
- 工具失败时 Retry and Reflect 插件能被注册到 Runner。
- 高价模型或超预算请求可以被拒绝或降级。
- 代码任务、长上下文任务、指定 Agent 可以触发模型路由。
- 默认 `direct` backend 不受影响。

---

## 3. Plugin 管理页

### 3.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Plugin 管理」 |
| 副标题 | 「管理 Agent 运行时治理、调试和增强插件」 |
| 顶部统计 | 总插件数、已启用、最近错误、今日调用次数 |
| 工具栏 | 搜索、类型筛选、启用状态筛选、作用阶段筛选、刷新、运行记录入口 |
| 主表 | 展示插件列表，支持服务端分页 |
| 详情抽屉 | 展示配置、绑定 Agent、运行统计、最近日志 |

### 3.2 表格列

| 列 | 字段 | UI 与交互 |
|----|------|-----------|
| 名称 | `name`、`key` | 主行名称，副行 key |
| 类型 | `category` | `observability` / `guard` / `routing` / `policy` / `tracking` / `debug` |
| 作用阶段 | `callback_points[]` | 用 `QChip` 展示 callback |
| 状态 | `enabled` | `QToggle`；无权限时禁用 |
| 范围 | `scope` | `global` / `agent` |
| 顺序 | `sort_order` | 展示数字；管理员可上移 / 下移 |
| 统计 | `invoke_count`、`error_count` | 展示调用、错误 |
| 最近执行 | `last_invoked_at`、`last_status` | 无值展示「未运行」 |
| 操作 | `permissions` | 配置、绑定 Agent、查看日志 |

### 3.3 详情抽屉

点击「配置」打开右侧 `QDrawer` 或 `QDialog`：

| Tab | 内容 |
|-----|------|
| 基础信息 | 名称、描述、类型、作用阶段、风险等级 |
| 配置 | 后端 schema 渲染动态表单 |
| Agent 绑定 | 选择生效 Agent；支持全局 / 指定 Agent |
| 运行统计 | 调用次数、拦截次数、错误次数 |
| 最近日志 | 最近 20 条运行摘要 |

配置保存行为：

1. 用户修改配置。
2. 前端本地校验 schema。
3. 调用 `PUT /v1/plugins/{id}/config`。
4. 成功后刷新该行与详情。
5. 失败时展示后端错误，不修改本地最终状态。

### 3.4 执行顺序

需求：

- 同一作用域内插件按 `sort_order` 升序执行。
- 管理员可通过上移 / 下移调整。
- 保存后调用 `PATCH /v1/plugins/{id}/sort-order`。
- 调整顺序需要提示：顺序可能影响运行结果。

---

## 4. Plugin 运行记录页

### 4.1 路由

| 路由 | 页面 | 说明 |
|------|------|------|
| `/plugins` | Plugin 管理 | 列表、启用、配置、排序、绑定 Agent |
| `/plugins/runs` | Plugin 运行记录 | 查看 Plugin callback 执行记录 |

### 4.2 筛选项

| 筛选项 | 参数 | 说明 |
|--------|------|------|
| Plugin | `plugin_id` | 指定插件 |
| Agent | `agent_id` | 指定 Agent |
| Callback | `callback_point` | 如 `before_model` / `before_tool` |
| 状态 | `status` | `success` / `blocked` / `error` / `skipped` |
| 时间范围 | `from` / `to` | ISO 时间 |

### 4.3 表格列

| 列 | 字段 | 说明 |
|----|------|------|
| 时间 | `started_at` | 执行时间 |
| Plugin | `plugin_name` | 插件名称 |
| Agent | `agent_display_name` | 关联 Agent |
| Callback | `callback_point` | 执行节点 |
| 状态 | `status` | 成功、阻断、错误、跳过 |
| 耗时 | `duration_ms` | callback 执行耗时 |
| 动作 | `action` | `pass` / `modify` / `block` / `retry` |
| 摘要 | `summary` | 脱敏后的输入输出摘要 |
| 错误 | `error_message` | 错误摘要 |

---

## 5. 数据模型

### 5.1 Plugin

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 唯一标识 |
| key | string | 插件唯一 Key，如 `runtime_audit` |
| name | string | 显示名称 |
| description | string | 插件描述 |
| category | enum | `observability` / `guard` / `routing` / `policy` / `tracking` / `debug` |
| risk_level | enum | `low` / `medium` / `high` |
| enabled | boolean | 是否启用 |
| scope | string | `"global"` 或 agent_id |
| callback_points | string[] | 注册的回调点列表 |
| sort_order | number | 执行顺序，数字越小越先执行 |
| config_schema_json | string | JSON Schema 定义配置项 |
| config_json | string | 当前生效配置 |
| default_config_json | string | 出厂默认配置 |
| invoke_count | number | 调用次数 |
| block_count | number | 拦截次数 |
| error_count | number | 错误次数 |
| last_invoked_at | string | 最近调用时间 |
| last_status | enum | `success` / `blocked` / `error` |
| created_at | string | 创建时间 |
| updated_at | string | 更新时间 |
| permissions | PluginPermissions | 操作权限 |

### 5.2 PluginCallbackPoint

框架支持的 7 个回调点：`before_agent`、`after_agent`、`before_model`、`after_model`、`before_tool`、`after_tool`、`on_event`。

### 5.3 PluginPermissions

| 字段 | 类型 | 说明 |
|------|------|------|
| can_view | boolean | 可查看 |
| can_toggle | boolean | 可启停 |
| can_edit_config | boolean | 可修改配置 |
| can_view_logs | boolean | 可查看日志 |

### 5.4 PluginRun

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 记录 ID |
| plugin_id | string | 插件 ID |
| plugin_name | string | 插件名称 |
| agent_id | string? | 关联 Agent |
| agent_display_name | string? | Agent 显示名 |
| invocation_id | string? | 调用 ID |
| callback_point | PluginCallbackPoint | 回调点 |
| status | enum | `success` / `blocked` / `error` / `skipped` |
| action | enum | `pass` / `modify` / `block` / `retry` / `route` / `mask` |
| started_at | string | 执行时间 |
| duration_ms | number | 耗时 |
| input_preview | string? | 脱敏输入摘要 |
| output_preview | string? | 脱敏输出摘要 |
| summary | string? | 执行摘要 |
| error_message | string? | 错误摘要 |

> **具体 Proto / Go / TypeScript 类型定义**参见设计文档 §2 和 §3。

---

## 6. API 契约

### 6.1 Plugin 列表

`GET /v1/plugins`

Query：

| 参数 | 类型 | 说明 |
|------|------|------|
| `search` | string | 名称、key、描述 |
| `category` | string | 类型 |
| `enabled` | string | "" / "true" / "false" 三态 |
| `callback_point` | string | 作用阶段 |
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

### 6.2 启用 / 停用

`PATCH /v1/plugins/{id}/enabled`

Request：

```json
{
  "enabled": true
}
```

Response：返回更新后的 `Plugin`。

### 6.3 更新配置

`PUT /v1/plugins/{id}/config`

Request：

```json
{
  "config_json": "{\"max_retries\": 3}"
}
```

Response：返回更新后的 `Plugin`。

### 6.4 调整顺序

`PATCH /v1/plugins/{id}/sort-order`

Request：

```json
{
  "sort_order": 10
}
```

Response：返回更新后的 `Plugin`。

### 6.5 更新作用域

`PATCH /v1/plugins/{id}/scope`

Request：

```json
{
  "scope": "global"
}
```

`scope` 取值：`"global"` 表示全局生效，或传入具体 `agent_id` 表示仅对该 Agent 生效。

Response：返回更新后的 `Plugin`。

### 6.6 运行记录

`GET /v1/plugins/runs`

Query：

| 参数 | 类型 | 说明 |
|------|------|------|
| `plugin_id` | string | 插件 |
| `agent_id` | string | Agent |
| `callback_point` | string | callback 类型 |
| `status` | string | 状态 |
| `from` | string | 开始时间 |
| `to` | string | 结束时间 |
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

---

## 7. 已确认决策

| 决策项 | 决策 | 依据 |
|--------|------|------|
| Plugin 是否按工作区/租户隔离 | 否，本期全局共享 | 当前为单租户架构，无需隔离 |
| 生产环境是否允许开启 `runtime_audit` 完整日志 | 允许但默认关闭 | 完整日志含敏感数据，需管理员主动启用并配置脱敏 |
| `retry_and_reflect` 对高风险工具是否默认禁止自动重试 | 是 | 高风险工具应先经过 `confirmation_guard` 确认，不自动重试 |
| `skill_usage_tracker` 是否作为 Skill 统计唯一来源 | 否，第一阶段为运行时摘要 | 后续接入持久化后作为主要来源，当前为辅助 |
| Plugin 注入 ADK Runner 的优先级 | 高于其他运行时能力 | Plugin 是治理和风控的基础设施，应优先接入 |

> **后端技术设计**（种子同步、热重载、配置校验、Agent 绑定、统计更新等）参见 [22 plugin.design.md](./22%20plugin.design.md)。

---

## 8. UI / UX 要求

### 8.1 设计风格

- 使用 Quasar 2 + Vue 3 + TypeScript。
- 与现有白昼模式保持一致，使用 `app-page-cream` 页面壳。
- 列表、筛选卡、详情抽屉使用现有奶油白卡片样式。
- 暗色模式需要有独立对比度，不依赖简单反色。

### 8.2 组件化建议

| 组件 | 说明 |
|------|------|
| `PluginPage.vue` | 页面编排 |
| `PluginFilterBar.vue` | 搜索和筛选 |
| `PluginTable.vue` | 插件列表 |
| `PluginStatsStrip.vue` | 顶部统计 |
| `PluginConfigDrawer.vue` | 配置详情抽屉 |
| `PluginConfigForm.vue` | 根据 schema 渲染表单 |
| `PluginRunTable.vue` | 运行记录表 |
| `PluginRiskBadge.vue` | 风险等级展示 |

### 8.3 Agent 绑定交互

Plugin 详情抽屉的「Agent 绑定」Tab：

| 选项 | 说明 |
|------|------|
| 全局生效 | 插件对所有 Agent 生效（默认） |
| 指定 Agent | 从 Agent 列表中选择一个 Agent，插件仅对该 Agent 生效 |

交互流程：
1. 默认选中「全局生效」。
2. 切换到「指定 Agent」时，展示 Agent 选择下拉框。
3. 选择 Agent 后，调用 `PATCH /v1/plugins/{id}/scope` 保存。
4. 切回「全局生效」时，scope 设为 `"global"`。
5. 保存后提示：作用域变更将在下次对话时生效。

---

## 9. 验收标准

### 9.1 Plugin 管理页

- 能查看所有内置 Plugin。
- 能按搜索、类型、启用状态、callback 阶段筛选。
- 能启用 / 停用 Plugin。
- 能修改 Plugin 配置，并通过 schema 校验。
- 能调整执行顺序。
- 能绑定到全局或指定 Agent。
- 无权限时按钮禁用或隐藏。
- 页面支持白昼 / 暗色模式。

### 9.2 运行记录

- 能查看 Plugin callback 执行记录。
- 能按 Plugin、Agent、callback、状态、时间筛选。
- 输入输出只展示脱敏摘要。
- 错误记录能显示错误摘要和耗时。

### 9.3 安全

- 前端无法上传任意插件代码。
- 后端不接受动态 callback 代码。
- 高风险插件默认停用。
- 修改高风险插件配置需要管理员权限。
- 敏感日志默认脱敏。
