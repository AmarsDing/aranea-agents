# Agent 分类 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ Taxonomy 系统可用（Platform 资源树 + position_key + 行业上下文注入）；🟡 拖拽/删除统计待补
> **需求**：[4.agent-type.md](./4.agent-type.md) · **设计**：[4.agent-type.design.md](./4.agent-type.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Agent 业务画像分类：三级级联（行业→部门→职位）。产品数据经 **Platform 资源树**（Taxonomy 系统）管理；Agent 通过 `position_key` 关联叶子节点，`agent_variant` / `variant_description` 描述职位方向变体。运行时通过 `BuildIndustryContext` 将行业/部门/职位上下文注入系统提示词。

**代码锚点**：
- `web/src/pages/TaxonomyPage.vue` — 分类管理 UI（路由 `/settings/taxonomy`）
- `web/src/components/agents/TaxonomyPicker.vue` — 级联选择器（创建/编辑 Agent 时选职位）
- `web/src/components/agents/TaxonomyFilter.vue` — 列表筛选组件（基于 TaxonomyPicker）
- `web/src/components/agents/TaxonomyTree.vue` — 树形展示组件
- `web/src/components/agents/TaxonomyTreeNodeHeader.vue` — 树节点头部
- `web/src/features/platform/taxonomyTreeUtils.ts` — 树工具函数（flatten/filter/patch）
- `web/src/features/platform/useTaxonomyPage.ts` — Taxonomy 页面 composable
- `web/src/features/platform/useTaxonomyTreeField.ts` — 树字段 composable
- `web/src/components/agents/agentUi.ts` — `flattenTaxonomyPositions` 展平职位选项
- `web/src/features/platform/api.ts` — `listPlatformResourceTree` / `ListTaxonomyTree` / CRUD
- `internal/agent/prompt.go` — `BuildIndustryContext` 行业上下文注入
- `internal/data/agent_repo.go` — `categoryPositionIDsForFilter` 分类筛选
- `agents.position_key` / `agents.agent_variant` / `agents.variant_description` — Agent 绑定字段

**创建/列表中的级联**：`AgentCreateDialog` 使用 `TaxonomyPicker` 选职位；`AgentsFiltersCard` 使用 `TaxonomyFilter` 筛选。

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类 CRUD / 树 | ✅ | Platform 资源树 + Ent（`agent_category` 独立服务已移除，统一走 Platform） |
| Agent 绑定 | ✅ | `position_key`（proto field 29）替代旧 `category_position_id` |
| 职位方向变体 | ✅ | `agent_variant`（proto field 30）/ `variant_description`（proto field 31） |
| 行业上下文注入 | ✅ | `BuildIndustryContext`（`internal/agent/prompt.go`）注入行业/部门/职位到系统提示词 |
| 分类筛选查询 | ✅ | `categoryPositionIDsForFilter`（`internal/data/agent_repo.go`）展开子树 ID |
| 软删除 / 层级 | ✅ | schema + 应用层校验 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类管理页 | ✅ | `TaxonomyPage.vue`（路由 `/settings/taxonomy`）替代旧 `AgentCategoriesPage` |
| 行业/部门/职位 UI | ✅ | `TaxonomyTree.vue` + `TaxonomyTreeNodeHeader.vue` 树形展示 |
| 创建弹窗级联 | ✅ | `AgentCreateDialog` 使用 `TaxonomyPicker.vue` |
| 列表分类筛选 | ✅ | `AgentsFiltersCard` 使用 `TaxonomyFilter.vue`（基于 TaxonomyPicker） |
| 职位选项展平 | ✅ | `flattenTaxonomyPositions`（`agentUi.ts`） |
| 树工具函数 | ✅ | `taxonomyTreeUtils.ts`（flatten/filter/patch） |
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
| CAT-05 | ✅ | ~~Platform 树与 `agent_category` RPC 单一真相源文档化~~ 已统一为 Platform 资源树，`agent_category` 独立服务已移除 |
| CAT-06 | P3 | `agent_variant` 前端 UI（创建/编辑 Agent 时选择职位方向变体） |

---

## 4. 验收标准

- [x] 管理页可维护三级分类（TaxonomyPage `/settings/taxonomy`）
- [x] 创建 Agent 时可选择职位叶子节点（TaxonomyPicker）
- [x] 列表可按分类筛选（TaxonomyFilter）
- [x] 行业上下文注入系统提示词（BuildIndustryContext）
- [ ] 删除分类前展示关联 Agent 数量
- [ ] 拖拽排序后 `sort_order` 正确
- [ ] `agent_variant` 前端 UI 可选

---

## 5. 依赖

| 模块 | 说明 |
|------|------|
| 2 创建 / 3 列表 / 5 设置 | 共用 `position_key` + `agent_variant` |
