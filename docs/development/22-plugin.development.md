# Plugin 插件 — 开发计划

> **版本**：2026-08-13 | **状态**：🟢 Phase 6 已完成；P3 沙箱/版本待做
> **需求**：[22-plugin.md](./22-plugin.md) · **设计**：[22-plugin.design.md](./22-plugin.design.md)

---

## 1. 模块定位

Plugin 是 Runner 层运行时回调扩展（治理 / 调试 / 风控）。与 Skill / Tool / Hook 的边界见 `internal/plugin/trpc/manager.go` 顶部编排注释。

**代码锚点**：

| 层级 | 文件路径 | 说明 |
|------|----------|------|
| Proto | `api/kratos/plugin/v1/plugin.proto` | Plugin / PluginRun 消息 + 7 RPC |
| Service | `internal/service/plugin.go` | CRUD + Bootstrap 种子同步 + 热重载 + Run 查询 |
| Biz | `internal/biz/plugin/plugin.go` | Usecase + Schema 校验 + Scope 校验 + Run 管理 |
| Biz Alias | `internal/biz/plugin.go` | type alias 重导出到 biz 包 |
| Data — Plugin | `internal/data/plugin.go` | Ent Repo（PlatformPlugin） |
| Data — Run | `internal/data/plugin_run.go` | Raw SQL Repo（plugin_runs 表） |
| Data — CostGuard | `internal/data/plugin_cost_guard_usage.go` | Raw SQL Repo（plugin_cost_guard_usage 表） |
| Ent Schema | `internal/data/ent/schema/plugin.go` | PlatformPlugin（表名 plugins） |
| DDL — Run | `internal/data/sql/plugin_run.sql` | plugin_runs 建表（迁移版本 20260621） |
| Runtime | `internal/plugin/trpc/runtime.go` | Runtime 热重载 + scope 过滤 |
| Manager | `internal/plugin/trpc/manager.go` | RunnerPluginsForAgent + 回调编排 |
| Adapter | `internal/plugin/trpc/adapter.go` | biz.Plugin → adaptedPlugin |
| Registry | `internal/plugin/trpc/registry.go` | BuiltinPluginDefs + Schema 常量 |
| Chain Adapter | `internal/plugin/trpc/chain_adapter.go` | Plugin → Chain 条目 + 回调点声明 |
| 内置插件 | `internal/plugin/trpc/{audit,skill_tracker,retry_reflect,sensitive_mask,confirmation_guard,cost_guard,model_router,permission_guard,output_policy}.go` | 9 个内置插件实现 |
| ConfirmGate | `internal/agent/tool_confirm_gate.go` | 统一 ConfirmGate（Chain） |
| ModelSelector | `internal/agent/model_selector.go` | ChainedModelSelector：model_router → cost_guard |
| 前端 API | `web/src/features/plugins/api.ts` | API 调用封装 |
| 前端页面 | `web/src/pages/PluginsPage.vue` / `web/src/pages/PluginRunsPage.vue` | 管理页 / 运行记录页 |
| 前端组件 | `web/src/components/plugins/*.vue` | 表格 / 详情 / 配置 / Schema 表单 / 规则编辑器 |

---

## 2. 现状评估（2026-06-06）

