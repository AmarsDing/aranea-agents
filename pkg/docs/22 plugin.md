# Plugin 管理（Quasar UI + 前后端契约）

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
| 真正注入 ADK Runner | 后续接入 | 当前 `ADKRuntimeAdapter` 仍是直连模型适配；后续真实接入 `adk-go runner.PluginConfig` 后生效 |

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

`adk-go/plugin` 的 Plugin 是运行时回调对象。每个 Plugin 通过 `plugin.New(plugin.Config{...})` 注册一组 callback：

| Callback | 触发时机 | 典型用途 |
|----------|----------|----------|
| `OnUserMessageCallback` | 用户消息进入运行链路时 | 输入改写、脱敏、审计 |
| `BeforeRunCallback` | 单次 Run 开始前 | 预算检查、环境准备、提前拒绝 |
| `AfterRunCallback` | 单次 Run 结束后 | 清理、统计、日志落库 |
| `BeforeAgentCallback` | Agent 执行前 | Agent 权限检查、上下文注入 |
| `AfterAgentCallback` | Agent 执行后 | 统计、审计 |
| `BeforeModelCallback` | 模型请求前 | 模型路由、脱敏、成本控制、改写请求 |
| `AfterModelCallback` | 模型响应后 | 输出审查、记录 token、响应改写 |
| `OnModelErrorCallback` | 模型请求失败时 | 降级模型、错误记录 |
| `BeforeToolCallback` | 工具执行前 | 权限检查、参数补充、确认机制 |
| `AfterToolCallback` | 工具执行后 | 结果审计、统计、结果改写 |
| `OnToolErrorCallback` | 工具执行失败时 | 重试、反思、自愈、错误治理 |

### 1.2 调用顺序

运行时创建 `Runner` 时传入：

```go
runner.PluginConfig{
  Plugins: []*plugin.Plugin{...}
}
```

`Runner` 创建 `PluginManager`，执行期间由 `Runner` / LLM flow 在固定节点主动调用。Plugin 不是编译器自动插入，也不是前端配置后执行任意代码。

执行规则：

1. Plugin 按 `PluginConfig.Plugins` 数组顺序执行。
2. 部分 callback 如果返回非空结果，会提前结束后续同类 callback。
3. callback 返回 error 时，当前执行链路按 ADK 运行规则中断或进入错误处理。
4. `CloseFunc` 在 PluginManager 关闭时执行，用于释放资源。

### 1.3 第三方扩展边界

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

### 2.1 Logging / Trace Plugin

定位：调试和可观测。

使用场景：

- 开发环境排查 Agent 执行链路。
- 追踪用户消息、Agent、模型、工具、事件和最终回复。
- 定位某个 Agent 为什么调用了某个工具。
- 比较不同模型或不同 Agent 配置下的行为。

默认策略：

- 开发环境可默认启用。
- 生产环境默认停用，或只记录脱敏摘要。

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `log_user_message` | boolean | true | 是否记录用户输入摘要 |
| `log_model_request` | boolean | true | 是否记录模型请求摘要 |
| `log_model_response` | boolean | true | 是否记录模型响应摘要 |
| `log_tool_args` | boolean | true | 是否记录工具参数摘要 |
| `max_content_length` | number | 500 | 单段日志最大字符数 |
| `redact_sensitive` | boolean | true | 是否脱敏敏感字段 |

### 2.2 Retry and Reflect Plugin

定位：工具失败后的自愈和反思重试。

使用场景：

- 工具参数格式错误。
- SQL / 搜索 / 文件操作等工具返回可恢复错误。
- 模型需要根据错误信息修正下一次工具调用。
- Skill 执行失败但错误信息明确。

默认策略：

