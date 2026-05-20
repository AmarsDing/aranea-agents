# Plugin 插件 — 开发计划

> **版本**：2026-05-19 | **状态**：🟢 Phase 1–2 已落地（9 内置插件均可实例化；`audit_log` 全回调点）
> **需求**：[22 plugin.md](./22%20plugin.md) · **设计**：[22 plugin.design.md](./22%20plugin.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-CB-01

---

## 1. 模块定位

Plugin 插件系统：运行时回调扩展机制，在 Agent 执行过程中插入治理、调试、增强、风控等逻辑。Plugin 与 Skill / Tool 的边界：

- **Skill**：面向 Agent 的能力、知识、脚本和使用规范。
- **Tool**：Agent 可调用的具体外部能力。
- **Plugin**：运行时拦截器 / 中间件，改变或增强 Agent 执行链路。

**代码锚点**：
- `api/kratos/plugin/v1/plugin.proto` — Plugin CRUD RPC 定义
- `internal/service/plugin.go` — PluginService（CRUD + 热重载）
- `internal/biz/plugin.go` — PluginUsecase + PluginRepo 接口
- `internal/data/plugin.go` — PluginRepo 实现（Ent ORM）
- `internal/data/ent/schema/plugin.go` — PlatformPlugin Ent Schema
- `internal/plugin/trpc/runtime.go` — plugintrpc.Runtime（热重载）
- `internal/plugin/trpc/adapter.go` — biz.Plugin → trpcplugin.Plugin 适配
- `internal/plugin/trpc/audit.go` — AuditLogPlugin 内置插件
- `internal/plugin/trpc/chain_adapter.go` — 内置 callback_points 注册与 DB 声明校验
- `internal/plugin/trpc/registry.go` — 内置插件定义 + `BuiltinPluginDefs()`
- `internal/biz/plugin_schema.go` — JSON Schema 校验（gojsonschema）
- `internal/plugin/trpc/stats.go` — StatsRecorder + 异步 `IncrementStats`
- `internal/agent/trpc_runtime.go` — WithPlugins 注入 Runner
- `internal/agent/turn_helpers.go` — Runner 构造时传入 Plugins
- `internal/agent/trpc_build.go` — LLMAgent 构建（EP-CB-01 修改点）
- `internal/agent/callbacks/callbacks.go` — Chain 回调链抽象（已有 AdaptAgentCallbacks/AdaptModelCallbacks/AdaptToolCallbacks）
- `internal/agent/callbacks/adapter.go` — Hook 适配器（已有 BeforeAgentHookFunc/AfterAgentHookFunc/ToolRecorderCallback）
- `internal/service/trpc_turn.go:67` — `pluginRT.Plugins()` 调用点
- `internal/team/runner_team_trpc.go:123` — `pluginRT.Plugins()` 调用点

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Plugin CRUD | ✅ | List / Get / ToggleEnabled / UpdateConfig / UpdateSortOrder |
| Plugin 运行时 | ✅ | `plugintrpc.Runtime` 热重载 + `Apply()` |
| Plugin 注入 Runner | ✅ | `trpcrunner.WithPlugins(deps.Plugins...)` |
| Plugin 热重载 | ✅ | Service 层写操作后 `reloadRuntime()`（已异步化 safego） |
| 前端管理 | ✅ | Plugin 列表 / 启停 / 配置 / 详情 / 排序 |
| Callback Chain 基础 | ✅ | `callbacks.Chain` 已有 `AdaptAgentCallbacks()` / `AdaptModelCallbacks()` / `AdaptToolCallbacks()` |
| Callback 适配器（部分） | ✅ | `BeforeAgentHookFunc`、`AfterAgentHookFunc`、`ToolRecorderCallback` 已实现 |
| 内置插件注册 | ✅ | DB 种子 9 条；`adapter.builtin()` 全部 key 可实例化 |
| EP-CB-01 Chain 接入 | ✅ | Chain 挂 LLMAgent Agent/Model/Tool；Runner `WithPlugins` 保留 |
| Plugin 种子同步 | ✅ | `seedBuiltinPlugins` 启动幂等；`GetByKey` / `CreatePlugin` |
| 配置 Schema 校验 | ✅ | `validateJSONSchema`；`UpdateConfig` / `Create` |
| 统计更新 | ✅ | `RepoStatsRecorder` + `PluginRepo.IncrementStats`；AuditLog 各回调点回写 |
| Agent 绑定 | ❌ | scope 字段存在但运行时未消费，无 UpdatePluginScope API |

---

## 3. 差距与优化

1. ~~**P2**：`model_router` 真路由~~ ✅ 迭代 4：`PluginModelSelector` + `ResolveModelAPI`（`cost_guard` 仍待 Selector 对齐）。
2. **P2**：`retry_and_reflect` 本期仅 slog 反思提示，不自动重试工具调用。
3. **P2**：Agent 绑定（`scope`）运行时未消费；无 `UpdatePluginScope` API。
4. ~~**P2**：Plugin 运行记录（PluginRun）~~ ✅ `plugin_runs` + `GET /v1/plugins/runs`（2026-05-20）。
5. **P2**：Plugin 无沙箱隔离。
6. **P3**：Plugin 无版本管理。

---

## 4. 开发阶段

### Phase 1：基础设施补全（✅ 已完成）

目标：让 Plugin 系统的基础设施完整，内置插件可注册、可配置、可触发回调。

- [x] EP-CB-01 Callback Chain 接入
- [x] Plugin 种子同步（T2）
- [x] 配置 Schema 校验（T3）
- [x] 统计更新机制（T4）

### Phase 2：内置插件实现（✅ 已完成）

目标：9 个内置插件全部实现并可注册到 Runner。

实现文件：`audit.go`、`sensitive_mask.go`、`confirmation_guard.go`、`cost_guard.go`、`model_router.go`、`permission_guard.go`、`output_policy.go`、`skill_tracker.go`、`retry_reflect.go`、`config.go`。

- runtime_audit（替换 audit_log）
- sensitive_data_mask
- confirmation_guard
- cost_guard
- model_router
- permission_guard
- output_policy
- skill_usage_tracker
- retry_and_reflect

### Phase 3：Agent 绑定 + 运行记录

目标：Plugin 按 Agent 维度生效，运行记录可查询。

- scope 过滤
- PluginRun 记录表 + API
- 前端运行记录页

### Phase 4：进阶能力

目标：沙箱隔离、版本管理。

- Plugin 进程隔离
- Plugin 版本表 + 回滚 API

---

## 5. 任务清单

### Phase 1：基础设施补全

#### T1：EP-CB-01 Callback Chain 接入 LLMAgent / Model

**优先级**：P0 | **依赖**：无 | **设计参考**：§11

**代码现状**：
- `callbacks.Chain` 已有 `AdaptAgentCallbacks()` 和 `AdaptModelCallbacks()` 方法（`internal/agent/callbacks/callbacks.go`）
- `callbacks/adapter.go` 已有 `BeforeAgentHookFunc`、`AfterAgentHookFunc`、`ToolRecorderCallback`
- `trpc_build.go:148` 已通过 `buildToolCallbacks` + `WithToolCallbacks` 接入 Tool 回调
- **缺失**：`BeforeModelHookFunc`、`AfterModelHookFunc` 适配器；`BuildTRPCLLMAgent` 未注入 Agent/Model 回调；Plugin 回调未通过 Chain 注入

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 1.1 | `internal/agent/callbacks/adapter.go` | 新增 `BeforeModelHookFunc` 和 `AfterModelHookFunc`（参照已有的 `BeforeAgentHookFunc` 和 `AfterAgentHookFunc`） |
| 1.2 | `internal/plugin/trpc/chain_adapter.go`（新建） | 实现 `adaptPluginToChainEntries(p Plugin, priority int) []callbacks.Callback`，将 Plugin 回调包装为 Chain 条目（设计参考 §11.3 步骤 1） |
| 1.3 | `internal/agent/trpc_build.go` | 在 `BuildTRPCLLMAgent` 中构建 Chain：合并 Plugin 回调 + 现有 ToolRecorderCallback，注入 `WithAgentCallbacks(chain.AdaptAgentCallbacks())` 和 `WithModelCallbacks(chain.AdaptModelCallbacks())` 和 `WithToolCallbacks(chain.AdaptToolCallbacks())` |
| 1.4 | `internal/agent/trpc_build.go` | 删除原有 `buildToolCallbacks` 调用，将其逻辑迁移为 Chain 中的 `ToolRecorderCallback` |
| 1.5 | `internal/agent/trpc_build.go` | `TRPCBuilderDeps` 新增 `Plugins []trpcplugin.Plugin` 字段，从调用方传入 |

**验证**：
- `make wire && make build` 编译通过
- 单元测试：构造 LLMAgent 时传入 mock Chain，验证 BeforeAgent / AfterAgent / BeforeModel / AfterModel 回调被触发
- 集成测试：启用 audit_log 插件，发送对话请求，确认 AfterTool 回调仍正常工作

#### T2：Plugin 种子同步机制

**优先级**：P0 | **依赖**：无 | **设计参考**：§10

**代码现状**：
- `biz.PluginRepo` 仅有 `SearchPlugins` / `GetPlugin` / `UpdatePluginEnabled` / `UpdatePluginConfig` / `UpdateSortOrder`
- `biz.PluginUsecase` 仅有 `List` / `ToggleEnabled` / `UpdateConfig` / `UpdateSortOrder`
- `service/plugin.go` 有 `reloadRuntime()` 但无 `seedBuiltinPlugins()`
- **缺失**：`GetByKey`、`CreatePlugin` Repo 方法；`BuiltinPluginDef` 定义；种子同步逻辑

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 2.1 | `internal/plugin/trpc/registry.go`（新建） | 定义 `BuiltinPluginDef` 结构体和 `builtinPluginDefs()` 函数，返回 9 个内置插件定义（含 ConfigSchemaJSON 和 DefaultConfigJSON，参见设计文档 §10.3 和 §8.4） |
| 2.2 | `internal/biz/plugin.go` | `PluginRepo` 新增 `GetByKey(ctx, key)` 和 `CreatePlugin(ctx, Plugin)` 方法 |
| 2.3 | `internal/biz/plugin.go` | `PluginUsecase` 新增 `GetByKey` 和 `Create` 方法 |
| 2.4 | `internal/data/plugin.go` | 实现 `GetByKey`（按 `plugin_key` 查询）和 `CreatePlugin`（插入记录），参见设计文档 §10.6 |
| 2.5 | `internal/service/plugin.go` | 新增 `seedBuiltinPlugins(ctx)` 方法，遍历 `builtinPluginDefs()`，对 DB 中不存在的 key 执行 Create |
| 2.6 | `internal/service/plugin.go` | 在 `NewPluginService` 或 Wire 初始化后调用 `seedBuiltinPlugins` |
| 2.7 | `internal/plugin/trpc/adapter.go` | `builtin()` 函数添加所有 9 个内置插件的 case 分支（先返回占位结构体，具体回调逻辑在 Phase 2 实现） |

**验证**：
- `make wire && make build` 编译通过
- 启动服务后查询 `GET /v1/plugins`，确认 9 个内置插件记录存在
- 重启服务后确认不会重复创建（幂等）
- 启用一个插件后确认 `reloadRuntime()` 被触发

#### T3：配置 Schema 校验

**优先级**：P1 | **依赖**：T2（需要种子同步写入 config_schema_json） | **设计参考**：§14

**代码现状**：
- `PluginUsecase.UpdateConfig` 仅校验 `json.Valid()`，不校验 Schema
- `go.mod` 无 `gojsonschema` 依赖
- **缺失**：Schema 校验逻辑

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 3.1 | `go.mod` | 添加 `github.com/xeipuuv/gojsonschema` 依赖 |
| 3.2 | `internal/biz/plugin.go` | 新增 `validateJSONSchema(schemaJSON, docJSON string) error` 函数（参见设计文档 §14.3） |
| 3.3 | `internal/biz/plugin.go` | `UpdateConfig` 方法中，在 `json.Valid()` 之后增加 Schema 校验逻辑（参见设计文档 §14.2） |

**验证**：
- `make build` 编译通过
- 用合法配置调用 `PUT /v1/plugins/{id}/config`，返回成功
- 用不符合 Schema 的配置调用，返回 400 错误并说明具体字段
- Schema 为空或 `{}` 时不校验（向后兼容）

#### T4：统计更新机制

**优先级**：P1 | **依赖**：T2 | **设计参考**：§13

**代码现状**：
- `plugins` 表有 `invoke_count` / `block_count` / `error_count` / `last_invoked_at` / `last_status` 字段
- `AuditLogPlugin` 回调中无统计更新逻辑
- **缺失**：`StatsUpdater` 接口、`PluginStatUpdate` 结构体、`IncrementStats` Repo 方法

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 4.1 | `internal/biz/plugin.go` | 新增 `PluginStatUpdate` 结构体和 `StatsUpdater` 接口（参见设计文档 §13.3） |
| 4.2 | `internal/biz/plugin.go` | `PluginRepo` 新增 `IncrementStats(ctx, key, delta)` 方法 |
| 4.3 | `internal/data/plugin.go` | 实现 `IncrementStats`：原子递增 invoke_count / block_count / error_count，更新 last_invoked_at / last_status（参见设计文档 §10.6） |
| 4.4 | `internal/plugin/trpc/stats.go`（新建） | 实现 `StatsUpdater` 接口，内部调用 `PluginRepo.IncrementStats` |
| 4.5 | `internal/plugin/trpc/adapter.go` | `builtin()` 签名增加 `stats StatsUpdater` 参数，各插件构造函数注入 |
| 4.6 | `internal/plugin/trpc/audit.go` | AuditLogPlugin 增加 `stats StatsUpdater` 字段，AfterTool 回调中异步更新统计 |
| 4.7 | `internal/plugin/trpc/runtime.go` | `Apply()` 调用 `adapt()` 时传入 `StatsUpdater` |

**验证**：
- `make wire && make build` 编译通过
- 启用 audit_log 插件，发送对话请求触发工具调用
- 查询 `GET /v1/plugins`，确认 audit_log 的 invoke_count 递增

---

### Phase 2：内置插件实现

> **前置依赖**：T1（EP-CB-01）完成后，BeforeModel / AfterModel / BeforeAgent / AfterAgent 回调点才可触发。BeforeTool / AfterTool / OnEvent 已可用。

#### T5：实现 runtime_audit 内置插件

**优先级**：P1 | **依赖**：T1, T2 | **设计参考**：§8.2, §8.4

**代码现状**：
- `internal/plugin/trpc/audit.go` 已有 `AuditLogPlugin`，仅注册 `AfterTool` 回调
- T2 种子同步后 `adapter.go` 已有 `case "runtime_audit":` 占位
- **需替换**：将 `AuditLogPlugin` 重构为 `RuntimeAuditPlugin`，注册全部 7 个回调点

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 5.1 | `internal/plugin/trpc/runtime_audit.go`（新建） | 实现 `RuntimeAuditPlugin`，注册 BeforeAgent / AfterAgent / BeforeModel / AfterModel / BeforeTool / AfterTool / OnEvent 回调 |
| 5.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 将 `case "audit_log", "audit-log", "auditlog":` 映射到 `RuntimeAuditPlugin`，新增 `case "runtime_audit":` |
| 5.3 | `internal/plugin/trpc/audit.go` | 保留文件但将 `AuditLogPlugin` 标记为 deprecated，或删除并统一使用 `RuntimeAuditPlugin` |
| 5.4 | `internal/plugin/trpc/registry.go` | 确认 `builtinPluginDefs()` 中 runtime_audit 定义完整（T2 已添加） |

**回调实现要点**：
- BeforeAgent：记录 Agent 名称、SessionID、开始时间
- AfterAgent：记录执行耗时、是否出错
- BeforeModel：按配置 `log_model_request` 记录请求摘要（截断到 `max_content_length`）
- AfterModel：按配置 `log_model_response` 记录响应摘要 + token 用量
- BeforeTool：按配置 `log_tool_args` 记录工具名和参数摘要
- AfterTool：记录工具执行结果状态
- OnEvent：记录事件类型
- 所有日志按配置 `redact_sensitive` 脱敏

**验证**：
- 启用 runtime_audit 插件，发送对话请求
- 检查 slog 输出包含 before_agent / after_agent / before_model / after_model / before_tool / after_tool 日志
- 修改 `max_content_length` 配置，确认日志截断生效

#### T6：实现 sensitive_data_mask 内置插件

**优先级**：P1 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 6.1 | `internal/plugin/trpc/sensitive_mask.go`（新建） | 实现 `SensitiveDataMaskPlugin`，注册 BeforeModel / AfterModel 回调 |
| 6.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "sensitive_data_mask":` |

**回调实现要点**：
- BeforeModel：扫描 `args.Request` 中的内容，按配置脱敏邮箱（`mask_email`）、手机号（`mask_phone`）、密钥（`mask_secret`）、自定义模式（`custom_patterns`），修改 `args.Request` 返回
- AfterModel：扫描模型响应，检测密钥泄漏；`block_leak_output=true` 时返回 CustomResponse 阻断

**验证**：
- 输入含 `sk-abc123`、`user@example.com`、`13800138000` 的对话
- 启用插件后，模型请求中敏感数据被脱敏
- 模型输出含密钥时被阻断或脱敏

#### T7：实现 confirmation_guard 内置插件

**优先级**：P1 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 7.1 | `internal/plugin/trpc/confirmation_guard.go`（新建） | 实现 `ConfirmationGuardPlugin`，注册 BeforeTool 回调 |
| 7.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "confirmation_guard":` |

**回调实现要点**：
- BeforeTool：检查工具名是否在 `confirm_tools` 列表或参数命中 `confirm_patterns`
- 命中时：本期无人类确认通道，按 `default_action` 决定（默认 `reject`），返回 CustomResult 阻断
- 超时逻辑本期不实现（需人类确认通道）

**验证**：
- 配置 `confirm_tools: ["delete_file"]`
- 调用 `delete_file` 工具时被阻断，返回拒绝原因
- 调用其他工具时正常通过

#### T8：实现 cost_guard 内置插件

**优先级**：P2 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 8.1 | `internal/plugin/trpc/cost_guard.go`（新建） | 实现 `CostGuardPlugin`，注册 BeforeModel 回调 |
| 8.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "cost_guard":` |

**回调实现要点**：
- BeforeModel：检查 `blocked_models` 列表，命中时返回 CustomResponse 拒绝
- 检查 `max_prompt_tokens`，超限时拒绝或降级到 `fallback_model`
- `daily_token_budget` 需要持久化计数器，本期可用内存 map + 每日重置实现

**验证**：
- 配置 `blocked_models: ["gpt-4"]`
- 使用 gpt-4 模型时请求被拒绝
- 配置 `fallback_model: "gpt-3.5-turbo"`，超限时自动降级

#### T9：实现 model_router 内置插件

**优先级**：P2 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 9.1 | `internal/plugin/trpc/model_router.go`（新建） | 实现 `ModelRouterPlugin`，注册 BeforeModel 回调 |
| 9.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "model_router":` |

**回调实现要点**：
- BeforeModel：根据 `rules` 匹配当前请求特征（Agent key、prompt 内容、上下文长度）
- 匹配到规则时修改 `args.Request.Model` 为目标模型
- 未匹配时使用 `default_model`
- 只改写 Model 名称，Provider / API Key / base URL 不变

**验证**：
- 配置 `code_model: "gpt-4"` + 规则匹配代码任务
- 代码相关请求被路由到 gpt-4
- 普通请求使用 default_model

#### T10：实现 permission_guard 内置插件

**优先级**：P2 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 10.1 | `internal/plugin/trpc/permission_guard.go`（新建） | 实现 `PermissionGuardPlugin`，注册 BeforeTool 回调 |
| 10.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "permission_guard":` |

**回调实现要点**：
- BeforeTool：检查工具名是否在 `deny_tools` 列表，命中时返回 CustomResult 拒绝
- 检查 `agent_allowlist`，当前 Agent 不在列表时跳过
- `confirm_tools` 列表本期按 reject 处理（同 confirmation_guard）

**验证**：
- 配置 `deny_tools: ["execute_sql"]`
- 调用 execute_sql 时被拒绝
- 其他工具正常通过

#### T11：实现 output_policy 内置插件

**优先级**：P2 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 11.1 | `internal/plugin/trpc/output_policy.go`（新建） | 实现 `OutputPolicyPlugin`，注册 AfterModel / OnEvent 回调 |
| 11.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "output_policy":` |

**回调实现要点**：
- AfterModel：扫描模型输出，匹配 `blocked_patterns` 和危险命令检测
- 命中时：`block_on_violation=true` 返回 CustomResponse 阻断，内容为 `replacement_message`
- OnEvent：对事件流中的文本内容同样检查

**验证**：
- 配置 `blocked_patterns: ["rm -rf"]`
- 模型输出含 `rm -rf` 时被阻断
- `block_on_violation=false` 时仅记录不阻断

#### T12：实现 skill_usage_tracker 内置插件

**优先级**：P2 | **依赖**：T1, T4 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 12.1 | `internal/plugin/trpc/skill_tracker.go`（新建） | 实现 `SkillUsageTrackerPlugin`，注册 BeforeTool / AfterTool 回调 |
| 12.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "skill_usage_tracker":` |

**回调实现要点**：
- BeforeTool：按配置 `capture_input_preview` 记录工具名和参数摘要
- AfterTool：记录执行结果、耗时、是否出错；按配置 `capture_output_preview` 记录输出摘要
- 摘要截断到 `max_preview_length`
- 第一阶段仅 slog 输出，后续接入持久化

**验证**：
- 启用插件，触发 Skill 工具调用
- slog 输出包含工具名、参数摘要、结果摘要

#### T13：实现 retry_and_reflect 内置插件

**优先级**：P2 | **依赖**：T1 | **设计参考**：§8.4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 13.1 | `internal/plugin/trpc/retry_reflect.go`（新建） | 实现 `RetryAndReflectPlugin`，注册 AfterTool 回调 |
| 13.2 | `internal/plugin/trpc/adapter.go` | `builtin()` 添加 `case "retry_and_reflect":` |

**回调实现要点**：
- AfterTool：检查 `args.Error != nil` 判断工具失败
- 失败时：检查 `excluded_tools` 排除列表和 `high_risk_tools_need_confirm`
- 未排除时：生成反思提示注入上下文，请求模型重试
- 使用 invocation scope 计数器跟踪重试次数，超过 `max_retries` 时按 `error_if_retry_exceeded` 处理
- **本期限制**：无法直接重试工具调用，只能在事件流中注入反思提示，依赖模型自行重试

**验证**：
- 启用插件，触发工具失败场景
- 事件流中出现反思提示
- 超过 max_retries 后停止重试

---

### Phase 3：Agent 绑定 + 运行记录

#### T14：Agent 绑定（scope 过滤）

**优先级**：P2 | **依赖**：T2 | **设计参考**：§15

**代码现状**：
- `plugins` 表有 `scope` 字段（默认 `"global"`）
- `plugintrpc.Runtime` 的 `active` 为 `[]trpcplugin.Plugin`，不含 scope 信息
- `service/trpc_turn.go:67` 和 `team/runner_team_trpc.go:123` 调用 `pluginRT.Plugins()` 获取全部插件
- **缺失**：`ScopedPlugin` 结构体、`PluginsForAgent` 方法、`UpdatePluginScope` API

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 14.1 | `internal/plugin/trpc/runtime.go` | 新增 `ScopedPlugin` 结构体，`Apply()` 保存 scope 信息，新增 `PluginsForAgent(agentID)` 方法（参见设计文档 §15.2） |
| 14.2 | `internal/service/trpc_turn.go` | 调用 `PluginsForAgent(agentID)` 替换 `Plugins()` |
| 14.3 | `internal/team/runner_team_trpc.go` | 同上 |
| 14.4 | `api/kratos/plugin/v1/plugin.proto` | 新增 `UpdatePluginScope` RPC（参见设计文档 §2.2） |
| 14.5 | `internal/biz/plugin.go` | `PluginRepo` 新增 `UpdateScope` 方法，`PluginUsecase` 新增 `UpdateScope` 方法 |
| 14.6 | `internal/data/plugin.go` | 实现 `UpdateScope`（参见设计文档 §10.6） |
| 14.7 | `internal/service/plugin.go` | 实现 `UpdatePluginScope` RPC + 热重载（参见设计文档 §5.4） |
| 14.8 | `web/src/services/plugin.ts` | 新增 `updatePluginScope` API 调用（参见设计文档 §9.1） |
| 14.9 | `web/src/pages/PluginsPage.vue` | 详情抽屉「Agent 绑定」Tab 实现（参见需求文档 §8.3） |

**验证**：
- 将插件 scope 设为指定 agent_id
- 该 Agent 对话时插件生效
- 其他 Agent 对话时插件不生效

#### T15：Plugin 运行记录

**优先级**：P2 | **依赖**：T4 | **设计参考**：需求 §4

| 步骤 | 文件 | 改动说明 |
|------|------|----------|
| 15.1 | `internal/data/ent/schema/plugin_run.go`（新建） | PluginRun Ent Schema |
| 15.2 | `api/kratos/plugin/v1/plugin.proto` | 新增 `ListPluginRuns` RPC |
| 15.3 | `internal/biz/plugin.go` | 新增 `PluginRunRepo` 接口和 Usecase |
| 15.4 | `internal/data/plugin_run.go`（新建） | 实现 `PluginRunRepo` |
| 15.5 | `internal/service/plugin.go` | 实现 `ListPluginRuns` RPC |
| 15.6 | `internal/plugin/trpc/stats.go` | 回调执行后写入 PluginRun 记录 |
| 15.7 | 前端 | `/plugins/runs` 页面 |

---

### Phase 4：进阶能力

#### T16：Plugin 进程隔离方案设计

**优先级**：P3 | **依赖**：Phase 2 完成

设计文档，评估 gRPC / MCP / WASM 方案。

#### T17：Plugin 版本表 + 回滚 API

**优先级**：P3 | **依赖**：Phase 2 完成

新增 `plugin_versions` 表，记录每次配置变更，支持回滚。

---

## 6. 验收标准

### Phase 1 验收

- [x] Plugin CRUD 端到端可用（List / Toggle / Config / SortOrder）
- [x] Plugin 热重载：写操作后 Runner 自动获取最新插件列表
- [x] 前端管理页可用
- [ ] 服务启动后 DB 中存在 9 个内置插件记录（种子同步）
- [ ] BeforeAgent / AfterAgent / BeforeModel / AfterModel 回调可触发（EP-CB-01）
- [ ] 配置不符合 Schema 时返回 400 错误
- [ ] 插件回调执行后 invoke_count / error_count 递增

### Phase 2 验收

- [ ] 9 个内置插件全部实现并可注册到 Runner
- [ ] runtime_audit 记录完整执行链路日志
- [ ] sensitive_data_mask 脱敏邮箱/手机号/密钥
- [ ] confirmation_guard 阻断高风险工具
- [ ] cost_guard 拒绝/降级超预算请求
- [ ] model_router 根据规则路由模型
- [ ] permission_guard 拒绝禁止的工具
- [ ] output_policy 阻断违规输出
- [ ] skill_usage_tracker 记录 Skill 调用统计
- [ ] retry_and_reflect 工具失败时注入反思提示

### Phase 3 验收

- [ ] Plugin 按 Agent 维度生效（scope 过滤）
- [x] Plugin 运行记录可查询（`ListPluginRuns`）

### Phase 4 验收

- [ ] Plugin 运行在隔离进程中
- [ ] Plugin 可管理版本并回滚

---

## 7. 依赖关系

```
T1 (EP-CB-01) ─────────────────────────┬── T5 (runtime_audit)
                                        ├── T6 (sensitive_data_mask)
                                        ├── T7 (confirmation_guard)
                                        ├── T8 (cost_guard)
                                        ├── T9 (model_router)
                                        ├── T10 (permission_guard)
                                        ├── T11 (output_policy)
                                        ├── T12 (skill_usage_tracker) ── T4 (stats)
                                        └── T13 (retry_and_reflect)

T2 (种子同步) ── T3 (Schema 校验)
             ── T4 (统计更新)
             ── T5~T13 (内置插件需要 DB 记录)
             ── T14 (Agent 绑定)

T4 (统计更新) ── T12 (skill_usage_tracker)
             ── T15 (运行记录)
```

**关键路径**：T2 → T1 → T5~T13

**建议执行顺序**：
1. T2（种子同步）— 先让 DB 有数据
2. T1（EP-CB-01）— 让回调链路通
3. T3（Schema 校验）— 独立，可并行
4. T4（统计更新）— 独立，可并行
5. T5~T13（内置插件）— 按 P1/P2 优先级逐个实现

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| EP-CB-01 改动面大，可能影响现有 Tool 回调 | 高 | 先写集成测试验证现有 Tool 回调不受影响，再添加 Agent/Model 回调 |
| 内置插件回调逻辑复杂，可能阻塞执行链 | 中 | 所有回调内部逻辑异步化（safego.Go），回调本身快速返回 |
| retry_and_reflect 无法直接重试工具调用 | 中 | 本期仅注入反思提示，依赖模型自行重试；后续考虑框架级重试 API |
| 统计更新频繁写入 DB | 低 | 使用 safego.Go 异步更新 + 批量合并（可选） |
| 种子同步在大量插件时启动变慢 | 低 | 仅启动时执行一次，且只 INSERT 不存在的记录 |
