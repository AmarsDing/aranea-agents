# Callback 回调模块 — 实现设计文档

> 对应需求：[28 callback.md](./28%20callback.md)  
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)  
> 关联模块：[22-plugin.design.md](./22-plugin.design.md)（内置 Plugin、Runner 注入）

---

## 一、模块概述

全链路回调：Agent / Model / Tool 生命周期拦截与增强，以及事件 OnEvent。产品层通过 **Callback Chain**（`internal/agent/callbacks`）桥接 Hook 规则与 trpc-agent-go 原生 Callback；DB Plugin 经 **Runner WithPlugins** 注入，二者边界见 §四。

---

## 二、架构总览

```
┌─────────────────────────────────────────────────────────┐
│                     产品层                               │
│  Hook CRUD (hooks) ──→ biz.HookResolver                  │
│  Plugin CRUD (plugins) ──→ plugintrpc.Runtime.Apply()    │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  plugintrpc.Manager                                        │
│  ├─ MergeChain(agent)     ← Hook → Chain 条目            │
│  ├─ RunnerPluginsForAgent ← DB Plugin + productEventPlugin│
│  └─ OnEvent               ← 作用域过滤后转发框架 Plugin   │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  agent.buildCallbackChainOptions (callback_chain.go)       │
│  productCallbackChain + Manager.MergeChain               │
│  → AdaptAgent/Model/ToolCallbacks → llmagent.With*       │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  trpc-agent-go                                             │
│  agent/model/tool.Callbacks · plugin.Plugin · Runner     │
└─────────────────────────────────────────────────────────┘
```

**ModelSelector 旁路**（不在 Chain 重复 BeforeModel 路由）：`model_router`、`cost_guard` 经 `agent.PluginModelSelector` / `PluginCostGuardSelector` 在 `trpc_build.go` 装配。

---

## 三、编排边界（三层 + 框架 Plugin）

权威注释：`internal/plugin/trpc/manager.go` 头部（1–17 行）；编排策略见 `orchestration_policy.go`。

| 层 | 职责 | 排序依据 |
|----|------|----------|
| **1. Runner WithPlugins** | DB 内置 Plugin（audit_log、guards、skill_tracker 等）+ 框架 Plugin（identity、guardrail、toolcallid、messagemerger） | DB Plugin `sort_order` ASC → 框架 Plugin 追加 |
| **2. LLMAgent Callback Chain** | 产品固定链 + Hook 规则 | 固定优先级 + Hook `300+sort_order` |
| **3. ModelSelector** | 仅 catalog 级模型切换（router / cost_guard blocked_models） | Agent 构造时单次选择 |

**框架 Plugin 自动注入**：`Manager.RunnerPluginsForAgent` 在 DB Plugin 之后追加 `productEventPlugin`（OnEvent 桥接）、`identity`（身份透传）、`guardrail`（PromptInjection + UnsafeIntent 检测）、`tool_call_id`（ToolCall ID 规范化）、`consecutive_message_merger`（消息合并）。这些 Plugin 无需 DB 配置，始终生效。

**工具确认**：`confirmation_guard` Runner Plugin 直接通过 BeforeTool `CustomResult` 阻断高风险工具，不再依赖 Chain ConfirmGate；`permission_guard` 处理 `deny_tools` 阻断。

**编排统一**：所有 DB Plugin 统一走 Runner `WithPlugins` 路径，不再支持 `callback_orchestration:"chain"` 镜像。`plugin_chain_mirror.go` 已移除。

### 3.1 System Message 注入顺序（前缀稳定化）

> 落地 2026-08-11（P1）；P2 深化（per-turn 动态 cue 末尾追加 + intent 搬移）同日落地。

DeepSeek prompt caching 从 token 0 开始匹配：system-message 前缀内任何 per-turn 变化会使整个缓存块失效。两档契约：

1. **会话级稳定 cue**（static/semi-static）：**禁止 prepend 到 position 0**，必须使用 `insertAfterLastSystem` 追加到已有 system 块之后；
2. **per-turn 动态 cue**（P2 深化）：仅「插在 system 块后」不够——history 每轮增长时动态 cue 的位置随之前的位置关系仍会把可缓存前缀截断在 cue 处，长会话命中率随轮次衰减。因此动态 cue 一律 **append 到消息列表末尾**（user/history 之后），使 `[system 块 + history + user]` 成为单调增长的可缓存前缀，只有尾部动态段每轮重算。

