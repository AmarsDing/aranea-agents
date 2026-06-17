# Agent 分类 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ Taxonomy 系统可用（Platform 资源树 + position_key + 行业上下文注入）；🟡 拖拽/删除统计/变体 UI 待补
> **需求**：[4.agent-type.md](./4.agent-type.md) · **设计**：[4.agent-type.design.md](./4.agent-type.design.md)

---

## 1. 模块定位

Agent 业务画像分类：三级级联（公司→部门→职位）。产品数据经 **Platform 资源树**（Taxonomy 系统）管理；Agent 通过 `position_key` 关联叶子节点，`agent_variant` / `variant_description` 描述职位方向变体。运行时通过 `BuildIndustryContext` 将公司/部门/职位上下文注入系统提示词。

> 需求与功能清单见 `4.agent-type.md`，架构/Proto/数据模型/接口设计见 `4-agent-type.design.md`。本文件只管代码锚点、现状、差距与任务。

---

## 2. 代码锚点

### 2.1 后端

| 文件 | 职责 |
|------|------|
| `api/kratos/taxonomy/v1/taxonomy.proto` | 主 Proto 契约（TaxonomyService，HTTP `/v1/taxonomy`） |
| `api/kratos/industry_taxonomy/v1/industry_taxonomy.proto` | 遗留 Proto（IndustryTaxonomyService，前端 `taxonomy-nodes` 资源仍引用） |
| `internal/service/taxonomy.go` | Service 层（TaxonomyNode ↔ OrganizationNode 转换） |
| `internal/biz/organization.go` | Biz 层（OrganizationNode / OrganizationRepo / OrganizationUsecase） |
| `internal/biz/organization_position_prompt.go` | Biz 层（PositionPromptUsecase：normalizeOrg 层级校验 / GetPositionPrompt / BuildResponsibility） |
| `internal/data/organization_repo.go` | Data 层 Repo 实现 |
| `internal/data/organization_redesign_migrate.go` | 迁移：level `industry` → `company` |
| `internal/data/ent/schema/organization.go` | Ent Schema（表 `organizations`，字段 `org_key`） |
| `internal/data/ent/schema/agent.go` | Agent 表关联字段（`position_key` / `agent_variant` / `variant_description`，唯一索引） |
| `internal/data/agent_repo.go` | `categoryPositionIDsForFilter`（line 455）分类筛选展开子树 |
| `internal/agent/prompt.go` | `BuildIndustryContext`（line 71）行业上下文注入系统提示词 |
| `internal/agent/trpc_build.go` | Agent 构建时引用 PositionKey |

### 2.2 前端

| 文件 | 职责 |
|------|------|
| `web/src/pages/OrganizationPage.vue` | 分类管理 UI（路由 `/settings/taxonomy`，标题「组织架构」） |
| `web/src/components/agents/TaxonomyPicker.vue` | 三级级联选择器（创建/编辑 Agent 时选职位） |
| `web/src/components/agents/TaxonomyFilter.vue` | 列表筛选组件（基于 TaxonomyPicker） |
| `web/src/components/agents/TaxonomyTree.vue` | 树形展示组件 |
| `web/src/components/agents/TaxonomyTreeNodeHeader.vue` | 树节点头部 |
| `web/src/components/agents/TaxonomyNodeHeader.vue` | 节点头部 |
| `web/src/components/agents/TaxonomyIndustryCard.vue` | 公司卡片 |
| `web/src/components/agents/TaxonomyDepartmentNode.vue` | 部门节点 |
| `web/src/components/agents/TaxonomyPositionCard.vue` | 职位卡片 |
| `web/src/components/agents/TaxonomyNodeDialog.vue` | 节点编辑弹窗 |
| `web/src/components/agents/agentUi.ts` | `flattenTaxonomyPositions` 展平职位选项 |
| `web/src/features/platform/api.ts` | `listPlatformResourceTree` / 平台资源 CRUD 分发 |
| `web/src/features/platform/useTaxonomyPage.ts` | Taxonomy 页面 composable |
| `web/src/features/platform/useTaxonomyTreeField.ts` | 树字段 composable |
| `web/src/features/platform/taxonomyTreeUtils.ts` | 树工具函数（flatten/filter/patch） |
| `web/src/features/platform/taxonomyLabels.ts` | 标签映射（level → 1/2/3，描述标签） |