- 对通用 Agent 可默认启用。
- 对高风险工具只允许生成反思提示，不自动重复危险操作。

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_retries` | number | 3 | 单个工具最大反思重试次数 |
| `tracking_scope` | enum | `invocation` | `invocation` / `global` |
| `error_if_retry_exceeded` | boolean | false | 超过次数后是否直接返回原错误 |
| `excluded_tools` | string[] | [] | 不允许重试的工具 |
| `high_risk_tools_need_confirm` | boolean | true | 高风险工具重试前是否需要确认 |

### 2.3 Function Call Modifier Plugin

定位：动态修改工具 schema，并处理模型生成的 function call 参数。

使用场景：

- 给特定工具临时注入 `workspace_id`、`tenant_id`。
- 向工具 schema 增加系统字段，但不希望模型直接控制最终值。
- 改写工具描述，引导模型正确调用。
- 模型返回 function call 后，把部分参数转存到 state。

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `target_tools` | string[] | [] | 需要修改的工具名 |
| `injected_args` | object | {} | 要注入的参数 schema |
| `override_description` | string | "" | 可选的工具描述覆盖模板 |
| `state_key_prefix` | string | "" | 转存 state 时的 key 前缀 |

### 2.4 Permission Guard Plugin

定位：工具调用前的权限检查。

使用场景：

- 普通用户不能调用删除类工具。
- 某些 Agent 不能访问数据库、文件系统或外部 API。
- 上传 Skill 后不允许自动执行脚本。
- 高风险工具调用前需要二次确认。

建议 callback：

- `BeforeToolCallback`
- 必要时结合 `BeforeAgentCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `deny_tools` | string[] | [] | 禁止调用的工具 |
| `confirm_tools` | string[] | [] | 需要确认的工具 |
| `agent_allowlist` | string[] | [] | 允许生效的 Agent |
| `role_rules` | object[] | [] | 基于角色的工具权限规则 |

### 2.5 Cost Guard Plugin

定位：模型调用前的成本和额度控制。

使用场景：

- 超过 token 预算拒绝执行。
- 非管理员不能使用高价模型。
- 单个 Agent 每日调用次数限制。
- 预算不足时自动降级模型。

建议 callback：

- `BeforeModelCallback`
- `OnModelErrorCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `daily_token_budget` | number | 0 | 每日 token 预算，0 表示不限制 |
| `max_prompt_tokens` | number | 0 | 单次请求最大 prompt token |
| `blocked_models` | string[] | [] | 禁用模型 |
| `fallback_model` | string | "" | 预算不足时降级模型 |
| `admin_bypass` | boolean | true | 管理员是否绕过限制 |

### 2.6 Model Router Plugin

定位：模型请求前的动态路由。

使用场景：

- 简单问答走便宜模型。
- 代码生成走强模型。
- 长上下文任务走大上下文模型。
- 中文 / 英文任务使用不同 provider。

建议 callback：

- `BeforeModelCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `rules` | object[] | [] | 路由规则列表 |
| `default_model` | string | "" | 默认模型 |
| `code_model` | string | "" | 代码任务模型 |
| `long_context_model` | string | "" | 长上下文模型 |
| `fallback_model` | string | "" | 失败回退模型 |

### 2.7 Sensitive Data Mask Plugin

定位：模型请求前脱敏，模型响应后审查。

使用场景：

- 隐藏 API Key、Token、数据库连接串。
- 脱敏邮箱、手机号、身份证号等隐私字段。
- 防止模型输出密钥或内部配置。

建议 callback：

- `BeforeModelCallback`
- `AfterModelCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mask_email` | boolean | true | 是否脱敏邮箱 |
| `mask_phone` | boolean | true | 是否脱敏手机号 |
| `mask_secret` | boolean | true | 是否脱敏密钥 |
| `custom_patterns` | object[] | [] | 自定义正则脱敏规则 |
| `block_leak_output` | boolean | true | 输出疑似泄漏时是否阻断 |

### 2.8 Output Policy Plugin

定位：模型输出后的策略检查。

使用场景：

- 禁止输出密钥。
- 禁止生成危险命令。
- 禁止直接执行破坏性操作说明。
- 检查是否包含未授权数据。

建议 callback：

- `AfterModelCallback`
- `OnEventCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `blocked_patterns` | string[] | [] | 禁止输出的模式 |
| `dangerous_command_check` | boolean | true | 是否检查危险命令 |
| `block_on_violation` | boolean | true | 命中策略时是否阻断 |
| `replacement_message` | string | "" | 阻断时返回的说明 |

### 2.9 Skill Usage Tracker Plugin

定位：自动统计 Skill 使用频率和质量。

使用场景：

- 记录哪个 Agent 调用了哪个 Skill。
- 统计成功次数、失败次数、耗时。
- 记录最近调用 Agent、时间和输出摘要。
- 支撑 Skill 管理页的使用频率统计。

建议 callback：

- `BeforeToolCallback`
- `AfterToolCallback`
- `OnToolErrorCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `track_success` | boolean | true | 是否记录成功调用 |
| `track_failure` | boolean | true | 是否记录失败调用 |
| `capture_input_preview` | boolean | true | 是否记录输入摘要 |
| `capture_output_preview` | boolean | true | 是否记录输出摘要 |
| `max_preview_length` | number | 500 | 摘要最大长度 |