| 项 | 状态 | 说明 |
|----|------|------|
| Plugin CRUD + 热重载 | ✅ | List / Toggle / Config / Sort / Scope |
| Plugin Run 查询 + 清空 | ✅ | ListPluginRuns / DeleteAllPluginRuns |
| 9 内置插件 | ✅ | `adapter.builtin()` 全覆盖 |
| ConfirmGate 统一 | ✅ | Chain 合并 catalog + confirmation_guard；AwaitUserReply 审批 |
| model_router rules[] | ✅ | priority + contains/regex/min_chars + 可视化编辑器 |
| cost_guard 日预算持久化 | ✅ | `plugin_cost_guard_usage` 跨进程 |
| cost_guard Agent scope 分桶 | ✅ | BeforeModel + ModelSelector 共用 `BudgetTrackerForContext` |
| Schema 配置表单 | ✅ | 表单 / JSON 双模式 |
| retry_and_reflect 反思注入 | ✅ | CustomResult + `plugin.retry_reflect` 事件 |
| 工具确认专用 UI | ✅ | RunStatus await 元数据 + Approve/Deny 按钮 |
| Scope Agent 校验 | ✅ | `UpdateScope` 通过 `ScopeAgentLookup.AgentExists` 校验 |
| 种子同步 Bootstrap | ✅ | `NewPluginServiceWithBootstrap` 启动时同步内置插件 |
| Plugin 沙箱 | ❌ | P3 待做（2026-08-13 死代码清理：`SandboxMode` 类型定义已移除，需要时重建） |
| 版本回滚 | ❌ | P3 待做（2026-08-13 死代码清理：`VersionPolicy` 类型定义已移除，需要时重建） |
| `plugin_cost_guard_usage` DDL 注册 | ✅ | 迁移 20261210 `plugin_cost_guard_usage_schema` 已注册（GAP-01 修复） |

---

## 3. 回调编排（权威）

| 层级 | 机制 | 顺序 |
|------|------|------|
| Runner Plugin | `WithPlugins`，DB `sort_order` ASC | 内置插件 Before/After/OnEvent |
| LLMAgent Chain | ConfirmGate(10) + metrics + recorder(50) + Hook(300+) | Agent/Model/Tool 回调 |
| ModelSelector | `ChainedModelSelector` | model_router（rules+启发式）→ cost_guard fallback |
| Hook on_event | `productEventPlugin` | 用户 Hook 规则 |

**工具确认**：Catalog `requires_confirmation` + Plugin `confirmation_guard` → **Chain ConfirmGate**；有 `AwaitUserReply` 时 mid-turn 审批，前端 `await_kind=tool_confirm` 专用 UI。Runner `confirmation_guard` 仅 telemetry。

> 完整回调编排注释位于 `internal/plugin/trpc/manager.go` 顶部。详细架构设计参见 [22-plugin.design.md](./22-plugin.design.md) §一、§七、§十一。

---

## 4. 差距与优化

### 4.1 已知差距

| 编号 | 差距 | 影响 | 优先级 |
|------|------|------|--------|
| ~~GAP-01~~ | ~~`plugin_cost_guard_usage` 表无 DDL 迁移注册~~ | ✅ 已修复（2026-08-13，迁移 20261210） | — |
| ~~GAP-02~~ | ~~`retry_and_reflect` registry 只注册 `after_tool`，但 `chain_adapter` 声明含 `after_agent`~~ | ✅ 已修复（2026-08-13，种子声明补齐 `after_agent` + `TestBuiltin_SeedCallbackPointsMatchImplementation` 回归） | — |
| GAP-03 | `cost_guard` schema 缺少 `admin_bypass` 字段（需求 §2.5 列出） | 管理员绕过功能未实现 | 低 |
| GAP-04 | `permission_guard` schema 缺少 `confirm_tools` 和 `role_rules` 字段（需求 §2.7 列出） | 基于角色的工具权限规则未实现 | 低 |
| GAP-05 | `NewPluginServiceWithBootstrap` 构造函数副作用（TECH-DEBT #plugin-bootstrap） | 应在 Wire 图构造后显式调用 | 低 |

### 4.2 优化建议

- ~~GAP-01：在 `ddl_migration_registry.go` 注册 `plugin_cost_guard_usage` 建表 SQL~~ ✅（2026-08-13，迁移 20261210 + PG 集成测试）
- ~~GAP-02：统一 `retry_and_reflect` 的 registry 回调点与 chain_adapter 声明~~ ✅（2026-08-13，种子声明 + 实现一致性回归测试）
- GAP-03/04：评估是否补全 schema 字段或更新需求文档移除未实现字段

