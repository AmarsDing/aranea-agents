# Agent 列表 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[3 agent-list.md](./3%20agent-list.md) · **设计**：[3 agent-list.design.md](./3%20agent-list.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 管理列表页：展示所有 Agent，支持搜索、筛选、排序、分页，以及快捷操作（编辑、复制、删除、运行测试）。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — ListAgents RPC
- `internal/service/agent.go` — ListAgents
- `internal/biz/agent_usecase.go` — AgentUsecase.List

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 列表 | ✅ | `ListAgents` RPC + 分页 |
| 搜索/筛选 | ✅ | search / status / agent_type 参数 |
| 排序 | ✅ | sort_order 字段 |
| 软删除 | ✅ | `deleted_at` 字段 |
| 复制 Agent | ✅ | `DuplicateAgent` RPC |

---

## 3. 差距与优化

1. **P2**：列表中"最近运行状态"列需聚合 session 数据，当前前端为静态展示。
2. **P3**：批量操作（批量启用/停用/删除）未实现。

---

## 4. 开发阶段

- **Phase 1**：Agent 列表增加最近运行状态聚合字段
- **Phase 2**：批量操作 API + 前端多选

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Agent 列表响应增加 `last_run_status` 聚合字段 | P2 | — |
| 2 | 批量操作 RPC（BatchUpdateAgents） | P3 | — |

---

## 6. 验收标准

- [ ] Agent 列表可显示最近一次运行状态
- [ ] 批量操作可正常执行

---

## 7. 依赖与风险

- 最近运行状态聚合依赖 Session 表查询，需注意性能（可加缓存）