**三层前缀设计**（按 Hook Layer 排序保证注入顺序）：

| 层 | Hook Layer | 内容 | 注入位置 |
|----|-----------|------|---------|
| Static | LayerStatic | 会话级稳定（不随 turn 变化）：基础 system prompt、静态运行时能力 cue | `insertAfterLastSystem` |
| Semi-Static | LayerSemiStatic | 任务/turn 级变化：动态运行时 cue、skill guidance | `insertAfterLastSystem` |
| Dynamic | LayerDynamic | 每 turn 变化：memory cue（含 recall 结果）、knowledge cue、reply reminder、intent context | **消息列表末尾 append** |

**最终消息结构**：`[base system, static cue, semi-static cue..., history..., user, dynamic cue...（末尾）]`

**已实现注入点**：

| 文件 | 钩子 | Layer | 位置 |
|------|------|-------|------|
| `runtime_cue_inject.go` | 静态运行时 cue | Static | insertAfterLastSystem |
| `runtime_cue_inject.go` | 动态运行时 cue | SemiStatic | insertAfterLastSystem |
| `skill_guidance_inject.go` | 完整 skill 指引 | SemiStatic | insertAfterLastSystem |
| `skill_guidance_inject.go` | 渐进式 skill 指引 | SemiStatic | insertAfterLastSystem |
| `memory_inject.go` | 记忆注入（recall + profile card） | Dynamic | **末尾 append** |
| `memory_inject.go` | 压缩后记忆重建 | Dynamic | 原位替换；无既有 cue 时**末尾 append** |
| `knowledge_inject.go` | 知识库 cue | Dynamic | **末尾 append** |
| `reply_reminder_inject.go` | 回复提醒 | Dynamic | **末尾 append** |
| `intent_reorder_inject.go` | intent context 搬移 | Dynamic（priority 100） | 稳定分区搬移到**末尾** |

**intent 搬移钩子**（P2）：框架 content processor 把 `RunOptions.InjectedContextMessages`（intent JSON）追加在 system 块之后、session history 之前，注入点无法控制位置；`newIntentReorderBeforeHook` 以 LayerDynamic + priority 100（晚于所有消息改写钩子）把 intent 系统消息稳定分区搬移到末尾。每次模型调用（含工具循环重入）请求都重新构建，搬移幂等不累积；无 intent 时快速路径零分配。

**测试契约**：`prompt_prefix_position_test.go`——会话级稳定 cue 用 `assertCueAfterBase` pin 住（base system index 0、cue 其后、user 跟随）；per-turn 动态 cue 用 `assertCueAtEnd` pin 住（base system index 0、user 保持原位、动态 cue 在末尾）；intent 搬移 4 个测试（移至末尾/落在其他动态 cue 之后/无 intent 不动/已在末尾幂等）。

---

## 四、现有代码资产

### 4.1 Chain 抽象

**位置**：`internal/agent/callbacks/`

| 类型 | 说明 |
|------|------|
| `CallbackPoint` | BeforeAgent … AfterTool、OnError（OnEvent 通过 productEventPlugin 桥接，非 Chain 枚举） |
| `Callback` + `*Hook` 接口 | 按点实现 `Handle*` |
| `Chain` | 优先级稳定排序；`Adapt*Callbacks()` 转框架类型 |
| `adapter.go` | `ToolRecorderCallback`、`Before*HookFunc` 等函数适配器 |

### 4.2 产品链装配

**位置**：`internal/agent/callback_chain.go`、`product_chain_builtins.go`

| 组件 | 说明 |
|------|------|
| `buildCallbackChainOptions` | 按链内容注入 `WithAgent/Model/ToolCallbacks` |
| `productCallbackChain` | 指标遥测 + 工具链（loop-guard/cache/timing/confirm/recorder/circuit-breaker/output-limiter 等） |
| `productCallbackChainWithRegistry` | 返回 Chain + CircuitBreakerRegistry |

**工具循环守卫**（`internal/agent/tool_loop_guard.go`，2026-08-16 / 并行窗口 2026-08-17）：

- 串行：同签名+同成功结果连续 2 次放行，第 3 次起拦截（error 回灌模型）；节点内拦截满 5 次升级 `StopError`。
- 并行：同一轮相同签名只放行第一次（`inflight`）；其余立即拦截。并行拦截**不计入**饱和次数，避免首轮扇出直接终止节点。
- 隔离键：`InvocationID|AgentName`，图谱节点互不连坐。