---

## 5. 开发阶段

### Phase 1–3（✅）

基础设施、内置插件、前端 scope/runs — 已完成。

### Phase 5：插件加深（✅ 2026-05-21）

- [x] T19 ConfirmGate + AwaitUserReply
- [x] T20 model_router.rules[]
- [x] T21 cost_guard daily_token_budget 持久化
- [x] T22 Schema 驱动配置表单

### Phase 6：运行时 UX 加深（✅ 2026-05-21）

- [x] T18 retry_and_reflect 事件流反思（CustomResult + bus）
- [x] T23 cost_guard 按 Agent scope 分桶
- [x] T24 工具确认专用 UI（结构化 reply，非文本启发式）
- [x] T25 model_router rules[] 可视化规则编辑器

### Phase 4：进阶能力（P3 待做）

- [ ] T16 外部插件沙箱（类型定义已于 2026-08-13 死代码清理移除，实现时重建）
- [ ] T17 `plugin_versions` + 回滚 API（类型定义已于 2026-08-13 死代码清理移除，实现时重建）

---

## 6. 验收标准

### 已验收

- [x] ConfirmGate 合并 catalog + confirmation_guard
- [x] Chat 有 AwaitHook 时可 mid-turn 审批工具
- [x] 工具确认 UI 发送 `__aranea:tool_confirm:approve|deny`，后端结构化解析
- [x] rules[] 配置化路由优先于 code/long_context 启发式
- [x] rules[] 可在 Plugin 配置页可视化编辑
- [x] cost_guard 重启后日预算累计不丢失
- [x] cost_guard 全局 scope 时按 agent_id 独立计桶
- [x] retry_and_reflect 失败时 LLM 收到 reflection_hint CustomResult
- [x] Plugin 配置页 Schema 表单可编辑并保存

### 待验收（P3）

- [ ] 外部插件沙箱隔离
- [ ] 插件版本管理与回滚

---

## 7. 改动文件清单

### 后端

| 文件路径 | 改动类型 | 说明 |
|----------|----------|------|
| `api/kratos/plugin/v1/plugin.proto` | 新增 | Plugin / PluginRun 消息 + 7 RPC |
| `internal/service/plugin.go` | 新增 | CRUD + Bootstrap + 热重载 + Run 查询 |
| `internal/biz/plugin/plugin.go` | 新增 | Usecase + Repo/RunRepo 接口 + Schema 校验 |
| `internal/biz/plugin.go` | 新增 | type alias 重导出 |
| `internal/data/plugin.go` | 新增 | Ent Repo 实现 |
| `internal/data/plugin_run.go` | 新增 | Raw SQL Repo（plugin_runs） |
| `internal/data/plugin_cost_guard_usage.go` | 新增 | Raw SQL Repo（plugin_cost_guard_usage） |
| `internal/data/plugin_run_schema.go` | 新增 | DDL Ensure 函数 |
| `internal/data/ent/schema/plugin.go` | 新增 | PlatformPlugin Ent Schema |
| `internal/data/sql/plugin_run.sql` | 新增 | plugin_runs 建表 SQL |
| `internal/plugin/trpc/runtime.go` | 新增 | Runtime 热重载 + scope 过滤 |
| `internal/plugin/trpc/manager.go` | 新增 | Manager + 回调编排 |
| `internal/plugin/trpc/adapter.go` | 新增 | biz.Plugin → adaptedPlugin 适配 |
| `internal/plugin/trpc/registry.go` | 新增 | BuiltinPluginDefs + Schema 常量 |
| `internal/plugin/trpc/chain_adapter.go` | 新增 | Plugin → Chain 条目适配 |
| `internal/plugin/trpc/{9 个内置插件}.go` | 新增 | 9 个内置插件实现 |
| `internal/plugin/trpc/cost_guard_budget.go` | 新增 | 日预算持久化 tracker |
| `internal/plugin/trpc/cost_guard_registry.go` | 新增 | Agent scope 分桶 registry |
| `internal/plugin/trpc/model_router_rules.go` | 新增 | rules[] 路由规则 |
| `internal/plugin/trpc/base_plugin.go` | 新增 | basePlugin 嵌入基类 |
| `internal/plugin/trpc/safe_logger.go` | 新增 | PluginSafeLogger（红线 16 合规） |
| `internal/plugin/trpc/{identity,guardrail}_bridge.go` | 新增 | 框架 Plugin 桥接 |
| `internal/agent/tool_confirm_gate.go` | 新增 | 统一 ConfirmGate |
| `internal/agent/model_selector.go` | 新增 | ChainedModelSelector |

