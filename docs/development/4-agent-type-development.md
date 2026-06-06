# Agent 分类 — 开发计划

> **版本**：2026-05-21 | **状态**：✅ 主路径可用（Platform 资源树）；🟡 拖拽/移动/统计待补
> **需求**：[4.agent-type.md](./4.agent-type.md) · **设计**：[4.agent-type.design.md](./4.agent-type.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Agent 业务画像分类：三级级联（行业→部门→职位）。产品数据经 **Platform 资源树**（`agent-categories`）管理；Agent 通过 `category_position_id` 关联叶子节点。

**代码锚点**：
- `web/src/pages/AgentCategoriesPage.vue` — 分类管理 UI
- `web/src/features/platform/api.ts` — `listPlatformResourceTree` / CRUD
- `api/kratos/agent_category/v1/` — 可选直连 RPC（与 Platform 树并存时需统一真相源）
- `internal/biz/agent_category.go` — 领域逻辑
- `agents.category_position_id` — Agent 绑定

**创建/列表中的级联**：`useAgentsPage` / `AgentsFiltersCard` 使用 Platform 树三级 `q-select`（非独立 `AgentCategoryCascade.vue` 文件）。

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类 CRUD / 树 | ✅ | `agent_category` 服务 + Ent |
| Agent 绑定 | ✅ | `category_position_id` |
| 软删除 / 层级 | ✅ | schema + 应用层校验 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类管理页 | ✅ | `AgentCategoriesPage.vue`（`/settings/agent-categories` 路由以 `frontend-pages.md` 为准） |
| 行业/部门/职位 UI | ✅ | 卡片 + 部门/职位列表 |
| 创建弹窗级联 | ✅ | `useAgentsPage` 三级 select |
| 列表分类筛选 | ✅ | `AgentsFiltersCard` |
| 拖拽排序 | ❌ | 仅 `sort_order` 字段，无拖拽 UI |
| 分类移动 / 环校验 | ❌ | 无 `MoveCategory` 环检测 |
| 删除前 Agent 计数 | ❌ | 无关联统计 API |
| 导入/导出 | ❌ | — |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| CAT-01 | P2 | 删除/移动前查询关联 Agent 数量 |
| CAT-02 | P3 | 拖拽排序持久化 `sort_order` |
| CAT-03 | P3 | 移动父节点循环引用校验 |
| CAT-04 | P3 | 分类导入/导出（多环境） |
| CAT-05 | P3 | Platform 树与 `agent_category` RPC 单一真相源文档化 |

---

## 4. 验收标准

- [x] 管理页可维护三级分类
- [x] 创建 Agent 时可选择职位叶子节点
- [x] 列表可按分类筛选
- [ ] 删除分类前展示关联 Agent 数量
- [ ] 拖拽排序后 `sort_order` 正确

---

## 5. 依赖

| 模块 | 说明 |
|------|------|
| 2 创建 / 3 列表 / 5 设置 | 共用 `category_position_id` |
