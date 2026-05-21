# Agent 列表 — 开发计划

> **版本**：2026-05-21 | **状态**：✅ 主路径 + 运行态 + 复制 + LIST-02；🟡 批量 / 迁移待补
> **变更记录**：[changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md)
> **需求**：[3 agent-list.md](./3%20agent-list.md) · **设计**：[3 agent-list.design.md](./3%20agent-list.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **系统待办**：[0-system-development.md](./0-system-development.md) §8.11 AGT-07

---

## 1. 模块定位

Agent 管理列表页：展示所有 Agent，支持搜索、筛选、排序、分页，以及快捷操作（编辑、删除、收藏）。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — `ListAgents` / `ToggleFavorite` / `DeleteAgent`
- `internal/service/agent.go` — `AgentService`
- `internal/biz/agent_usecase.go` — `AgentUsecase.List`
- `web/src/pages/AgentsPage.vue` — 页面壳
- `web/src/features/agents/useAgentsPage.ts` — 列表/筛选/创建组合逻辑
- `web/src/components/agents/AgentsListSection.vue` — 网格/表格/空态/骨架屏

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 列表 | ✅ | `ListAgents` RPC + 分页 |
| 搜索/筛选 | ✅ | `keyword` / `status` / `provider` / `category_id` / `created_by`（`mine` 或用户 id） |
| 软删除 | ✅ | `deleted_at` |
| 收藏切换 | ✅ | `ToggleFavorite` |
| **复制 Agent** | ✅ | `DuplicateAgent` → `POST /v1/agents/{id}/duplicate`；深拷贝 files + `CheckAgentKey`；**副本 `created_by` = 当前用户**（不复用源 Agent） |
| `last_run_status` / `last_run_at` | ✅ | `ListExtrasForAgents`（批量 session + pending 计数）→ proto 24–25；终态 `runtime.status` 由 `persistRunStatus` 保留 |
| `pending_evolution_count` | ✅ | evolution_suggestion `pending` 计数 → proto 26 |
| `created_by` | ✅ | Ent + 迁移 `docs/sql/02_agent_created_by.sql`（含 `idx_agents_created_by`）；`ResolveListCreatedByFilter`（`mine` / 用户 id） |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 列表页 | ✅ | `AgentsPage.vue` |
| 网格/列表视图 | ✅ | `AgentsListSection` + `viewMode` |
| 搜索防抖 | ✅ | `useAgentsPage` watch `keyword` 等触发 `loadAgentList` |
| 类型/Provider/分类筛选 | ✅ | `AgentsFiltersCard` |
| localStorage 视图 | ✅ | `LS_VIEW` = `agents.viewMode` |
| 空态/加载态 | ✅ | 骨架屏 + `empty-agent-card` |
| 卡片底栏运行态 | ✅ | `formatLastRunContext`（`last_run_status` + 时间） |
| 列表「复制」 | ✅ | `duplicateAgent`；卡片/菜单入口（迭代 10） |
| 「进化中」chip | ✅ | `isAgentEvolving` = `self_evolve && pending_evolution_count > 0` |
| Agent 迁移入口 | 🟡 | 对话框占位文案，无导入导出 |
| 批量操作 | ❌ | 无多选 / `BatchUpdateAgents` |
| 创建者筛选 | ✅ | `AgentsFiltersCard`；`GET /v1/agents/creators`（「仅我的」`user_id=mine`）；创建成功后 `resetListFiltersAfterCreate` 清空筛选 |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 | 说明 |
|----|--------|----------|------|
| LIST-01 | P2 | `last_run_status` 聚合 | ✅ 迭代 9 + 审查：批量查询、终态 status 持久化 |
| LIST-02 | P2 | `created_by` 列 + 筛选 | ✅ Ent + 列表筛选 + 创建者下拉 |
| LIST-03 | P3 | `DuplicateAgent` RPC | ✅ 迭代 10 |
| LIST-04 | P3 | 批量启用/停用/删除 | `BatchUpdateAgents` + 表格多选 |
| LIST-05 | P3 | 虚拟滚动 | 100+ Agent 性能 |
| LIST-06 | P3 | Agent 迁移实现 | 当前仅 `AgentsPage` 占位对话框 |

---

## 4. 任务清单

| # | 任务 | 层 | 优先级 | 状态 |
|---|------|-----|--------|------|
| 1 | `agents.created_by` + Ent | 后端 | P2 | ✅ |
| 2 | `ListAgents` 增加 `last_run_status` | 后端 | P2 | ✅ |
| 3 | `DuplicateAgent` proto + Usecase | 后端 | P3 | ✅ |
| 4 | `BatchUpdateAgents` | 后端 | P3 | ⏳ |
| 5 | 列表展示运行状态 / 创建者筛选 | 前端 | P2 | ✅ |
| 6 | 批量操作 UI | 前端 | P3 | ⏳ |

---

## 5. 验收标准

- [x] 列表搜索、筛选、分页、网格/列表切换可用
- [x] 空列表与加载中有合理 UI
- [x] 列表可显示最近一次运行状态（`last_run_status` / `last_run_at`）
- [x] 创建者筛选可正常工作
- [x] 单 Agent 复制可执行
- [ ] 批量操作可执行（若产品确认需要）

---

## 6. 依赖与风险

| 依赖模块 | 说明 |
|----------|------|
| 模块2 创建 | 创建成功后 `loadAgentList` 刷新 |
| 模块4 分类 | `category_position_id` 筛选标签 |
| 模块10 Session | `last_run_status` 数据源 |
| 模块40 Runner | 可选：RunRegistry 状态摘要 |

**风险**：
- 运行状态聚合需索引 `sessions.agent_id` + `updated_at`；批量删除需软删关联 settings/files。
- **多租户**：当前可按任意 `created_by` 用户 id 筛选全库 Agent；M2 需 workspace 范围 + RBAC。
- **历史数据**：迁移前 `created_by=''` 的 Agent 不出现在创建者下拉，且无法按创建者筛中。
