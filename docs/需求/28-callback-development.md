# Callback 回调 — 开发计划

> **版本**：2026-05-19 | **状态**：🟢 Phase 1–3 已落地
> **需求**：[28 callback.md](./28%20callback.md) · **设计**：[28 callback.design.md](./28%20callback.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-CB-01

---

## 1. 模块定位

Callback 回调：全链路回调钩子，覆盖 Agent/Model/Tool 执行前后的拦截、修改和增强。基于 `internal/agent/callbacks` Chain 抽象，桥接产品层 Plugin/Hook 与 trpc-agent-go 原生 Callback 体系。

**代码锚点**：
- `internal/agent/callbacks/` — Chain 抽象 + 适配器（已实现）
- `internal/agent/trpc_build.go` — Agent 构造 + Tool 回调注入（部分实现）
- `internal/plugin/trpc/` — Plugin Runtime + AuditLogPlugin（已实现）
- `internal/biz/hook.go` — Hook CRUD + `hook_config` / `hook_resolver`
- `internal/biz/plugin.go` — Plugin CRUD + `IncrementStats`

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Chain 回调链抽象 | ✅ | `callbacks.go` + `adapter.go` + 测试 |
| Tool Before/After 回调 | ✅ | Chain：`toolCallTiming` + `toolConfirmation` + `ToolRecorderCallback` |
| Plugin 运行时注入 | ✅ | `trpc_runtime.go:WithPlugins` |
| AuditLogPlugin | ✅ | Agent/Model/Tool 全点 + `StatsRecorder` 回写 |
| Hook CRUD | ✅ | `biz/hook.go` + `data/hook.go` + `service/hook.go` |
| Plugin CRUD | ✅ | `biz/plugin.go` + `data/plugin_repo.go` |
| Agent BeforeAgent/AfterAgent | ✅ | `buildCallbackChainOptions` → `WithAgentCallbacks` |
| Model BeforeModel/AfterModel | ✅ | `buildCallbackChainOptions` → `WithModelCallbacks` |
| PluginManager 统一管理 | ✅ | `plugin/trpc/manager.go` + `TRPCBuilderDeps.PluginManager` |
| OnEvent 事件回调 | ✅ | `RunnerPlugins()` + `aranea_event_bridge`；Hook `on_event` 点 |
| Hook → Callback 桥接 | ✅ | `biz/hook_resolver.go` + `hook_callbacks.go` |
| Plugin → Chain 桥接 | 🟡 | Plugin 经 Runner `WithPlugins`；`chain_adapter` 校验 callback_points；Hook 经 Chain + 错误隔离 |

---

## 3. 差距与优化（后续）

1. **P1**：Plugin Phase 2 — 除 `audit_log` 外内置插件回调实现
2. **P2**：Plugin `scope` 运行时过滤 + `UpdatePluginScope` API
3. **P2**：Hook `modify` 覆盖更多字段（tool arguments 深度合并策略文档化）
4. **P3**：将 Plugin 回调点镜像进 Chain（需避免与 Runner `WithPlugins` 双触发）

---

## 4. 开发阶段

### Phase 1：Agent/Model 回调挂载（EP-CB-01 核心）

目标：让 Chain 抽象真正生效，Agent/Model 生命周期回调可触发。

- 修改 `BuildTRPCLLMAgent`：构建 Chain 并通过 `WithAgentCallbacks` / `WithModelCallbacks` 注入
- 将现有 `buildToolCallbacks` 逻辑迁移为 Chain 中的 `ToolRecorderCallback`
- 验证 BeforeAgent / AfterAgent / BeforeModel / AfterModel 回调触发

### Phase 2：PluginManager 统一管理

目标：Plugin 和 Hook 统一生成 Chain，回调可动态配置。

- 新建 `internal/plugin/trpc/manager.go`：PluginManager 聚合 Plugin Runtime + HookResolver
- 新建 `internal/biz/hook_resolver.go`：Hook 规则解析为 Callback
- 修改 `BuildTRPCLLMAgent`：从 PluginManager.BuildChain() 获取 Chain
- 扩展 AuditLogPlugin 覆盖 Agent/Model 回调点

### Phase 3：OnEvent + 产品层闭环

目标：事件流经回调处理，产品层可配置回调规则。

- 实现 PluginManager.OnEvent
- Hook 规则支持 Webhook 通知动作
- 前端 CallbackEditor 组件
- Prometheus 回调指标

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | EP |
|---|------|--------|-------|-----|
| 1 | `internal/agent/trpc_build.go`：构建 Chain，注入 `WithAgentCallbacks` + `WithModelCallbacks` | P0 | 1 | EP-CB-01 |
| 2 | `internal/agent/trpc_build.go`：将 `buildToolCallbacks` 迁移为 Chain 中的 `ToolRecorderCallback`，用 `WithToolCallbacks(chain.AdaptToolCallbacks())` 替换 | P0 | 1 | EP-CB-01 |
| 3 | `internal/agent/trpc_build.go`：`TRPCBuilderDeps` 新增 `PluginManager` 字段 | P1 | 2 | EP-CB-01 |
| 4 | `internal/plugin/trpc/manager.go`：PluginManager 实现（BuildChain + OnEvent） | P1 | 2 | — |
| 5 | `internal/biz/hook_resolver.go`：Hook 规则解析为 Callback | P1 | 2 | — |
| 6 | `internal/plugin/trpc/audit.go`：扩展 AuditLogPlugin 覆盖 Agent/Model 回调点 | P2 | 2 | — |
| 7 | `internal/biz/hook.go`：Hook 增加 CallbackPoint / Condition / Action 语义方法 | P1 | 2 | — |
| 8 | PluginManager.OnEvent 实现 | P2 | 3 | — |
| 9 | Hook 规则 Webhook 通知动作 | P2 | 3 | — |
| 10 | Wire 注入：NewHookResolver + PluginManager → TRPCBuilderDeps | P1 | 2 | — |
| 11 | Prometheus 回调指标（执行次数 / 耗时 / 错误率） | P2 | 3 | — |
| 12 | 前端 CallbackEditor.vue | P2 | 3 | — |

---

## 6. 验收标准

- [x] Agent 执行前后回调正确触发（Phase 1）
- [x] LLM 调用前后回调正确触发（Phase 1）
- [x] Tool 调用前后回调正确触发（Phase 1，不退化）
- [x] PluginManager 统一管理三层回调（Phase 2）
- [x] Hook 规则可解析为 Callback 并生效（Phase 2）
- [x] 事件流经 OnEvent 回调正确处理（Phase 3）
- [x] 产品层可配置回调规则（Phase 3：HooksPage + AgentHooksPanel + CallbackEditor）
- [x] `go test ./internal/agent/callbacks/...` 通过
- [x] `go test ./internal/plugin/trpc/...` 通过

---

## 7. 依赖与风险

- **框架依赖**：Agent/Model 回调注入依赖 trpc-agent-go `llmagent.WithAgentCallbacks` / `WithModelCallbacks` 的可用性
- **Hook 规则兼容**：现有 Hook 的 `config_json` 格式需向后兼容，新增字段以可选方式扩展
- **性能风险**：回调链过长可能增加延迟，需监控单次回调耗时
- **错误隔离**：回调错误不应导致 Agent 运行崩溃，Chain 需支持 `continue-on-error` 配置