### 前端

| 文件路径 | 改动类型 | 说明 |
|----------|----------|------|
| `web/src/features/plugins/api.ts` | 新增 | API 调用封装 |
| `web/src/features/plugins/pluginRunsTableUi.ts` | 新增 | 运行记录表 UI 工具 |
| `web/src/pages/PluginsPage.vue` | 新增 | Plugin 管理页 |
| `web/src/pages/PluginRunsPage.vue` | 新增 | Plugin 运行记录页 |
| `web/src/components/plugins/PluginsTable.vue` | 新增 | 插件列表表格 |
| `web/src/components/plugins/PluginDetailDialog.vue` | 新增 | 插件详情对话框 |
| `web/src/components/plugins/PluginConfigDialog.vue` | 新增 | 配置详情对话框 |
| `web/src/components/plugins/PluginSchemaForm.vue` | 新增 | Schema 驱动配置表单 |
| `web/src/components/plugins/ModelRouterRulesEditor.vue` | 新增 | rules[] 可视化编辑器 |
| `web/src/components/plugins/PluginRunDetailDialog.vue` | 新增 | 运行详情对话框 |
| `web/src/components/plugins/pluginUi.ts` | 新增 | 插件 UI 工具函数 |
| `web/src/stores/plugins/index.ts` | 新增 | Pinia store |

---

## 8. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 四层回调顺序难感知 | manager.go 编排注释 + sort_order 提示 |
| Schema 表单不支持复杂嵌套 | 数组 object 仍用 JSON 子编辑器；完整 JSON 模式保留 |
| 工具确认与通用 await 混淆 | `await_kind` 区分 reply / tool_confirm；内存 cache + 同步 persist |
| high_risk 跳过反思 | confirmation_guard **或** catalog `requires_confirmation` |
| cost_guard 双路径计量 | BeforeModel 与 ModelSelector 共用 `BudgetTrackerForContext` |
| ~~`plugin_cost_guard_usage` 表 DDL 缺失~~ | ✅ GAP-01 已修复（迁移 20261210） |
| 构造函数副作用（Bootstrap） | TECH-DEBT #plugin-bootstrap：应在 Wire 图构造后显式调用 |

---

## 9. 整体评审修复批次（2026-08-14，✅ 已完成）

整体深入评审发现 6 阻断级（N-B1~N-B6）+ 12 推荐级（R 系列），已全部修复并通过回归测试。

### 阻断级

| # | 问题 | 修复 |
|---|------|------|
| N-B1 | 三个 config getter 遍历全工作区 map，跨租户配置泄漏且选中结果受 map 迭代序影响 | getter 增加 `workspaceID` 参数，经 `configEntriesFor` 过滤（本租户 > shared；system 确定性 shared 优先 + 分区按 ID 排序）；调用点（trpc_build / tool_confirm_gate / ToolMatchesConfirmationGuard）传 `workspace.IDFromContext(ctx)` |
| N-B2 | build 时固化的预算 tracker 桶（`default:{agent}`）与运行时桶（`{ws}:{agent}`）分裂计量 | `PluginCostGuardSelector` 改为按请求 ctx 解析 tracker（`Manager.BudgetTrackerForContext`），两条路径共用同一桶 |
| N-B3 | HookDeliveryRetryWorker 非缓冲 channel 致 Stop 信号在 retryStale 期间丢失、goroutine 泄漏 | close(stop) 广播 + sync.Once 幂等 + done 通道有界 Wait |
| N-B4 | nil 接收器路径解引用（fallbackBudgetTracker / runtime / cost_guard budget） | 显式接口类型声明 + nil 守卫（后由 R-1 进一步收口为 nil 兜底） |
| N-B5 | （批次 A/B 已修，见 §8 风险表与 git 记录） | — |
| N-B6 | （批次 A/B 已修） | — |