### 2.3 遗留死代码（待清理）

| 文件 | 问题 |
|------|------|
| `web/src/features/agent-categories/api.ts` | 引用不存在的 `agent_category` 服务（`createAgentCategoryService`），属遗留死代码 |
| `web/src/services/index.ts` | 仍导出 `createAgentCategoryService`（line 169-170），引用 `agent_category/v1/index`（line 32） |

**创建/列表中的级联**：`AgentCreateDialog` 使用 `TaxonomyPicker` 选职位；`AgentsFiltersCard` 使用 `TaxonomyFilter` 筛选。

---

## 3. 现状评估

### 3.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类 CRUD / 树 | ✅ | Platform 资源树 + Ent（`agent_category` 独立服务已移除，统一走 `taxonomy` Proto + `organization` Biz） |
| Agent 绑定 | ✅ | `position_key`（agent schema）替代旧 `category_position_id` |
| 职位方向变体 | ✅ | `agent_variant`（默认 `general`）/ `variant_description`（agent schema，唯一索引 position_key+agent_variant） |
| 行业上下文注入 | ✅ | `BuildIndustryContext`（`internal/agent/prompt.go:71`）注入公司/部门/职位到系统提示词 |
| 分类筛选查询 | ✅ | `categoryPositionIDsForFilter`（`internal/data/agent_repo.go:455`）展开子树 |
| 层级校验 | ✅ | `normalizeOrg`（`internal/biz/organization_position_prompt.go:235`）company→department→position |
| 软删除 | ✅ | schema `deleted_at` + 应用层过滤 |
| 同级排序 | ✅ | `ReorderTaxonomy` RPC（PUT `/v1/taxonomy/reorder`） |

### 3.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类管理页 | ✅ | `OrganizationPage.vue`（路由 `/settings/taxonomy`，标题「组织架构」） |
| 公司/部门/职位 UI | ✅ | `TaxonomyTree.vue` + `TaxonomyTreeNodeHeader.vue` + `TaxonomyIndustryCard` / `TaxonomyDepartmentNode` / `TaxonomyPositionCard` |
| 创建弹窗级联 | ✅ | `AgentCreateDialog` 使用 `TaxonomyPicker.vue` |
| 列表分类筛选 | ✅ | `AgentsFiltersCard` 使用 `TaxonomyFilter.vue`（基于 TaxonomyPicker） |
| 职位选项展平 | ✅ | `flattenTaxonomyPositions`（`agentUi.ts`） |
| 树工具函数 | ✅ | `taxonomyTreeUtils.ts`（flatten/filter/patch） |
| 标签映射 | ✅ | `taxonomyLabels.ts`（level → 1/2/3，公司/部门/职位） |
| 拖拽排序 UI | ❌ | 有 `ReorderTaxonomy` RPC，无拖拽 UI |
| 分类移动 / 环校验 | ❌ | 无 `MoveCategory` 环检测 |
| 删除前 Agent 计数 | ❌ | 后端有引用检查，前端无关联统计展示 API |
| `agent_variant` 前端 UI | ❌ | 后端字段就绪，创建/编辑 Agent 时无变体选择 UI |
| 导入/导出 | ❌ | — |
| 遗留死代码清理 | ❌ | `web/src/features/agent-categories/api.ts` + `services/index.ts` 仍引用 `agent_category` |

---

