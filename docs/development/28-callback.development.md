# Callback 回调 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Phase 1–3 已落地
> **需求**：[28 callback.md](./28%20callback.md) · **设计**：[28 callback.design.md](./28%20callback.design.md)
> **进度**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-CB-01 ✅
> **变更**：[DocSync](../changelog/2026-05-21-Callback-DocSync.md) · [P2](../changelog/2026-05-21-Callback-P2.md) · [P3](../changelog/2026-05-21-Callback-P3.md) · [DocAlign](../changelog/2026-06-06-Callback-DocAlign.md)

---

## 1. 模块定位

Callback：全链路回调钩子，覆盖 Agent/Model/Tool 与 OnEvent。`internal/agent/callbacks` 为 Chain 真相源；`plugintrpc.Manager` 聚合 Hook 与 Runner Plugin。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| Chain | `internal/agent/callbacks/` |
| 装配 | `internal/agent/callback_chain.go`、`product_chain_builtins.go` |
| Manager | `internal/plugin/trpc/manager.go`（含编排边界注释）、`orchestration_policy.go` |
| Hook 领域 | `internal/biz/hook/hook.go`（实现）、`internal/biz/hook.go`（re-export）、`internal/biz/hook_resolver.go` |
| Hook 执行 | `internal/plugin/trpc/hook_callbacks.go`、`hook_modify.go`、`hook_events.go`、`hook_notify.go`、`hook_audit.go`、`hook_resilience.go`、`hook_retry_worker.go` |
| 前端 | `web/src/pages/HooksPage.vue`、`HookDeliveriesPage.vue`、`PluginRunsPage.vue`、`AgentHooksPanel.vue`、`CallbackEditor.vue` |

---

## 2. 现状评估（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| Chain 抽象 + 适配器 | ✅ | `callbacks.go`、`adapter.go`、单测 |
| Agent/Model/Tool Chain 挂载 | ✅ | `buildCallbackChainOptions` → `With*Callbacks` |
| 产品工具链 | ✅ | guard/cache/timing/confirm/recorder/circuit-breaker/command-safety |
| PluginManager | ✅ | `manager.go`：`MergeChain`、`RunnerPluginsForAgent`、`OnEvent` |
| Hook → Chain | ✅ | `hook/hook.go` + `hook_resolver.go` + `HookCallbacks` + `wrapResilientHooks` |
| Runner 内置 Plugin | ✅ | `runtime.go` + 9 `builtin()`；scope 过滤 |
| OnEvent + Hook on_event | ✅ | `productEventPlugin`、`hook_events.go` |
| Prometheus 回调指标 | ✅ | `metrics.ObserveCallback`、`PluginInvokeTotal`（product_chain） |
| 前端 Hook 管理 | ✅ | `/hooks`、`/hooks/deliveries`、`/plugins/runs`、`AgentHooksPanel`、`CallbackEditor` |
| 编排统一 Runner 路径 | ✅ | `orchestration_policy.go`：`ResolvePluginOrchestration` 始终返回 `OrchestrationRunner`；Chain 镜像已废弃 |
| Hook notify 投递队列 | ✅ | `hook_deliveries` + `HookNotifier` 重试 + `HookDeliveryRetryWorker` |

---

## 3. 优化列表（按优先级）

### P0 — 文档真理库（本轮）

| ID | 项 | 状态 |
|----|-----|------|
| CB-DOC-01 | `28 callback.md` 移除过时「未接通」现状，回归纯需求 | ✅ |
| CB-DOC-02 | `28 callback.design.md` 对齐四层编排与真实文件索引 | ✅ |
| CB-DOC-03 | `28-callback-development.md` 作为实现差距唯一来源 | ✅ |

### P1 — 代码质量 / 单一职责（本轮）

| ID | 项 | 状态 |
|----|-----|------|
| CB-CODE-01 | 产品链指标遥测拆至 `product_chain_builtins.go` | ✅ |
| CB-CODE-02 | `callbacks.PluginCallback` 注释与 Manager 实现对齐 | ✅ |
| CB-CODE-03 | `callback_chain.go` 引用 `manager.go` 编排边界注释 | ✅ |

### P2 — 产品与可观测（2026-05-21 ✅）