### 4.3 PluginManager

**位置**：`internal/plugin/trpc/manager.go`

| 方法 | 说明 |
|------|------|
| `MergeChain` | HookResolver → `HookCallbacks` + `wrapResilientHooks` |
| `RunnerPluginsForAgent` | 作用域过滤 Plugin + `productEventPlugin` + 框架 Plugin（identity/guardrail/toolcallid/messagemerger） |
| `ReloadHooks` | CRUD 后刷新 Hook 快照 |

### 4.4 Hook 解析

**位置**：`internal/biz/hook/hook.go`（Hook 模型、Config、Resolver、Usecase、Delivery 实现）；`internal/biz/hook.go`（re-export 至 `biz` 包）；执行 `internal/plugin/trpc/hook_callbacks.go`、`hook_modify.go`、`hook_events.go`

`config_json` 结构：

```json
{
  "callback_point": "before_model",
  "condition": { "agent_id": "...", "tool_name": "..." },
  "action": {
    "type": "log|notify|block|modify",
    "webhook_url": "...",
    "modify_patch": {},
    "log_level": "info"
  }
}
```

### 4.5 数据模型与 Schema

#### Ent Schema（核心表）

| 表 | Ent Schema | 关键字段 |
|----|------------|----------|
| `hooks` | `internal/data/ent/schema/hook.go`（`PlatformHook`，`entsql.Annotation{Table: "hooks"}`） | `id`、`hook_key`、`name`、`status`、`enabled`、`sort_order`、`config_json`、`metadata_json` |
| `plugins` | `internal/data/ent/schema/plugin.go`（`PlatformPlugin`，`entsql.Annotation{Table: "plugins"}`） | `id`、`plugin_key`、`scope`、`callback_points_json`、`config_json`、`fallback_config_json`（StorageKey `default_config_json`）、`sort_order`、`enabled` |

#### DDL 迁移表（非 Ent Schema）

| 表 | DDL 位置 | 说明 |
|----|----------|------|
| `hook_deliveries` | `internal/data/sql/hook_delivery.sql` + 迁移 `20260622_hook_delivery_schema.sql` | Hook notify 投递队列，含 `status`、`attempt_count`、`max_attempts`、`idempotency_key`；索引 `idx_hook_deliveries_retry`（status=pending + updated_at）支撑重试 worker |

> `hook_deliveries` 不进 Ent Schema，通过 DDL Migration Registry 管理（符合 DB-R3：FTS5/队列等通过 DDL 迁移补充但不另建 Ent Schema）。Repo 实现见 `internal/data/hook_delivery.go`（Raw SQL，走 `RWDB()` 事务感知路径）。

### 4.6 Hook `modify` 合并策略（before_tool）

| 字段 | 行为 |
|------|------|
| `modify_patch.arguments` | 整包替换工具参数 JSON（优先级最高） |
| `modify_patch.merge_arguments` | 与当前参数 **深度合并**；嵌套 `map` 递归合并；标量与数组由 patch 覆盖 |
| 实现 | `internal/plugin/trpc/tool_argument_merge.go` |

### 4.7 审计查询（plugin_runs）

| 来源 | `plugin_key` | 落库条件 |
|------|----------------|----------|
| 内置 Plugin | `audit_log` 等 | `blocked` / `error` |
| Hook 规则 | `hook:<hook_key>` | `blocked` / `error`（`hook_audit.go`） |

管理端 `/plugins/runs`：按 lifecycle point、agent_id、status、时间范围筛选；`plugin_key=hook:` 前缀匹配所有 Hook 记录。

### 4.8 Plugin 编排路径（统一 Runner）

所有 DB 内置 Plugin 统一走 Runner `WithPlugins` 路径，`orchestration_policy.go` 中 `ResolvePluginOrchestration` 始终返回 `OrchestrationRunner`，Chain 镜像路径已废弃。

内置插件（`audit_log`、`model_router`、`cost_guard` 等）与声明 `on_event` 的插件**强制 runner**，防止双触发与 OnEvent 丢失。

`plugin_chain_mirror.go` 已移除；`callbacks.PluginCallback` 接口保留供未来 Chain 镜像预留。`skill_usage_tracker` 等原白名单插件也统一走 Runner 路径。