### 2.10 Confirmation Plugin

定位：高风险操作的确认机制。

使用场景：

- 删除文件。
- 执行脚本。
- 修改数据库。
- 调用外部支付 / 邮件 / 发布 API。
- 上传含高风险文件的 Skill 后触发执行。

建议 callback：

- `BeforeToolCallback`

配置项：

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `confirm_tools` | string[] | [] | 需要确认的工具 |
| `confirm_patterns` | string[] | [] | 参数命中后需要确认 |
| `timeout_seconds` | number | 300 | 确认超时时间 |
| `default_action` | enum | `reject` | 超时默认行为：`reject` / `allow` |

### 2.11 第一批内置 Plugin 落地范围

第一批目标是先让 ADK Runner 模式具备基础治理能力，采用 **后端内置注册 + 环境变量启用** 的方式先跑通 callback 链路；后续再与 Plugin 管理页、数据库配置、Agent 绑定打通。

本批实现插件：

| Plugin | Key | 默认状态 | Callback | 本期行为 |
|--------|-----|----------|----------|----------|
| 运行日志和审计 | `runtime_audit` | 开发环境建议启用 | `OnUserMessage`、`BeforeModel`、`AfterModel`、`BeforeTool`、`AfterTool`、`OnToolError`、`OnEvent` | 记录 Agent、模型、工具调用链路的脱敏摘要 |
| Skill 调用统计 | `skill_usage_tracker` | 可启用 | `BeforeTool`、`AfterTool`、`OnToolError` | 识别 Skill 工具调用，记录成功 / 失败、耗时、Agent 摘要；正式落库后更新 Skill 统计字段 |
| 工具失败自愈 | `retry_and_reflect` | 可启用 | `OnToolError` | 复用 ADK 内置 Retry and Reflect，对可恢复工具错误生成反思重试 |
| 输入输出脱敏 | `sensitive_data_mask` | 建议启用 | `OnUserMessage`、`BeforeModel`、`AfterModel` | 对 API Key、Token、邮箱、手机号、连接串等敏感内容进行掩码 |
| 高风险操作确认 | `confirmation_guard` | 默认启用但默认拒绝 | `BeforeTool` | 删除文件、执行脚本、数据库写操作等高风险工具在未确认时被阻断 |
| 模型成本控制 | `cost_guard` | 可启用 | `BeforeModel` | 检查 prompt token 预算、高价模型、禁用模型；必要时降级到 fallback model |
| 模型路由 | `model_router` | 可启用 | `BeforeModel` | 按 Agent、代码任务、长上下文路由模型 |
| 工具权限控制 | `permission_guard` | 可启用 | `BeforeTool` | 按 Agent allowlist、工具 denylist、高风险策略拦截工具调用 |
| 输出策略检查 | `output_policy` | 可启用 | `AfterModel` | 拦截危险命令、密钥泄漏等输出 |

启用方式：

```bash
RUNTIME_BACKEND=adk_runner
ADK_RUNNER_PLUGINS=runtime_audit,sensitive_data_mask,confirmation_guard,retry_and_reflect,skill_usage_tracker,cost_guard,model_router,permission_guard,output_policy
```

管理页启用：

- 后端启动时同步内置插件到数据库 `plugins` 表。
- `/plugins` 页面展示内置插件、启用状态、风险等级、callback 点和 JSON 配置。
- 页面启停后，`adk_runner` 优先读取数据库启用状态和 `config_json` 生成 `runner.PluginConfig`。
- 如果未接入数据库 PluginSource，才回退读取 `ADK_RUNNER_PLUGINS` 环境变量。

