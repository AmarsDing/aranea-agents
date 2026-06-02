# Plugin 插件 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 Phase 6 已完成；P3 沙箱/版本待做
> **需求**：[22 plugin.md](./22%20plugin.md) · **设计**：[22 plugin.design.md](./22%20plugin.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)
> **变更**：[changelog/2026-05-21-Plugin-Phase6.md](../changelog/2026-05-21-Plugin-Phase6.md) · [Review 修复](../changelog/2026-05-21-Plugin-Phase6-Review-Fixes.md)

---

## 1. 模块定位

Plugin 是 Runner 层运行时回调扩展（治理 / 调试 / 风控）。与 Skill / Tool / Hook 的边界见 `internal/plugin/trpc/orchestration.go`。

**代码锚点**：
- `api/kratos/plugin/v1/plugin.proto`
- `internal/service/plugin.go` — CRUD + Bootstrap + 热重载
- `internal/biz/plugin.go` — Usecase + Schema 校验 + Scope 校验
- `internal/plugin/trpc/` — 9 内置插件、`Runtime`、`Manager`、telemetry
- `internal/agent/tool_confirm_gate.go` — 统一 ConfirmGate（Chain）
- `internal/agent/model_selector.go` — model_router / cost_guard 真路由
- `web/src/components/plugins/PluginSchemaForm.vue` — Schema 驱动配置
- `web/src/components/plugins/ModelRouterRulesEditor.vue` — model_router rules[] 可视化

---

## 2. 现状评估（2026-05-21）

| 项 | 状态 | 说明 |
|----|------|------|
| Plugin CRUD + 热重载 | ✅ | List / Toggle / Config / Sort / Scope |
| 9 内置插件 | ✅ | `adapter.builtin()` 全覆盖 |
| ConfirmGate 统一 | ✅ | Chain 合并 catalog + confirmation_guard；AwaitUserReply 审批 |
| model_router rules[] | ✅ | priority + contains/regex/min_chars + 可视化编辑器 |
| cost_guard 日预算持久化 | ✅ | `plugin_cost_guard_usage` 跨进程 |
| cost_guard Agent scope 分桶 | ✅ | BeforeModel + ModelSelector 共用 `BudgetTrackerForContext` |
| Schema 配置表单 | ✅ | 表单 / JSON 双模式 |
| retry_and_reflect 反思注入 | ✅ | CustomResult + `plugin.retry_reflect` 事件 |
| 工具确认专用 UI | ✅ | RunStatus await 元数据 + Approve/Deny 按钮 |
| Plugin 沙箱 | ❌ | P3 |
| 版本回滚 | ❌ | P3 |

---

## 3. 回调编排（权威）

| 层级 | 机制 | 顺序 |
|------|------|------|
| Runner Plugin | `WithPlugins`，DB `sort_order` ASC | 内置插件 Before/After/OnEvent |
| LLMAgent Chain | ConfirmGate(10) + metrics + recorder(50) + Hook(300+) | Agent/Model/Tool 回调 |
| ModelSelector | `ChainedModelSelector` | model_router（rules+启发式）→ cost_guard fallback |
| Hook on_event | `productEventPlugin` | 用户 Hook 规则 |

**工具确认**：Catalog `requires_confirmation` + Plugin `confirmation_guard` → **Chain ConfirmGate**；有 `AwaitUserReply` 时 mid-turn 审批，前端 `await_kind=tool_confirm` 专用 UI。Runner `confirmation_guard` 仅 telemetry。

---

## 4. 开发阶段

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

- [ ] T16 外部插件沙箱
- [ ] T17 `plugin_versions` + 回滚 API

---

## 5. 验收标准

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

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 四层回调顺序难感知 | orchestration.go + sort_order 提示 |
| Schema 表单不支持复杂嵌套 | 数组 object 仍用 JSON 子编辑器；完整 JSON 模式保留 |
| 工具确认与通用 await 混淆 | `await_kind` 区分 reply / tool_confirm；内存 cache + 同步 persist |
| high_risk 跳过反思 | confirmation_guard **或** catalog `requires_confirmation` |
| cost_guard 双路径计量 | BeforeModel 与 ModelSelector 共用 `BudgetTrackerForContext` |