## 4. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| CAT-01 | P2 | 删除/移动前查询关联 Agent 数量（前端展示） |
| CAT-02 | P3 | 拖拽排序持久化 `sort_order`（后端 RPC 已有，前端缺 UI） |
| CAT-03 | P3 | 移动父节点循环引用校验 |
| CAT-04 | P3 | 分类导入/导出（多环境） |
| CAT-05 | ✅ | ~~Platform 树与 `agent_category` RPC 单一真相源文档化~~ 已统一为 Platform 资源树，`agent_category` 独立服务已移除 |
| CAT-06 | P3 | `agent_variant` 前端 UI（创建/编辑 Agent 时选择职位方向变体） |
| CAT-07 | P3 | 清理遗留死代码（`web/src/features/agent-categories/api.ts` + `services/index.ts` 中 `agent_category` 导出） |

---

## 5. Phase 划分与任务清单

### Phase 1：核心可用（✅ 已完成）

| 任务 | 状态 |
|------|------|
| Taxonomy Proto + Service + Biz + Data 全栈实现 | ✅ |
| `organizations` 表 Ent Schema + 迁移（industry→company） | ✅ |
| Agent `position_key` / `agent_variant` / `variant_description` 字段 | ✅ |
| `BuildIndustryContext` 行业上下文注入 | ✅ |
| `OrganizationPage.vue` 管理页 + Taxonomy 组件群 | ✅ |
| `TaxonomyPicker` 三级联动 + `TaxonomyFilter` 列表筛选 | ✅ |
| `ReorderTaxonomy` 同级排序 RPC | ✅ |

### Phase 2：体验补全（🟡 待办）

| 任务 | 状态 | 关联 ID |
|------|------|---------|
| 删除前展示关联 Agent 数量 | ⬜ | CAT-01 |
| 拖拽排序 UI（对接 `ReorderTaxonomy`） | ⬜ | CAT-02 |
| `agent_variant` 前端选择 UI | ⬜ | CAT-06 |

### Phase 3：增强与清理（⬜ 待办）

| 任务 | 状态 | 关联 ID |
|------|------|---------|
| 移动父节点循环引用校验 | ⬜ | CAT-03 |
| 分类导入/导出 | ⬜ | CAT-04 |
| 清理 `agent-categories` 遗留死代码 | ⬜ | CAT-07 |

---

## 6. 验收标准

- [x] 管理页可维护三级分类（OrganizationPage `/settings/taxonomy`）
- [x] 创建 Agent 时可选择职位叶子节点（TaxonomyPicker）
- [x] 列表可按分类筛选（TaxonomyFilter）
- [x] 行业上下文注入系统提示词（BuildIndustryContext）
- [x] 同级排序 RPC 可用（ReorderTaxonomy）
- [ ] 删除分类前展示关联 Agent 数量
- [ ] 拖拽排序 UI 可用
- [ ] `agent_variant` 前端 UI 可选
- [ ] 遗留 `agent-categories` 死代码已清理

---

## 7. 改动文件清单（Phase 2/3 预估）

| Phase | 文件 | 改动类型 |
|-------|------|---------|
| 2 | `web/src/components/agents/TaxonomyNodeDialog.vue` 或新增删除确认弹窗 | 新增关联 Agent 计数展示 |
| 2 | `web/src/components/agents/TaxonomyTree.vue` | 拖拽排序交互 |
| 2 | `web/src/components/agents/AgentCreateDialog.vue` | `agent_variant` 选择 UI |
| 3 | `internal/biz/organization.go` | `MoveOrgNode` + 环检测 |
| 3 | `web/src/features/agent-categories/api.ts` | 删除（死代码清理） |
| 3 | `web/src/services/index.ts` | 移除 `createAgentCategoryService` 导出 |

---

## 8. 依赖

| 模块 | 说明 |
|------|------|
| 2 创建 / 3 列表 / 5 设置 | 共用 `position_key` + `agent_variant` |
| Platform 资源树 | 分类数据统一入口 |

---

*文档版本：开发计划边界；需求见 `4-agent-type.md`，设计见 `4-agent-type.design.md`。*