Hook 规则使用 **`wrapResilientHooks`**（非 block 错误不中断回合）。

### 4.9 Hook notify 投递

表 `hook_deliveries`（`internal/data/sql/hook_delivery.sql`）：`pending` → 重试 → `success`/`failed`。

`HookAction.notify_max_retries`（默认 3）、`notify_timeout_sec`（默认 8）。

- **指标**：入队成功记 `PluginInvokeTotal{status=queued}`；最终投递失败记 `delivery_failed`。
- **SSRF**：`pkg/webhookurl.ValidateNotifyURL` — 仅 http/https、禁止 loopback/私网/metadata 解析结果。
- **管理 API**：`GET /v1/hooks/deliveries`（`ListHookDeliveries`）；前端 `/hooks/deliveries`。

---

## 五、运行时注入

### 5.1 Agent 构造

```
BuildTRPCLLMAgent(deps)
  ├─ ModelSelector（router / cost_guard）
  ├─ buildCallbackChainOptions → With*Callbacks
  └─ ConfirmGate 配置来自 PluginManager.ConfirmationGuardConfigForAgent
```

### 5.2 Runner 构造

```
NewTRPCRunner
  └─ WithPlugins(PluginManager.RunnerPluginsForAgent(...))
```

### 5.3 事件流

```
Run 事件 → productEventPlugin.OnEvent → Manager.OnEvent → 各 DB Plugin
Hook on_event 规则经 HookResolver + event 桥接（非 Chain 条目）
```

---

## 六、Wire 依赖

| 组件 | 注入点 |
|------|--------|
| `biz.NewHookResolver` | `HookUsecase` |
| `plugintrpc.NewManager` | `Runtime` + `HookResolver` |
| `TRPCBuilderDeps.PluginManager` | `internal/agent/builder_deps.go` |

**Runtime.Close 清理范围**（2026-05-28）：`HookRetryWorker.Stop()` → `StatsRecorder.Close()`（batch worker drain） → `CostGuardBudgetRegistry.Reset()`（清空 byScope map）。

**并发安全**（2026-05-28）：`Manager.resolveAgentID` 读写加 `resolveMu sync.RWMutex` 保护；`StatsRecorder.resolveAgent` 读写加 `resolveMu sync.RWMutex` 保护；`ModelRouterRule.compiled()` 惰性编译加 `compileOnce sync.Once` 保护；`CostGuardBudgetRegistry.TrackerForScope` 读路径用 `RLock` 优化。

**日志规范**（2026-05-28）：`PluginSafeLogger` 统一使用 `event.SysLog*`（符合红线 16），不再直接写 `os.Stderr`；Hook 事件 key 按场景区分：`system.hook.reload_fail`（仅规则加载）、`system.hook.delivery_fail`（交付失败）、`system.hook.delivery_retry`（重试）。

**goroutine 安全**（2026-05-28）：`StatsRecorder.worker()` 改用 `safego.Go` 启动（符合红线 13），防止 panic 导致进程崩溃。

---

## 七、涉及文件（维护索引）

| 文件 | 职责 |
|------|------|
| `internal/agent/callbacks/*.go` | Chain 类型与适配 |
| `internal/agent/callback_chain.go` | 链装配入口 |
| `internal/agent/product_chain_builtins.go` | 产品固定链（指标等） |
| `internal/agent/tool_*.go` | 工具确认、缓存、计时、记录、熔断、命令安全、循环守卫、结果兜底截断 |
| `internal/plugin/trpc/manager.go` | 聚合 Hook + Plugin（含编排边界注释） |
| `internal/plugin/trpc/hook_*.go` | Hook 动作执行（callbacks/modify/notify/audit/events/resilience/retry_worker） |
| `internal/plugin/trpc/orchestration_policy.go` | 编排策略（统一 Runner） |
| `internal/plugin/trpc/runtime.go` | 内置 Plugin 注册与生命周期 |
| `internal/plugin/trpc/chain_adapter.go` | 内置 Plugin 回调点声明（`BuiltinCallbackPoints`） |
| `internal/biz/hook/hook.go` | Hook 领域实现（Config/Resolver/Usecase/Delivery） |
| `internal/biz/hook.go` | Hook 领域 re-export 至 `biz` 包 |
| `internal/service/hook.go` | HookService gRPC/HTTP 实现 |
| `internal/data/hook.go` | hooks 表 Repo |
| `internal/data/hook_delivery.go` | hook_deliveries 表 Repo（Raw SQL） |
| `internal/data/sql/hook_delivery.sql` | hook_deliveries 表 DDL |

