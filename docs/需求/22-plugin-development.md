# Plugin 插件 — 开发计划

> **版本**：2026-05-18 | **状态**：🟡 CRUD + Runtime 已通，内置插件 + Callback Chain 待完善
> **需求**：[22 plugin.md](./22%20plugin.md) · **设计**：[22 plugin.design.md](./22%20plugin.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-CB-01

---

## 1. 模块定位

Plugin 插件系统：运行时回调扩展机制，在 Agent 执行过程中插入治理、调试、增强、风控等逻辑。Plugin 与 Skill / Tool 的边界：

- **Skill**：面向 Agent 的能力、知识、脚本和使用规范。
- **Tool**：Agent 可调用的具体外部能力。
- **Plugin**：运行时拦截器 / 中间件，改变或增强 Agent 执行链路。

**代码锚点**：
- `api/kratos/plugin/v1/` — Plugin CRUD RPC（List / ToggleEnabled / UpdateConfig / UpdateSortOrder）
- `internal/service/plugin.go` — PluginService
- `internal/biz/plugin.go` — PluginUsecase + PluginRepo 接口
- `internal/data/plugin.go` — PluginRepo 实现（Ent ORM）
- `internal/data/ent/schema/plugin.go` — PlatformPlugin Ent Schema
- `internal/plugin/trpc/runtime.go` — plugintrpc.Runtime（热重载）
- `internal/plugin/trpc/adapter.go` — biz.Plugin → trpcplugin.Plugin 适配
- `internal/plugin/trpc/audit.go` — AuditLogPlugin 内置插件
- `internal/agent/trpc_runtime.go` — WithPlugins 注入 Runner
- `internal/agent/turn_helpers.go` — Runner 构造时传入 Plugins

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Plugin CRUD | ✅ | List / Get / ToggleEnabled / UpdateConfig / UpdateSortOrder |
| Plugin 运行时 | ✅ | `plugintrpc.Runtime` 热重载 + `Apply()` |
| Plugin 注入 Runner | ✅ | `trpcrunner.WithPlugins(deps.Plugins...)` |
| Plugin 热重载 | ✅ | Service 层写操作后 `reloadRuntime()`（已异步化 safego） |
| 前端管理 | ✅ | Plugin 列表 / 启停 / 配置 / 详情 / 排序 |
| 内置插件注册 | 🟡 | 仅 `audit_log` 一个；需求定义 9 个内置插件 |
| Callback Chain | 🟡 | Tool/Plugin 回调已通；LLMAgent/Model Chain 未挂（EP-CB-01） |
| Pre*/Post* 配对 | 🟡 | R18 红线待统一落地 |

---

## 3. 差距与优化

1. **P1**：内置插件仅实现 `audit_log`，需求定义的 `runtime_audit` / `skill_usage_tracker` / `retry_and_reflect` / `sensitive_data_mask` / `confirmation_guard` / `cost_guard` / `model_router` / `permission_guard` / `output_policy` 共 9 个待逐个实现（EP-CB-01 依赖）。
2. **P1**：Callback Chain 未挂到 LLM Agent / Model lifecycle，Plugin 的 BeforeModel / AfterModel / BeforeAgent / AfterAgent 等回调点尚无法触发（EP-CB-01）。
3. **P2**：Plugin 无沙箱隔离，恶意插件可能影响主进程安全。
4. **P3**：Plugin 无版本管理，更新后无法回滚。
5. **P3**：Plugin 无市场/分享机制。

---

## 4. 开发阶段

- **Phase 1**（当前）：完善内置插件实现 + EP-CB-01 Callback Chain 接入
- **Phase 2**：Plugin 沙箱隔离（进程级隔离）
- **Phase 3**：Plugin 版本管理
- **Phase 4**：Plugin 市场

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | EP-CB-01: Callback Chain 接上 LLMAgent / Model | P1 | EP-CB-01 |
| 2 | 实现 runtime_audit 内置插件（替换 audit_log） | P1 | EP-CB-01 |
| 3 | 实现 sensitive_data_mask 内置插件 | P1 | — |
| 4 | 实现 confirmation_guard 内置插件 | P1 | — |
| 5 | 实现 cost_guard 内置插件 | P2 | — |
| 6 | 实现 model_router 内置插件 | P2 | — |
| 7 | 实现 permission_guard 内置插件 | P2 | — |
| 8 | 实现 output_policy 内置插件 | P2 | — |
| 9 | 实现 skill_usage_tracker 内置插件 | P2 | — |
| 10 | 实现 retry_and_reflect 内置插件 | P2 | — |
| 11 | Plugin 进程隔离方案设计 | P2 | — |
| 12 | Plugin 版本表 + 回滚 API | P3 | — |
| 13 | Plugin 市场前端页面 | P3 | — |

---

## 6. 验收标准

- [x] Plugin CRUD 端到端可用（List / Toggle / Config / SortOrder）
- [x] Plugin 热重载：写操作后 Runner 自动获取最新插件列表
- [x] 前端管理页可用
- [ ] 9 个内置插件全部实现并可注册到 Runner
- [ ] Callback Chain 接上 LLMAgent / Model lifecycle（EP-CB-01）
- [ ] Plugin 运行在独立进程中，崩溃不影响主进程
- [ ] Plugin 可管理多个版本并回滚

---

## 7. 依赖与风险

- EP-CB-01 是内置插件全面实现的前置依赖（BeforeModel / AfterModel 等回调点需先挂上）
- 进程隔离增加通信开销
- Plugin 市场需与 Ecosystem 模块联动