| ID | 项 | 状态 |
|----|-----|------|
| CB-01 | Audit 查询体验 | ✅ `/plugins/runs` 生命周期点/Agent/结果/时间筛选；Hook `hook:` 落库；详情弹窗 |
| CB-02 | Hook `modify` 深度合并 | ✅ `tool_argument_merge.go` + 单测；design §4.6；CallbackEditor 说明 |
| CB-03 | `ListPluginRuns` 与 Callback 联动 | ✅ 共享 `callback/constants.ts`；Hooks→Runs 深链；`hook:` 前缀 SQL |

### P3 — 架构演进（2026-05-21 ✅）

| ID | 项 | 状态 |
|----|-----|------|
| CB-ARCH-01 | 编排统一 Runner 路径 | ✅ `orchestration_policy.go`：`ResolvePluginOrchestration` 始终返回 Runner；Chain 镜像已废弃，`plugin_chain_mirror.go` 已移除 |
| CB-ARCH-02 | Hook 投递队列 | ✅ `hook_deliveries` + `hook_notify.go`；前端 notify 重试字段 |

---

## 4. 开发阶段（已完成）

### Phase 1：Agent/Model/Tool Chain 挂载

- `buildCallbackChainOptions` + `productCallbackChain`
- `ToolRecorderCallback` 替代原 `buildToolCallbacks`

### Phase 2：PluginManager + Hook 桥接

- `manager.go`、`hook_resolver.go`、`MergeChain`
- AuditLog 等内置 Plugin 覆盖多回调点（Runner 层）

### Phase 3：OnEvent + 产品闭环

- `RunnerPlugins` / `productEventPlugin`
- HooksPage + CallbackEditor
- Prometheus `ObserveCallback`

---

## 5. 任务清单（归档）

| # | 任务 | Phase | 状态 |
|---|------|-------|------|
| 1 | Chain 注入 Agent/Model/Tool | 1 | ✅ |
| 2 | ToolRecorder 迁入 Chain | 1 | ✅ |
| 3 | TRPCBuilderDeps.PluginManager | 2 | ✅ |
| 4 | PluginManager BuildChain/MergeChain | 2 | ✅ |
| 5 | hook_resolver + hook_callbacks | 2 | ✅ |
| 6 | AuditLog 多回调点（Runner） | 2 | ✅ |
| 7 | OnEvent 桥接 | 3 | ✅ |
| 8 | Hook webhook notify | 3 | ✅ |
| 9 | Wire HookResolver + Manager | 2 | ✅ |
| 10 | Prometheus 回调指标 | 3 | ✅ |
| 11 | CallbackEditor + HooksPage | 3 | ✅ |

---

## 6. 验收标准

- [x] Agent / Model / Tool 回调触发（Chain）
- [x] PluginManager 统一 Hook 合并与 Runner Plugin 注入
- [x] Hook 规则解析并生效（含 block/modify/notify）
- [x] OnEvent 与 Hook `on_event` 作用域正确
- [x] 产品层 Hook CRUD + Agent 设置面板
- [x] `go test ./internal/agent/callbacks/... ./internal/plugin/trpc/...`

---

## 7. Review 跟进（P0–P2，2026-05-21 ✅）

| ID | 项 | 状态 |
|----|-----|------|
| CB-R0-01 | `StatsRecorder.RecordEvent`；`recordHookAudit` 去掉 `*RepoStatsRecorder` 断言 | ✅ |
| CB-R0-02 | `EnqueueNotify` Insert 改 `safego` 异步 | ✅ |
| CB-R1-01 | Chain 镜像已废弃，Hook 统一使用 `wrapResilientHooks` | ✅ |
| CB-R1-02 | notify 指标 `queued` / `delivery_failed` | ✅ |
| CB-R1-03 | Chain 镜像白名单已随镜像移除而废弃；编排统一 Runner | ✅ |
| CB-R2-01 | `ListHookDeliveries` API + `/hooks/deliveries` 页 | ✅ |
| CB-R2-02 | Webhook SSRF（`pkg/webhookurl`） | ✅ |

变更：[Review-Fixes](../changelog/2026-05-21-Callback-Review-Fixes.md) · [Review-P0-P2](../changelog/2026-05-21-Callback-Review-P0-P2.md)

---

## 8. 依赖与风险

- **框架**：依赖 trpc-agent-go `llmagent.With*Callbacks` 与 `plugin.Plugin.OnEvent`
- **Hook 兼容**：`config_json` 仅可选扩展字段
- **性能**：链过长增加延迟；以 `ObserveCallback` 监控
- **错误隔离**：Hook 层 `wrapResilientHooks`；Chain Agent 侧 `ContinueOnError(false)` 与框架默认一致
