# L4 — 开发计划

> **需求**：[`L4.md`](./L4.md) · **设计**：[`L4.design.md`](./L4.design.md)

---

## 现状（2026-05-24）

| 项 | 状态 |
|----|------|
| memory_entities/relations 表 | ✅ |
| L4GraphRepo / L4GraphUsecase | ✅ |
| L4MemoryCue prompt 注入 | ✅ |
| WriteFromUserText (regex) | 🟡 MVP |
| AutoMemory → L4 | 🟡 |
| 冲突/衰减元数据 | 🟡 |
| Evolution API proto | ✅ |
| Evolution/Graph 前端 | 🟡 占位 / feature flag |
| 级联 BFS + Proposal | ❌ |
| bi-temporal 边 | ❌ |
| LLM 实体抽取 | ❌ |

---

## 待办

| # | 任务 | 状态 |
|---|------|------|
| L4-1 | LLM 实体/关系抽取 | ❌ P2 |
| L4-2 | CascadeProposal RPC + BFS | ❌ P2 |
| L4-3 | valid_from/valid_to 迁移 | ❌ P2 |
| L4-4 | Evolution 审核 UI 闭环 | ❌ |
| L4-5 | Graph Tab 生产就绪 | 🟡 |
| L4-6 | Identity/Strategy 全链路 | ❌ |

---

## 代码锚点

- `internal/biz/memory_l4_usecase.go` · `memory_l4*.go`
- `internal/data/memory_l4.go`
- `internal/agent/l4_prompt.go`
- `internal/cronrunner/jobs/auto_memory.go`

---

## Phase 1 剩余（来自总计划）

- ❌ Identity/Strategy/Evolution 全链路审核 UI
- ❌ GraphRAG 深度检索
