# Agent 列表 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ 主路径 + 运行态 + 复制 + 分类体系 + 三组布局 + 拖拽排序；🟡 BatchUpdate/Reorder 待补 proto RPC
> **需求**：[3 agent-list.md](./3-agent-list.md) · **设计**：[3-agent-list.design.md](./3-agent-list.design.md)
> **系统待办**：[0-system.development.md](./0-system.development.md) §8.11 AGT-07

---

## 1. 模块定位

Agent 管理列表页：展示所有 Agent，支持搜索、筛选、排序、分页，以及快捷操作（编辑、删除、收藏、复制）。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — `ListAgents` / `DeleteAgent` / `ToggleFavorite` / `DuplicateAgent` / `ListAgentCreators`
- `internal/service/agent.go` — `AgentService.ListAgents` / `DeleteAgent` / `ToggleFavorite` / `DuplicateAgent` / `ListAgentCreators`
- `internal/biz/agent_usecase.go` — `AgentUsecase.List` / `Delete` / `ToggleFavorite` / `BatchUpdateAgents` / `ReorderAgents`
- `internal/biz/agent_duplicate.go` — `AgentUsecase.Duplicate`（复制逻辑）
- `internal/biz/agent_kind.go` — agent kind 归一化（builtin/preset/user）
- `internal/biz/agent_context.go` — `ResolveListCreatedByFilter`（`mine` / 用户 id 解析）
- `internal/biz/agent_list_extras.go` — `AgentListExtras`（运行态富化字段）
- `internal/data/agent_repo.go` — `agentRepo.SearchAgents` / `DeleteAgent` / `ListExtrasForAgents` / `ListAgentCreators` / `ReorderAgents`
- `internal/data/ent/schema/agent.go` — `agents` 表 Schema
- `web/src/pages/AgentsPage.vue` — 页面壳
- `web/src/features/agents/useAgentsPage.ts` — 列表/筛选/创建组合逻辑
- `web/src/features/agents/api.ts` — `listAgentsPaged` / `deleteAgent` / `toggleAgentFavorite` / `duplicateAgent` / `listAgentCreators`
- `web/src/features/agents/types.ts` — `Agent` / `AgentListQuery` / `AgentListResult` 类型
- `web/src/stores/agents/index.ts` — Pinia Store `useAgentsPageStore`（列表状态、筛选、分页、CRUD）
- `web/src/components/agents/AgentsListSection.vue` — 网格/表格/空态/骨架屏/三组分组/拖拽排序
- `web/src/components/agents/AgentsFiltersCard.vue` — 筛选行
- `web/src/components/agents/AgentsPaginationBar.vue` — 分页栏
- `web/src/components/agents/AgentCard.vue` — 网格卡片
- `web/src/components/agents/KindBadge.vue` — Agent 归属类型徽章
- `web/src/components/agents/TaxonomyFilter.vue` — 分类体系筛选（替代旧 category filter）
- `web/src/components/agents/AgentsWorkspaceHero.vue` — 页头 Hero

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 列表 | ✅ | `ListAgents` RPC + 分页（`agent.proto:478`） |
| 搜索/筛选 | ✅ | `keyword` / `status` / `provider` / `org_node_id` / `created_by`（`mine` 或用户 id）/ `role` / `kind`（`agent_repo.go:509`） |
| 软删除 | ✅ | `deleted_at` + `status=deleted`（`agent_repo.go:744`） |
| 收藏切换 | ✅ | `ToggleFavorite` RPC（`agent.proto:499`） |
| **复制 Agent** | ✅ | `DuplicateAgent` RPC → `POST /v1/agents/{id}/duplicate`；深拷贝 files + `CheckAgentKey`；**副本 `created_by` = 当前用户**（不复用源 Agent） |
| `last_run_status` / `last_run_at` | ✅ | `ListExtrasForAgents`（批量 session + pending 计数）→ proto 24–25；终态 `runtime.status` 由 `persistRunStatus` 保留 |
| `pending_evolution_count` | ✅ | evolution_suggestion `pending` 计数 → proto 26 |
| `created_by` | ✅ | Ent Schema 字段（`internal/data/ent/schema/agent.go:47`，Ent Auto-Migration）；`ResolveListCreatedByFilter`（`mine` / 用户 id） |
| `BatchUpdateAgents` | 🟡 | biz 层已实现（`agent_usecase.go:909`），尚无 proto RPC |
| `agent_variant` / `kind` / `source` | ✅ | proto 字段 28–33；kind 归一化 `agent_kind.go`；Schema enum 字段 |
| `position_key` / `position_id` | ✅ | 分类体系字段；Schema `position_key` + `position_id`（renamed from `taxonomy_position_id`） |
| `ReorderAgents` | 🟡 | biz/data 层 stub（`return nil`，`agent_repo.go:999`） |
| `ListAgentCreators` | ✅ | RPC `GET /v1/agents/creators`；返回创建者列表（含「仅我的」首项） |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 列表页 | ✅ | `AgentsPage.vue` |
| 网格/列表视图 | ✅ | `AgentsListSection` + `viewMode` |
| 搜索防抖 | ✅ | `useAgentsPageStore` watch `keyword` 等触发 `loadAgentList` |
| 类型/Provider/分类筛选 | ✅ | `AgentsFiltersCard`（`selectedStatus` / `selectedProvider` / `selectedTaxonomy` / `selectedCreator`） |
| localStorage 视图 | ✅ | `LS_VIEW` = `agents.viewMode` |
| 空态/加载态 | ✅ | 骨架屏 + `empty-agent-card` |
| 卡片底栏运行态 | ✅ | `formatLastRunContext`（`last_run_status` + 时间） |
| 列表「复制」 | ✅ | `duplicateAgent`；卡片/菜单入口 |
| 「进化中」chip | ✅ | `isAgentEvolving` = `self_evolve && pending_evolution_count > 0` |
| Agent 迁移入口 | 🟡 | `migrationOpen` 对话框占位文案，无导入导出 |
| 批量操作 | ❌ | 无多选 / `BatchUpdateAgents` RPC |
| 创建者筛选 | ✅ | `AgentsFiltersCard`；`GET /v1/agents/creators`（「仅我的」`user_id=mine`）；创建成功后 `resetListFiltersAfterCreate` 清空筛选 |
| KindBadge | ✅ | `KindBadge.vue` — 归属类型徽章（builtin/preset/user） |
| TaxonomyFilter | ✅ | `TaxonomyFilter.vue` — 替代旧 category filter |
| 拖拽排序 | ✅ | `AgentsListSection` 拖拽排序（三组：builtin/preset/user） |
| 三组布局 | ✅ | Built-in / Preset / User 分组展示 |
| Pinia Store | ✅ | `stores/agents/index.ts` `useAgentsPageStore` 管理列表状态（符合 aranea-frontend-guide §3 数据流铁律） |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 | 说明 |
|----|--------|----------|------|
| LIST-01 | P2 | `last_run_status` 聚合 | ✅ 迭代 9 + 审查：批量查询、终态 status 持久化 |
| LIST-02 | P2 | `created_by` 列 + 筛选 | ✅ Ent + 列表筛选 + 创建者下拉 |
| LIST-03 | P3 | `DuplicateAgent` RPC | ✅ 迭代 10 |
| LIST-04 | P3 | 批量启用/停用/删除 | ✅ biz；⏳ proto RPC + 表格多选 |
| LIST-05 | P3 | 虚拟滚动 | 100+ Agent 性能 |
| LIST-06 | P3 | Agent 迁移实现 | 当前仅 `AgentsPage` 占位对话框 |
| LIST-07 | P3 | ReorderAgents 实现 | biz/data stub 已有，需补 proto RPC + 前端对接 + data 层真实实现 |
| LIST-08 | P3 | Debug trace 清理 | 运行态相关 debug 日志待清理 |

