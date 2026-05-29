# 22/28 Plugin / Callback Review

> **评分**：97 / 100 | **风险等级**：P3  
> **文档**：[22-plugin-development.md](../需求/22-plugin-development.md) · [28-callback-development.md](../需求/28-callback-development.md)  
> **代码锚点**：`internal/plugin/trpc/` · `internal/biz/plugin.go` · `internal/biz/hook.go` · `web/src/pages/PluginsPage.vue`  
> **审查时间**：2026-05-21 · **优化更新**：2026-05-29

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 9 内置 Plugin + Chain+Hook+OnEvent + Schema/Scope 闭环 ✅；沙箱/版本（Phase 4）未实现 |
| 架构一致性 | 24 | 25 | `internal/plugin/trpc` 四层分工文档化 ✅；Plugin Manager + Hook 桥接 ✅；orchestration.go 已删除，职责归入 manager.go 注释 ✅ |
| 后端实现质量 | 20 | 20 | 9 内置 `builtin()` ✅；OnEvent scope ✅；cost_guard 分桶 + Functional Options ✅；model_router rules[] ✅；ConfirmGate ✅；魔法值提取为常量 ✅；并发安全修复 ✅；工厂函数替代裸构造 ✅；persistAdd 异步化 ✅；TaskRepo 子接口拆分 ✅；TaskDispatchReader 窄接口 ✅；TaskGraphResolver 窄接口 ✅；ensureDayLocked 锁优化 ✅ |
| 前端实现质量 | 14 | 15 | Plugin 表格 + Schema 表单 + Agent 绑定 ✅；`PluginRunsPage` 扩展筛选 ✅；`ModelRouterRulesEditor` ✅ |
| 测试与验证 | 7 | 10 | `runtime_scope_test.go` ✅；Hook 投递测试 ✅；Plugin 链 E2E 测试缺失 |
| 文档一致性 | 9 | 10 | Phase 1–3 + P2/P3 状态表同步 ✅；四层分工已文档化 ✅；TECH-DEBT 标注完整 ✅；TD-01 已修复 ✅ |

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

## 架构四层分工

| 层 | 职责 | 代码位置 |
|----|------|----------|
| Plugin Manager | Plugin 生命周期管理 + Agent scope 过滤 | `manager.go`（含 resolveMu + orchestration 边界注释）|
| Hook 桥接 | 将 Plugin 回调挂入 Runner Hook 点 | `hook_notify.go` + `hook_events.go` |
| Callback Chain | 按优先级串联多个 Plugin 回调 | `manager.go` ResolvePlugins |
| OnEvent | 异步侧效处理（Monitor/Audit/Usage）| `stats.go`（channel+batch worker）|

**状态**：四层分工文档化完整 ✅；`orchestration.go` 已删除，职责归入 `manager.go` 注释 ✅

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

## 2026-05-29 优化更新

### 已完成优化