模型成本控制环境变量：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `ADK_COST_MAX_PROMPT_TOKENS` | `0` | 单次 prompt token 预算，0 表示不限制 |
| `ADK_COST_BLOCKED_MODELS` | 空 | 禁用模型列表，逗号分隔 |
| `ADK_COST_FALLBACK_MODEL` | 空 | 命中预算或禁用模型时降级到该模型 |
| `ADK_COST_DEFAULT_MODEL` | 空 | 请求未指定模型时使用 |
| `ADK_COST_BLOCK_PREMIUM_MODELS` | `true` | 是否默认限制 `opus`、`gpt-4o`、`gpt-5`、`o1/o3`、`pro/ultra` 等高价模型 |

模型路由环境变量：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `ADK_ROUTER_DEFAULT_MODEL` | 空 | 请求未指定模型时的默认模型 |
| `ADK_ROUTER_CODE_MODEL` | 空 | 代码 / 调试任务使用的模型 |
| `ADK_ROUTER_LONG_CONTEXT_MODEL` | 空 | 长上下文任务使用的模型 |
| `ADK_ROUTER_LONG_CONTEXT_TOKENS` | `8000` | 触发长上下文模型的 prompt token 阈值 |
| `ADK_ROUTER_AGENT_MODELS` | 空 | Agent 到模型的映射，格式：`agent_a:model_a,agent_b:model_b` |

本期配置来源：

| 来源 | 说明 |
|------|------|
| 环境变量 `ADK_RUNNER_PLUGINS` | 控制启用哪些内置插件，逗号分隔 |
| 内置默认配置 | 每个插件带安全默认值 |
| 代码注册表 | 后端只允许注册已编译内置插件 |

第一批安全策略：

- `sensitive_data_mask` 默认不会保留原始敏感值，只返回掩码文本。
- `runtime_audit` 默认只记录摘要，摘要长度限制为 500 字符。
- `confirmation_guard` 对高风险工具默认 `reject`，除非后续接入明确的人类确认结果。
- `retry_and_reflect` 不应自动重试高风险工具，高风险工具应先经过确认插件。
- `skill_usage_tracker` 第一阶段可先记录运行时摘要；后续接入持久化后作为 Skill 调用统计的主要来源。
- `cost_guard` 默认对高价模型执行保护；如果配置了 fallback model，则优先降级而不是拒绝。
- `model_router` 只改写 ADK `LLMRequest.Model`，Provider、API Key、base URL 仍复用当前已选 ProviderModel。

第一批验收标准：

- `adk_runner` backend 下可通过 `ADK_RUNNER_PLUGINS` 启用第一批内置插件。
- 输入含 `sk-...`、`token=...`、邮箱、手机号时，进入模型前被脱敏。
- 模型输出疑似泄漏密钥时，响应被脱敏或阻断。
- 高风险工具调用在未确认时被阻断，并返回明确原因。
- Skill 工具成功 / 失败时产生统计摘要。
- 工具失败时 Retry and Reflect 插件能被注册到 Runner。
- 高价模型或超预算请求可以被拒绝或降级。
- 代码任务、长上下文任务、指定 Agent 可以触发模型路由。
- 默认 `direct` backend 不受影响。

### 2.12 Plugin 应用场景实现状态

| 后端能力 | 适合 Plugin | 当前实现状态 | 已有 Key / 后续 Key | 备注 |
|----------|-------------|--------------|---------------------|------|
| Skill 调用统计 | 是 | 部分完成 | `skill_usage_tracker` | 已有运行时内存统计摘要；待落库到 `skill_invocation` 和 Skill 统计字段 |
| 工具权限控制 | 是 | 已完成基础版 | `permission_guard` | 支持 Agent allowlist、工具 denylist、高风险工具拦截；待接入用户角色 |
| 高风险操作确认 | 是 | 已完成基础版 | `confirmation_guard` | 当前未接入人类确认 UI，默认阻断 |
| 模型成本控制 | 是 | 已完成基础版 | `cost_guard` | 支持 prompt token 预算、禁用模型、高价模型保护、fallback model |
| 模型路由 | 是 | 已完成基础版 | `model_router` | 支持 Agent、代码任务、长上下文模型路由 |
| 输入输出脱敏 | 是 | 已完成基础版 | `sensitive_data_mask` | 支持 API Key、Token、邮箱、手机号、连接串 |
| 工具失败自愈 | 是 | 已接入 | `retry_and_reflect` | 复用 ADK 内置插件 |
| 运行日志和审计 | 是 | 已完成基础版 | `runtime_audit` | 当前输出到后端日志；待写入 `plugin_run` |
| 输出策略检查 | 是 | 已完成基础版 | `output_policy` | 支持危险命令、密钥泄漏拦截；待接入内容合规模型 |
| 工作区上下文注入 | 是 | 未完成 | `workspace_context_injector` | 待在 `BeforeToolCallback` 中补 `workspace_id` |

