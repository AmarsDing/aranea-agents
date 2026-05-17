# Agent 分类 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[4.agent-type.md](./4.agent-type.md) · **设计**：[4.agent-type.design.md](./4.agent-type.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 业务画像分类：三级级联（行业→部门→职位），数据存储在 `agent_categories` 表，Agent 通过 `category_position_id` 关联叶子节点。

**代码锚点**：
- `api/kratos/agent_category/v1/` — AgentCategory CRUD + Tree
- `internal/service/agent_category.go` — AgentCategoryService
- `internal/biz/agent_category.go` — AgentCategoryUsecase
- `internal/data/agent_category.go` — AgentCategoryRepo

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 分类 CRUD | ✅ | Create/Update/Delete/Get/List |
| 树形结构 | ✅ | parent_id + level 字段 |
| Agent 绑定 | ✅ | agents.category_position_id |
| 前端级联选择 | ✅ | AgentCategoryCascade 组件 |

---

## 3. 差距与优化

1. **P3**：分类树排序仅靠 sort_order，无拖拽排序 UI。
2. **P3**：分类移动（改变父节点）时未校验循环引用。

---

## 4. 开发阶段

- **Phase 1**：分类移动循环引用校验
- **Phase 2**：拖拽排序 UI

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | MoveCategory RPC 增加循环引用校验 | P3 | — |
| 2 | 前端拖拽排序组件 | P3 | — |

---

## 6. 验收标准

- [ ] 移动分类不会产生循环引用
- [ ] 拖拽排序后 sort_order 持久化正确

---

## 7. 依赖与风险

无重大依赖。