| 项 | 文件 | 改动 |
|----|------|------|
| base_plugin 提取 | `base_plugin.go` | 新建 `basePlugin` 结构体，统一 Name/Stats/Logger |
| safe_logger 重构 | `safe_logger.go` | `os.Stderr` → `event.SysLog*` + `safego.Go` 异步写 |
| manager 并发安全 | `manager.go` | `resolveMu sync.RWMutex` + orchestration 边界注释 |
| model_router 预编译 | `model_router_rules.go` | `compileModelRouterRules` 构建时预编译正则 |
| stats 异步化 | `stats.go` | channel+batch worker + `safego.Go` |
| hook_notify 规范化 | `hook_notify.go` | `kerrors` + 常量提取 + `ctx.WithTimeout` 取消感知 |
| config 清理 | `config.go` | 删除废弃辅助函数 |
| orchestration 删除 | `orchestration.go` | 删除整个文件，职责归入 manager.go |
| identity/guardrail 拆分 | `identity_bridge.go` + `guardrail_bridge.go` | 拆分身份和护栏桥接 |
| plugin_chain_mirror 删除 | `plugin_chain_mirror.go` | 删除整个文件 |
| 魔法值提取 | `model_router.go` + `hook_notify.go` + `output_policy.go` | 提取为命名常量 |
| 包装函数删除 | `cost_guard_budget.go` | `estimateRequestTokens` 内联为 `estimatePromptTokens` |
| TryConsume 锁模式 | `cost_guard_budget.go` | 手动 Unlock → 锁内计算+锁外提交 |
| indexSync 竞态修复 | `memory_l4_cascade.go` | `indexMu sync.RWMutex` 保护 `indexSync` |
| CascadeGraphStore 拆分 | `memory_l4_cascade.go` | Usecase 依赖 4 个子接口（各 ≤ 5 方法） |
| TaskDispatcher 可取消 | `task_dispatcher.go` | `cancel context.CancelFunc` + `sync.Once` |
| TaskDispatcher 纯 context | `task_dispatcher.go` | 移除 `stop chan` + `stopOnce`，统一为 `cancel context.CancelFunc` + `Start()` 幂等保护 |
| Functional Options | `cost_guard_budget.go` | `NewCostGuardBudgetTracker` 重构为 Functional Options（`WithUsageRepo`/`WithScopeKey`），移除 TECH-DEBT 标注 |
| TrackerForScope 统一 | `cost_guard_registry.go` | 使用 `WithScopeKey` + `WithUsageRepo` 选项替代 `SetUsageRepo` 方法 |
| chained reviewer 工厂 | `guardrail_bridge.go` | `newChainedPromptInjectionReviewer`/`newChainedUnsafeIntentReviewer` 工厂函数 |
| TaskRepo 子接口拆分 | `task.go` | 14 方法 `TaskRepo` → 6 个子接口（`TaskReader`/`TaskWriter`/`TaskCommentStore`/`TaskLogStore`/`TaskRunStore`/`TaskEventStore`），`TaskUsecase` 按需组合子接口 |
| TaskDispatchReader 窄接口 | `task_dispatcher.go` | `*TaskUsecase` → `TaskDispatchReader` 接口（6 方法），导出 `IsTaskReadyForDispatch`/`ResolveDispatchAssignee` |
| persistAdd 异步化 | `cost_guard_budget.go` | 同步 DB 写入 → channel+batch worker 异步写入（修复红线 3 违规）；聚合合并同 day+scope delta；`Close()` 优雅关闭 |
| Registry Close | `cost_guard_registry.go` | 新增 `Close()` 方法，`Reset()` 时关闭所有 tracker worker |
| TaskGraphResolver 窄接口 | `task.go` | `*GraphUsecase` → `TaskGraphResolver` 接口（3 方法：`GetExecution`/`FindGraphNode`/`FindNodeDef`），`findNodeDef` 从 3 步调用简化为 1 步 |
| ensureDayLocked 锁优化 | `cost_guard_budget.go` | 持锁同步读 DB → unlock-read-lock + 二次确认模式，消除 DB 延迟对并发 `TryConsume`/`WouldExceed` 的阻塞 |

### TECH-DEBT 标注

| 位置 | 问题 | 标注 | 状态 |
|------|------|------|------|
| `cost_guard_budget.go` | `EstimateInvocationTokens` 直接访问框架内部 | `// TECH-DEBT: framework-internal-access` | Open |
| `cost_guard_budget.go` | `NewCostGuardBudgetTracker` 两阶段构造 | `// TECH-DEBT: two-phase construction` | ✅ 已修复（改为 Functional Options） |

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
4. ~~`TaskRepo` 接口 14 方法 → 按 CRUD/Comment/Log/Run/Event 拆分子接口（P2）~~ ✅ 已完成。
5. ~~`NewTaskDispatcher` 接收 `*TaskUsecase` 具体类型 → 定义 `TaskDispatchReader` 窄接口（P2）~~ ✅ 已完成。
6. `EstimateInvocationTokens` 直接访问框架内部 → 等待框架公开 API 后重构（P2，blocked on framework）。
7. ~~`persistAdd` 在 plugin 回调中同步写库 → 改为异步写入（P3）~~ ✅ 已完成（修复红线 3 违规）。
8. ~~`TaskUsecase` 持有 `*GraphUsecase` 具体类型 → 定义 `TaskGraphResolver` 窄接口（P3）~~ ✅ 已完成。
9. ~~`ensureDayLocked` 在持锁时同步读 DB → 考虑异步化（P3，影响低：每日仅一次）~~ ✅ 已完成（unlock-read-lock + 二次确认模式）。
10. `TaskDispatchReader` 接口 6 方法略超 5 方法上限 → 考虑拆分为 `TaskDispatchChecker`/`TaskDispatchMutator`（P3，收益有限）。
