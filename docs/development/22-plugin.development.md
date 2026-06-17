# Plugin 插件 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Phase 6 已完成；P3 沙箱/版本待做
> **需求**：[22 plugin.md](./22%20plugin.md) · **设计**：[22 plugin.design.md](./22%20plugin.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)
> **变更**：[changelog/2026-05-21-Plugin-Phase6.md](../changelog/2026-05-21-Plugin-Phase6.md) · [Review 修复](../changelog/2026-05-21-Plugin-Phase6-Review-Fixes.md)

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
| Plugin 沙箱 | ❌ | P3（`SandboxMode` 类型已定义，未接入运行时） |
| 版本回滚 | ❌ | P3（`VersionPolicy` 类型已定义，未接入运行时） |
| `plugin_cost_guard_usage` DDL 注册 | ⚠️ | 表被代码引用但未找到 DDL 迁移注册（TECH-DEBT） |

---

## 3. 回调编排（权威）

| 层级 | 机制 | 顺序 |
|------|------|------|
| Runner Plugin | `WithPlugins`，DB `sort_order` ASC | 内置插件 Before/After/OnEvent |
| LLMAgent Chain | ConfirmGate(10) + metrics + recorder(50) + Hook(300+) | Agent/Model/Tool 回调 |
| ModelSelector | `ChainedModelSelector` | model_router（rules+启发式）→ cost_guard fallback |
| Hook on_event | `productEventPlugin` | 用户 Hook 规则 |

**工具确认**：Catalog `requires_confirmation` + Plugin `confirmation_guard` → **Chain ConfirmGate**；有 `AwaitUserReply` 时 mid-turn 审批，前端 `await_kind=tool_confirm` 专用 UI。Runner `confirmation_guard` 仅 telemetry。

> 完整回调编排注释位于 `internal/plugin/trpc/manager.go` 顶部。详细架构设计参见 [22 plugin.design.md](./22%20plugin.design.md) §一、§七、§十一。

---

## 4. 差距与优化

### 4.1 已知差距

| 编号 | 差距 | 影响 | 优先级 |
|------|------|------|--------|
| GAP-01 | `plugin_cost_guard_usage` 表无 DDL 迁移注册 | 服务首次启动可能因表不存在导致 cost_guard 持久化失败 | 高 |
| GAP-02 | `retry_and_reflect` registry 只注册 `after_tool`，但 `chain_adapter` 声明含 `after_agent` | 回调点声明与实际注册不一致 | 中 |
| GAP-03 | `cost_guard` schema 缺少 `admin_bypass` 字段（需求 §2.5 列出） | 管理员绕过功能未实现 | 低 |
| GAP-04 | `permission_guard` schema 缺少 `confirm_tools` 和 `role_rules` 字段（需求 §2.7 列出） | 基于角色的工具权限规则未实现 | 低 |
| GAP-05 | `NewPluginServiceWithBootstrap` 构造函数副作用（TECH-DEBT #plugin-bootstrap） | 应在 Wire 图构造后显式调用 | 低 |

### 4.2 优化建议

- GAP-01：在 `ddl_migration_registry.go` 注册 `plugin_cost_guard_usage` 建表 SQL
- GAP-02：统一 `retry_and_reflect` 的 registry 回调点与 chain_adapter 声明
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

- [ ] T16 外部插件沙箱（`SandboxMode` 类型已定义，未接入运行时）
- [ ] T17 `plugin_versions` + 回滚 API（`VersionPolicy` 类型已定义，未接入运行时）

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
| `plugin_cost_guard_usage` 表 DDL 缺失 | GAP-01：需补 DDL 迁移注册 |
| 构造函数副作用（Bootstrap） | TECH-DEBT #plugin-bootstrap：应在 Wire 图构造后显式调用 |