未完成项必须进入后续迭代：

1. `permission_guard`：继续接入用户角色、租户角色和细粒度工具风险等级规则。
2. `output_policy`：继续接入内容合规模型、可配置 blocked_patterns、审计落库。
3. `workspace_context_injector`：工具参数中自动注入 `workspace_id`。
4. Plugin Run 落库：将当前日志摘要写入 `plugin_run`，支持前端检索。
5. Skill 使用统计持久化：将 `skill_usage_tracker` 写入 Skill 调用记录和统计字段。

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
| 类型 | `category` | `observability` / `guard` / `routing` / `policy` / `tracking` |
| 作用阶段 | `callback_points[]` | 用 `QChip` 展示 callback |
| 状态 | `enabled` | `QToggle`；无权限时禁用 |
| 范围 | `scope` | `global` / `agent` / `environment` |
| 顺序 | `sort_order` | 展示数字；管理员可上移 / 下移 |
| 统计 | `invoke_count`、`error_count`、`avg_duration_ms` | 展示调用、错误、耗时 |
| 最近执行 | `last_invoked_at`、`last_status` | 无值展示「未运行」 |
| 操作 | `permissions` | 配置、绑定 Agent、查看日志 |

### 3.3 详情抽屉

点击「配置」打开右侧 `QDrawer` 或 `QDialog`：

| Tab | 内容 |
|-----|------|
| 基础信息 | 名称、描述、类型、作用阶段、风险等级 |
| 配置 | 后端 schema 渲染动态表单 |
| Agent 绑定 | 选择生效 Agent；支持全局 / 指定 Agent |
| 运行统计 | 调用次数、拦截次数、错误次数、平均耗时 |
| 最近日志 | 最近 20 条运行摘要 |

配置保存行为：

1. 用户修改配置。
2. 前端本地校验 schema。
3. 调用 `PUT /plugins/:id/config`。
4. 成功后刷新该行与详情。
5. 失败时展示后端错误，不修改本地最终状态。

### 3.4 执行顺序

需求：

- 同一作用域内插件按 `sort_order` 升序执行。
- 管理员可通过上移 / 下移调整。
- 保存后调用 `PUT /plugins/order`。
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

```ts
type Plugin = {
  id: string;
  key: string;
  name: string;
  description: string;
  category: "observability" | "guard" | "routing" | "policy" | "tracking" | "debug";
  risk_level: "low" | "medium" | "high";
  enabled: boolean;
  scope: "global" | "agent" | "environment";
  callback_points: PluginCallbackPoint[];
  sort_order: number;
  config_schema: PluginConfigSchema;
  config_json: Record<string, unknown>;
  default_config_json: Record<string, unknown>;
  invoke_count: number;
  block_count: number;
  error_count: number;
  avg_duration_ms?: number;
  last_invoked_at?: string;
  last_status?: "success" | "blocked" | "error" | "skipped";
  created_at: string;
  updated_at: string;
  permissions: PluginPermissions;
};
```

### 5.2 PluginCallbackPoint

```ts
type PluginCallbackPoint =
  | "on_user_message"
  | "before_run"
  | "after_run"
  | "before_agent"
  | "after_agent"
  | "before_model"
  | "after_model"
  | "on_model_error"
  | "before_tool"
  | "after_tool"
  | "on_tool_error"
  | "on_event";
```

### 5.3 PluginPermissions

```ts
type PluginPermissions = {
  can_view: boolean;
  can_toggle: boolean;
  can_edit_config: boolean;
  can_bind_agent: boolean;
  can_reorder: boolean;
  can_view_logs: boolean;
};
```

### 5.4 PluginBinding

```ts
type PluginBinding = {
  id: string;
  plugin_id: string;
  scope: "global" | "agent" | "environment";
  agent_id?: string;
  environment?: "development" | "staging" | "production";
  enabled: boolean;
  config_override_json?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};
```

