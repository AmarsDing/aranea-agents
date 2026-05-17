# Graph 工作流 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础 CRUD 可用；❌ 执行引擎未实现
> **需求**：[36 graph-workflow.md](./36%20graph-workflow.md) · **设计**：[36 graph-workflow.design.md](./36%20graph-workflow.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-10

---

## 1. 模块定位

Graph 工作流：构建确定性工作流引擎，支持有向图定义、条件路由、状态管理、人工审批和检查点恢复。

**代码锚点**：
- `api/kratos/graph/v1/` — GraphDefinition CRUD RPC
- `internal/service/graph.go` — GraphService
- `internal/biz/graph.go` — GraphUsecase
- `internal/data/graph.go` — GraphRepo
- `pkg/trpc-agent-go/graph/` — trpc-agent-go Graph 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| GraphDefinition CRUD | ✅ | Create/Update/Delete/Get/List |
| 图结构存储 | ✅ | `nodes_json` / `edges_json` |
| 图执行引擎 | ❌ | 无 DAG 执行器 |
| 条件路由 | ❌ | 无条件边执行 |
| 状态管理 | ❌ | 无 State Schema + Reducer |
| 人工审批节点 | ❌ | 无 HITL 中断/恢复 |
| 检查点/回放 | ❌ | 无 Checkpoint + TimeTravel |
| 前端画布 | ❌ | 无可视化编辑器 |

---

## 3. 差距与优化

1. **P1（EP-BIZ-10）**：图执行引擎未实现，Graph 仅存储不执行。这是 Graph 工作流的核心功能缺失。
2. **P2**：条件路由未实现，无法基于状态动态选择执行路径。
3. **P2**：状态管理未实现，节点间无法共享状态。
4. **P2**：人工审批节点未实现，无法在流程中插入人工审核。
5. **P3**：检查点/回放未实现，无法恢复中断的流程。
6. **P3**：前端画布编辑器未实现，无法可视化设计工作流。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-10）**：图执行引擎（DAG 拓扑排序 + 并行执行 + Agent 节点）
- **Phase 2**：条件路由 + 状态管理（State Schema + Reducer）
- **Phase 3**：人工审批节点（HITL 中断/恢复）
- **Phase 4**：检查点/回放 + 前端画布

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `internal/graph/engine.go`：DAG 拓扑排序 + 并行执行 | P1 | EP-BIZ-10 |
| 2 | Agent 节点执行器（调用 Chat turn） | P1 | EP-BIZ-10 |
| 3 | 条件路由：基于状态的条件边 | P2 | — |
| 4 | State Schema + Reducer | P2 | — |
| 5 | HITL 中断/恢复机制 | P2 | — |
| 6 | Checkpoint + TimeTravel | P3 | — |
| 7 | 前端画布编辑器（React Flow） | P3 | — |
| 8 | Wire 注入 Engine 到启动流程 | P1 | EP-BIZ-10 |

---

## 6. 验收标准

- [ ] Graph 创建后可执行，节点按拓扑顺序运行
- [ ] 条件路由根据状态动态选择路径
- [ ] 人工审批节点可暂停/恢复流程
- [ ] `go test ./internal/graph/...` 通过

---

## 7. 依赖与风险

- Graph 执行引擎与 Chat/Team 对话流程紧耦合
- 前端画布编辑器开发量大
- 检查点恢复需考虑状态一致性