---

## 4. 任务清单

| # | 任务 | 层 | 优先级 | 状态 |
|---|------|-----|--------|------|
| 1 | `agents.created_by` + Ent | 后端 | P2 | ✅ |
| 2 | `ListAgents` 增加 `last_run_status` | 后端 | P2 | ✅ |
| 3 | `DuplicateAgent` proto + Usecase | 后端 | P3 | ✅ |
| 4 | `BatchUpdateAgents` | 后端 | P3 | ✅ biz；⏳ proto RPC |
| 5 | 列表展示运行状态 / 创建者筛选 | 前端 | P2 | ✅ |
| 6 | 批量操作 UI | 前端 | P3 | ⏳ |
| 7 | `agent_variant` / `kind` / `source` proto 字段 | 后端 | P2 | ✅ |
| 8 | `position_key` 分类体系字段 | 后端 | P2 | ✅ |
| 9 | KindBadge + TaxonomyFilter 前端组件 | 前端 | P2 | ✅ |
| 10 | 三组布局 + 拖拽排序 | 前端 | P2 | ✅ |
| 11 | `ReorderAgents` proto RPC + 前端对接 | 全栈 | P3 | ⏳ |
| 12 | Debug trace 清理 | 后端 | P3 | ⏳ |

---

## 5. 验收标准

- [x] 列表搜索、筛选、分页、网格/列表切换可用
- [x] 空列表与加载中有合理 UI
- [x] 列表可显示最近一次运行状态（`last_run_status` / `last_run_at`）
- [x] 创建者筛选可正常工作
- [x] 单 Agent 复制可执行
- [x] Agent 归属类型（KindBadge）可正常展示
- [x] 分类体系筛选（TaxonomyFilter）可正常工作
- [x] 三组布局（Built-in / Preset / User）可正常展示
- [x] 拖拽排序可正常执行（前端侧；后端 stub 待实现）
- [ ] 批量操作可执行（若产品确认需要）
- [ ] ReorderAgents RPC 对接完成（biz stub → 真实实现 + proto RPC + 前端持久化）

---

## 6. 依赖与风险

| 依赖模块 | 说明 |
|----------|------|
| 模块2 创建 | 创建成功后 `loadAgentList` 刷新 |
| 模块4 分类 | `position_id` 筛选标签 |
| 模块10 Session | `last_run_status` 数据源 |
| 模块40 Runner | 可选：RunRegistry 状态摘要 |

**风险**：
- 运行状态聚合需索引 `sessions.agent_id` + `updated_at`；批量删除需软删关联 settings/files。
- **多租户**：当前可按任意 `created_by` 用户 id 筛选全库 Agent；M2 需 workspace 范围 + RBAC。
- **历史数据**：迁移前 `created_by=''` 的 Agent 不出现在创建者下拉，且无法按创建者筛中。
- **ReorderAgents**：biz/data 层为 stub（`return nil`），前端拖拽仅本地生效，刷新后顺序丢失；需补 data 层持久化 + proto RPC。