### 5.5 PluginRun

```ts
type PluginRun = {
  id: string;
  plugin_id: string;
  plugin_name: string;
  agent_id?: string;
  agent_display_name?: string;
  invocation_id?: string;
  callback_point: PluginCallbackPoint;
  status: "success" | "blocked" | "error" | "skipped";
  action: "pass" | "modify" | "block" | "retry" | "route" | "mask";
  started_at: string;
  duration_ms: number;
  input_preview?: string;
  output_preview?: string;
  summary?: string;
  error_message?: string;
};
```

---

## 6. API 契约

### 6.1 Plugin 列表

`GET /plugins`

Query：

| 参数 | 类型 | 说明 |
|------|------|------|
| `search` | string | 名称、key、描述 |
| `category` | string | 类型 |
| `enabled` | boolean | 启用状态 |
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

`PATCH /plugins/:id/enabled`

Request：

```json
{
  "enabled": true
}
```

Response：返回更新后的 `Plugin`。

### 6.3 更新配置

`PUT /plugins/:id/config`

Request：

```json
{
  "config_json": {
    "max_retries": 3,
    "tracking_scope": "invocation"
  }
}
```

Response：返回更新后的 `Plugin`。

### 6.4 调整顺序

`PUT /plugins/order`

Request：

```json
{
  "items": [
    { "plugin_id": "plugin_retry_reflect", "sort_order": 10 },
    { "plugin_id": "plugin_permission_guard", "sort_order": 20 }
  ]
}
```

Response：

```json
{
  "success": true
}
```

### 6.5 Agent 绑定

`GET /plugins/:id/bindings`

返回当前插件绑定。

`PUT /plugins/:id/bindings`

Request：

```json
{
  "bindings": [
    {
      "scope": "global",
      "enabled": true
    },
    {
      "scope": "agent",
      "agent_id": "agent_xxx",
      "enabled": true,
      "config_override_json": {}
    }
  ]
}
```

### 6.6 运行记录

`GET /plugins/runs`

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

## 7. 后端设计建议

### 7.1 内置注册表

后端维护内置插件注册表：

```go
type BuiltinPluginDefinition struct {
  Key            string
  Name           string
  Description    string
  Category       string
  RiskLevel      string
  CallbackPoints []string
  DefaultConfig  map[string]any
  ConfigSchema   map[string]any
  Factory        func(config map[string]any) (*plugin.Plugin, error)
}
```

启动时：

1. 注册内置插件定义。
2. 同步数据库中缺失的内置插件。
3. 读取启用状态、配置、排序和绑定关系。
4. 后续接入真实 ADK Runner 时构造 `runner.PluginConfig.Plugins`。

### 7.2 不允许动态代码注入

本期后端不得实现：

- 上传 `.go` 插件后直接编译运行。
- 上传 `.so` / `.dll` / `.dylib` 动态库后加载。
- 前端传入 callback 脚本。

如未来支持第三方插件，应使用隔离运行：

- 外部进程。
- gRPC。
- MCP。
- WASM 沙箱。

### 7.3 运行时接入点

当前 `aranea` 的 `ADKRuntimeAdapter` 仍是模型直连适配边界，Plugin 管理页本期可以先落库和展示。

真正生效需要后续实现：

1. `ADKRuntimeAdapter` 接入真实 `adk-go runner.Runner`。
2. 从数据库读取启用插件。
3. 根据作用域和 Agent 绑定生成 `runner.PluginConfig`。
4. 执行 callback 时写入 `plugin_run` 记录。

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
| `PluginBindingPanel.vue` | Agent 绑定 |
| `PluginRunTable.vue` | 运行记录表 |
| `PluginRiskBadge.vue` | 风险等级展示 |

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

---

## 10. 待确认问题

1. Plugin 管理页是否本期实现，还是先只补数据模型和需求文档？
2. Plugin 是否需要按工作区 / 租户隔离？
3. 生产环境是否允许开启 `LoggingPlugin` 的完整模型请求日志？
4. `RetryAndReflectPlugin` 对高风险工具是否默认禁止自动重试？
5. `SkillUsageTrackerPlugin` 是否作为 Skill 统计的唯一来源？
6. 后续真实接入 ADK Runner 的优先级是否高于其他运行时能力？
