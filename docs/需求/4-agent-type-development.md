# Agent 分类 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[4.agent-type.md](./4.agent-type.md) · **设计**：[4.agent-type.design.md](./4.agent-type.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 业务画像分类：三级级联（行业→部门→职位），数据存储在 `agent_category_nodes` 表，Agent 通过 `category_position_id` 关联叶子节点。

**代码锚点**：
- `api/kratos/agent_category/v1/` — AgentCategory CRUD + Tree
- `internal/service/agent_category.go` — AgentCategoryService
- `internal/biz/agent_category.go` — AgentCategoryUsecase
- `internal/data/agent_category.go` — AgentCategoryRepo
- `internal/data/ent/schema/agent_category.go` — Ent Schema

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类 CRUD | ✅ | Create/Update/Delete/Get/List |
| 树形结构 | ✅ | parent_id + level 字段 |
| Agent 绑定 | ✅ | agents.category_position_id |
| 软删除 | ✅ | deleted_at 字段 |
| 层级校验 | ✅ | level CHECK 约束 + 应用层校验 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 分类管理页面 | 🟡 待验证 | 需确认 `/settings/agent-categories` 页面是否已实现 |
| QTree 树形展示 | 🟡 待验证 | 需确认树形组件是否已实现 |
| 级联选择组件 | 🟡 待验证 | `AgentCategoryCascade.vue` 是否已实现 |
| 拖拽排序 | ❌ 未实现 | 需求 §5.3 提到排序，但无拖拽 UI |
| 分类移动 | ❌ 未实现 | 无 MoveCategory RPC，无循环引用校验 |
| 种子数据 | 🟡 待验证 | 需确认系统预置分类是否已初始化 |

---

## 3. 差距与优化

1. **P3**：分类树排序仅靠 sort_order，无拖拽排序 UI。
2. **P3**：分类移动（改变父节点）时未校验循环引用。
3. **P2**：分类与 Agent 的关联统计缺失（如某职位下有多少 Agent），删除时无法提示影响范围。
4. **P3**：分类树导入/导出功能未设计（多环境同步场景）。
5. **P3**：系统预置分类的种子数据初始化策略未明确。

---

## 4. 开发阶段

- **Phase 1**：分类移动循环引用校验 + Agent 关联统计
- **Phase 2**：拖拽排序 UI + 种子数据初始化
- **Phase 3**：分类导入/导出

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | MoveCategory RPC 增加循环引用校验 | 后端 | P3 | — | 需求 §2.3 |
| 2 | 分类删除前查询关联 Agent 数量 | 后端 | P2 | — | 需求 §4 |
| 3 | 前端拖拽排序组件（QTree + sortable） | 前端 | P3 | — | 需求 §5.3 |
| 4 | 系统预置分类种子数据 SQL | 后端 | P3 | — | 需求 §2.2 |
| 5 | 前端分类管理页面完善（编辑/删除确认） | 前端 | P3 | — | 需求 §5.3 |
| 6 | 分类导入/导出 API + 前端 | 后端+前端 | P3 | — | — |

---

## 6. 验收标准

- [ ] 移动分类不会产生循环引用
- [ ] 拖拽排序后 sort_order 持久化正确
- [ ] 删除分类前可查看关联 Agent 数量
- [ ] 系统预置分类在首次启动时自动初始化
- [ ] `go test ./internal/biz/... -run TestAgentCategory` 通过

---

## 7. 依赖与风险

### 7.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块2 Agent创建 | `AgentCategoryCascade.vue` | 创建弹窗业务分类级联选择 |
| 模块3 Agent列表 | `category_position_id` 筛选 | 列表页业务分类筛选下拉 |
| 模块5 Agent设置 | 分类只读展示 | 设置页业务分类展示/编辑 |

### 7.2 风险

- 循环引用校验需递归查询父链，深度过大时需设上限
- 删除行业级节点时子树级联软删需事务保证
- 种子数据需幂等，重复执行不产生重复行

---

## 8. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| 移动分类产生循环引用 | 400 Bad Request | `CIRCULAR_REFERENCE` | Toast：移动后将产生循环引用 |
| 删除有子节点的分类 | 409 Conflict | `CATEGORY_HAS_CHILDREN` | 确认对话框：先删除子节点 |
| 删除有 Agent 引用的职位 | 409 Conflict | `CATEGORY_HAS_AGENTS` | 展示关联 Agent 数量 + 迁移提示 |
| 同级同名已存在 | 409 Conflict | `CATEGORY_NAME_DUPLICATE` | inline error：名称已存在 |
| 层级校验失败（如部门挂到职位下） | 400 Bad Request | `CATEGORY_LEVEL_INVALID` | Toast：层级关系不合法 |