---

## 八、Web 前端

| 组件 | 路由 / 用途 |
|------|-------------|
| `HooksPage.vue` | `/hooks` 全局 Hook CRUD |
| `HookDeliveriesPage.vue` | `/hooks/deliveries` Hook 投递记录查看 |
| `PluginRunsPage.vue` | `/plugins/runs` Plugin/Callback 运行记录（含 `hook:` 前缀筛选） |
| `AgentHooksPanel.vue` | Agent 设置内嵌作用域 Hook：作用域规则 CRUD（名称/启停/排序/删除）+ 生效中全局规则只读分组；`condition.agent_id` 锁定当前 Agent |
| `CallbackEditor.vue` | `config_json` 可视化编辑（log/notify/block/modify）；支持 `lock-agent-id`、`tool_name` 下拉（注入工具目录时，未注入回退手输）、`event_type` 下拉（`HOOK_EVENT_TYPE_VALUES`） |
| `HooksTable.vue` | Hook 规则列表（page / agent / readonly 三种模式） |

共享常量：`web/src/features/callback/constants.ts`（`CALLBACK_POINT_VALUES`、`useCallbackPointOptions`、`callbackPointLabel`、`HOOK_EVENT_TYPE_VALUES`、`PLUGIN_RUN_KEY_PRESETS`）。

API：`HookService` gRPC/HTTP；Agent 设置页复用同一编辑器组件。

---

## 九、Proto / API 契约

### 9.1 HookService（`api/kratos/hook/v1/hook.proto`）

| RPC | HTTP | 请求 | 响应 | 说明 |
|-----|------|------|------|------|
| `ListHooks` | `GET /v1/hooks` | `google.protobuf.Empty` | `ListHooksResponse{items[]}` | 列出全部 Hook |
| `CreateHook` | `POST /v1/hooks` | `CreateHookRequest`（key/name 必填） | `Hook` | 创建 Hook，成功后触发 `ReloadHooks` |
| `GetHook` | `GET /v1/hooks/{id}` | `GetHookRequest{id}` | `Hook` | 查询单个 Hook |
| `UpdateHook` | `PATCH /v1/hooks/{id}` | `UpdateHookRequest{id, hook}` | `Hook` | 更新 Hook，成功后触发 `ReloadHooks` |
| `DeleteHook` | `DELETE /v1/hooks/{id}` | `DeleteHookRequest{id}` | `google.protobuf.Empty` | 删除 Hook，成功后触发 `ReloadHooks` |
| `ListHookDeliveries` | `GET /v1/hooks/deliveries` | `ListHookDeliveriesRequest{hook_key,status,from,to,page,page_size}` | `ListHookDeliveriesResponse{items[],total,page,page_size}` | Hook notify 投递记录分页查询 |

### 9.2 消息字段契约

**`Hook`**：`id`、`key`、`name`、`description`、`status`、`enabled`、`sort_order`、`config_json`、`metadata_json`、`created_at`、`updated_at`、`deleted_at`

**`HookDelivery`**：`id`、`hook_key`、`hook_id`、`webhook_url`、`payload_json`、`status`（pending/success/failed）、`attempt_count`、`max_attempts`、`last_error`、`created_at`、`updated_at`、`idempotency_key`

### 9.3 Service 实现

**位置**：`internal/service/hook.go`（`HookService`）

- 注入 `biz.HookUsecase` + `biz.HookDeliveryUsecase` + `plugintrpc.Manager` + `loggateway.Logger`
- 写操作（Create/Update/Delete）成功后通过 `safego.Go` 异步调用 `mgr.ReloadHooks` 热更新
- 错误翻译：`apierror.IsCode(err, CodeNotFound)` → `apierror.NotFound("HOOK", ...)`
- 分页：复用 `biz.PageToLimitOffset`

### 9.4 Plugin 运行记录查询

`ListPluginRuns`（`api/kratos/plugin/v1/plugin.proto`，`internal/service/plugin.go`）支撑 `/plugins/runs` 审计查询，按 `plugin_key` 前缀 `hook:` 匹配所有 Hook 审计记录。
