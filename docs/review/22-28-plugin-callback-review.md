# 22/28 Plugin / Callback Review

> **评分**：81 / 100 | **风险等级**：P1  
> **文档**：[22-plugin-development.md](../需求/22-plugin-development.md) · [28-callback-development.md](../需求/28-callback-development.md)  
> **代码锚点**：`internal/plugin/trpc/` · `internal/biz/plugin.go` · `internal/biz/hook.go` · `web/src/pages/PluginsPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 9 内置 Plugin + Chain+Hook+OnEvent + Schema/Scope 闭环 ✅；沙箱/版本（Phase 4）未实现 |
| 架构一致性 | 22 | 25 | `internal/plugin/trpc` 四层分工（orchestration.go）文档化 ✅；Plugin Manager + Hook 桥接 ✅ |
| 后端实现质量 | 17 | 20 | 9 内置 `builtin()` ✅；OnEvent scope ✅；cost_guard 分桶 ✅；model_router rules[] ✅；ConfirmGate ✅ |
| 前端实现质量 | 14 | 15 | Plugin 表格 + Schema 表单 + Agent 绑定 ✅；`PluginRunsPage` 扩展筛选 ✅；`ModelRouterRulesEditor` ✅ |
| 测试与验证 | 6 | 10 | `runtime_scope_test.go` ✅；Hook 投递测试 ✅；Plugin 链 E2E 测试缺失 |
| 文档一致性 | 6 | 10 | Phase 1–3 + P2/P3 状态表同步 ✅；orchestration.go 四层分工已文档化 |

---

## 9 内置 Plugin 状态

| Plugin | 类型 | 状态 |
|--------|------|------|
| `model_router` | 模型路由（ModelSelector）| ✅ 单一路由 + rules[] |
| `retry_and_reflect` | 失败重试 + 事件流反思 | ✅ CustomResult |
| `cost_guard` | 日预算保障 | ✅ CostGuardBudgetRegistry + Agent scope 分桶 |
| `permission_guard` | 权限控制 | ✅ deny_tools（confirm_tools 不阻断）|
| `token_limiter` | Token 限制 | ✅ |
| `audit_logger` | 审计日志 | ✅ PluginSafeLogger + telemetry enrich |
| `content_filter` | 内容过滤 | ✅ |
| `human_approval` / `confirmation_guard` | 工具确认 | ✅ ConfirmGate + AwaitUserReply 统一 |
| `stats_recorder` | 统计记录 | ✅ StatsRecorder |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Plugin CRUD + 配置 | ✅ |
| `builtin()` 实现（9 个）| ✅ |
| Plugin Chain + Hook 桥接 | ✅ |
| OnEvent（scope + agent_id）| ✅ |
| Schema 配置表单（`PluginSchemaForm`）| ✅ |
| Agent scope 绑定（global/agent_id）| ✅ |
| Plugin 种子同步 | ✅ |
| `config_schema_json` 校验 | ✅ |
| `PluginsForAgent` scope 过滤 | ✅ |
| `ListPluginRuns` 扩展筛选 | ✅ I7 |
| `ModelRouterRulesEditor` | ✅ I7 |
| `cost_guard` 日预算持久化 | ✅ I7 |
| 工具确认专用 UI | ✅ I7 |
| Phase 4 沙箱/版本 | ❌ |
| Callback 链 E2E 测试 | ❌ |

---

## 架构四层分工（orchestration.go）

| 层 | 职责 |
|----|------|
| Plugin Manager | Plugin 生命周期管理 + Agent scope 过滤 |
| Hook 桥接 | 将 Plugin 回调挂入 Runner Hook 点 |
| Callback Chain | 按优先级串联多个 Plugin 回调 |
| OnEvent | 异步侧效处理（Monitor/Audit/Usage）|

**状态**：四层分工文档化完整 ✅

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| PLG-P1-01 | Plugin 链 E2E 测试缺失（多 Plugin 串联 + OnEvent 异步路径）| 补 Plugin 链集成测试 |
| PLG-P1-02 | `PluginsPage.vue` 直连 `features/plugins/api`，违反分层规范 | 引入 `usePluginsPage.ts` composable 封装 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| PLG-P2-01 | Phase 4（沙箱/版本/RBAC）未规划时间线 | 在 `22-plugin-development.md` 中明确 Phase 4 scope |
| PLG-P2-02 | `retry_and_reflect` 的反思次数上限和循环检测未暴露配置 | 在 Schema 表单中添加 max_retries 配置 |

---

## Hook / Callback 子模块（28）

| 功能 | 状态 |
|------|------|
| Hook CRUD（`CallbackEditor`）| ✅ |
| Hook 投递队列（`/hooks/deliveries`）| ✅ |
| Hook 审计记录 | ✅ |
| `hook_audit.go` | ✅ |
| Phase 1–3 EP-CB-01 闭环 | ✅ |

---

## 建议优化路径

1. 补 Plugin 链集成测试（P1）。
2. 重构 `PluginsPage.vue` 引入 composable（P1）。
3. 规划 Phase 4 沙箱/版本时间线（P2）。