### 推荐级（本批）

| # | 问题 | 修复 |
|---|------|------|
| R-1 | 兜底路径每次调用 `NewCostGuardBudgetTracker`，泄漏 persist/retry 两个 worker goroutine | 兜底统一返回 nil（tracker 全部方法 nil 接收器 no-op），删除 `fallbackBudgetTracker` |
| R-2 | `byScope` 分桶（`{ws}:{agent}`）无界增长 | 软上限 1024 + idle>48h 淘汰；淘汰前 Close 冲刷，新桶从 DB 重载 |
| R-3 | 并发热重载乱序提交，陈旧快照覆盖新配置 | `applySeq`/`appliedSeq` 纪元守卫，陈旧 Apply 丢弃 + Warn 日志 |
| R-4 | `TryConsume` 失败回滚在跨日窗口误减新日额度 | 回滚仅在 `t.day == 捕获日` 时执行（`rollbackReservation`） |
| R-5 | 管理员非法 config JSON 静默 fail-open（guard 插件按默认配置运行） | 解析失败 Warn 日志后回退默认配置 |
| R-6 | PluginSafeLogger 每条日志 spawn 一个 goroutine 发布 MonitorEvent | 共享有界队列（256）+ 单 worker；满则丢弃并计数限流告警 |
| R-7 | 租户与 shared 同 key 插件同时返回，重复注册回调 | `PluginsForAgent` 按 key 去重，租户自有覆盖 shared |

### 回归测试

`internal/plugin/trpc/runtime_r_series_test.go`（R-1/R-2/R-3/R-4/R-6/R-7 共 6 项）、`runtime_workspace_test.go::TestRuntime_ConfigGetters_workspaceIsolation`（N-B1）、`hook_retry_worker_test.go`（N-B3）。全部 PASS；`internal/plugin/...`、`internal/agent/...`、`internal/data`（PG）测试全绿。

### 改动文件（本批）

| 文件 | 改动 |
|------|------|
| `internal/plugin/trpc/runtime.go` | N-B1 configEntriesFor + 三 getter 签名、R-3 纪元、R-7 去重、删 legacy `CostGuardBudgetTracker()` |
| `internal/plugin/trpc/manager.go` | getter 签名透传、`BudgetTrackerForContext`、删 `CostGuardBudgetTracker{,ForAgent}` |
| `internal/plugin/trpc/cost_guard_registry.go` | R-1 nil 兜底、R-2 分桶淘汰、N-B1（ToolMatchesConfirmationGuard）、删 `CostGuardBudgetTrackerForAgent` |
| `internal/plugin/trpc/cost_guard_budget.go` | R-4 rollbackReservation 跨日守卫、lastUsedUnix |
| `internal/plugin/trpc/cost_guard.go` | R-1 budget() nil 兜底 |
| `internal/plugin/trpc/safe_logger.go` | R-6 有界队列 + 单 worker |
| `internal/plugin/trpc/hook_retry_worker.go` | N-B3 Stop 广播 + 幂等 + Wait |
| `internal/plugin/trpc/config.go` | R-5 解析失败告警 |
| `internal/agent/model_selector.go` | N-B2 tracker 按 ctx 解析 |
| `internal/agent/trpc_build.go` | N-B1/N-B2 调用点 |
| `internal/agent/tool_confirm_gate.go` | N-B1 调用点 |
