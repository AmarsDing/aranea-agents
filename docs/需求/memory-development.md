# Memory 记忆 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 L0-L3 部分实现；❌ L4 未实现
> **需求**：[12-16 memory.md](./12%20memory.md) · **设计**：[12-16 memory.design.md](./12%20memory.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-04

---

## 1. 模块定位

Agent 记忆系统：五层架构（L0 上下文压缩 / L1 工作记忆 / L2 短期记忆 / L3 长期记忆 / L4 知识图谱），为 Agent 提供跨会话的持久化记忆能力。

**代码锚点**：
- `internal/service/session_compress.go` — L0 上下文压缩
- `internal/biz/memory.go` — MemoryUsecase（L1/L2/L3）
- `internal/data/memory.go` — MemoryRepo
- `internal/agent/trpc_build.go` — Memory 装配（L1-L3）
- `pkg/trpc-agent-go/memory/` — trpc-agent-go Memory 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| L0 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| L1 工作记忆（当前会话） | ✅ | trpc `WorkingMemory` |
| L2 短期记忆（最近 N 条） | ✅ | `MemoryUsecase` + Ent 表 |
| L3 长期记忆（向量检索） | ✅ | `MemoryUsecase` + Embedding + 向量搜索 |
| L4 知识图谱 | ❌ | `RuntimeSettings.L4Enabled` 字段存在但无实现 |
| Memory 类型区分 | ✅ | observation / reflection / plan |
| 记忆注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithMemory` |

---

## 3. 差距与优化

1. **P2（EP-BIZ-04）**：L4 知识图谱层完全未实现。`RuntimeSettings` 中有 `L4Enabled` / `L4GraphInjectNeighbors` / `L4GraphMaxNeighbors` / `L4GraphMaxHops` / `L4IdentityInject` / `L4StrategyInject` 字段，但无任何业务逻辑。
2. **P2**：L3 长期记忆的向量搜索质量未优化，无 re-ranking 机制。
3. **P3**：记忆无过期/衰减机制，长期运行后记忆库膨胀。
4. **P3**：记忆无"遗忘"策略，用户无法手动删除特定记忆。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-04）**：L4 知识图谱基础实现（节点/边 CRUD + 图遍历 + 注入）
- **Phase 2**：L3 向量搜索 re-ranking（Cross-Encoder 重排）
- **Phase 3**：记忆衰减与遗忘策略

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | L4 Graph Schema：`memory_graph_node` / `memory_graph_edge` Ent 表 | P2 | EP-BIZ-04 |
| 2 | L4 Graph Usecase：节点/边 CRUD + BFS 遍历 | P2 | EP-BIZ-04 |
| 3 | L4 Graph 注入：`BuildTRPCLLMAgent` 中 `WithGraphMemory` | P2 | EP-BIZ-04 |
| 4 | L3 re-ranking：Cross-Encoder 重排 | P2 | — |
| 5 | 记忆衰减：基于时间的权重递减 | P3 | — |
| 6 | 记忆遗忘：用户手动删除 + 自动过期 | P3 | — |

---

## 6. 验收标准

- [ ] Agent 可使用 L4 知识图谱进行图遍历和邻居注入
- [ ] L3 向量搜索结果经 re-ranking 后相关性提升
- [ ] `go test ./internal/biz/... -run TestMemory` 通过

---

## 7. 依赖与风险

- L4 知识图谱依赖图数据库或 SQLite 图扩展
- re-ranking 需额外 LLM 调用，增加延迟和成本
