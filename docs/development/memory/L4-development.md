# L4 — 开发计划

> **需求**：[`L4.md`](./L4.md) · **设计**：[`L4.design.md`](./L4.design.md)
> **进度真相**：以本文为准；需求/设计正文不写修复记录。

---

## 现状（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| 知识图谱存储 | ✅ | `internal/data/memory_l4.go`（memory_entities + memory_relations） |
| L4GraphUsecase | ✅ | `internal/biz/memory_l4_usecase.go`（WriteFromUserText 英文+中文 regex + RunDecayWithConfig） |
| Cascade Saga | ✅ | `internal/biz/memory_l4_cascade.go`（4 步 Saga + 完整补偿回滚：UpsertEntity 恢复旧名称 + TouchAffected 清理元数据 + ReplaceFacts 还原 + SyncIndex 重标记） |
| Cascade 持久化 | ✅ | `internal/data/memory_shim_cascade.go`（Proposal + Saga Steps） |
| Business Decay | ✅ | `internal/data/memory_l4.go`（指数衰减 + reinforcement + 归档） |
| Decay Worker | ✅ | `internal/cronrunner/jobs/memory_l4_decay.go`（定期衰减） |
| Neighborhood 查询 | ✅ | `internal/data/memory_l4.go`（图遍历） |
| Prompt 注入 | ✅ | `internal/agent/l4_prompt.go`（L4MemoryCue，entity + neighborhood JSON） |
| Evolution Metrics API | ✅ | `internal/service/agent_evolution.go`（tool success rate / retrieval quality） |
| Name Conflict 检测 | ✅ | `internal/biz/memory_l4_usecase.go`（同 scope person 冲突 gate） |
| 中文 regex | ✅ | `internal/biz/memory_l4_usecase.go`（我叫/我的名字是/我是 + 我喜欢/我偏好/我偏爱等） |
| Graph Tab | 🟡 | 前端图谱展示，需 `VITE_MEMORY_GRAPH_TAB=1` feature flag |
| LLM 实体抽取 | ❌ | LLM 提取仅在 L3 consolidation 路径，L4 无 LLM 实体/关系抽取 |
| bi-temporal 边 | ❌ | 无 valid_from/valid_to 时间维度 |
| Evolution 审核 UI | ❌ | 进化提议审批前端 |
| Identity/Strategy 全链路 | ❌ | Agent 身份和策略完整链路 |

---

## 待办

| # | 任务 | 状态 | 优先级 |
|---|------|------|--------|
| L4-1 | LLM 实体/关系抽取 | ❌ | P2 |
| L4-2 | bi-temporal 边（valid_from/valid_to） | ❌ | P2 |
| L4-3 | Evolution 审核 UI 闭环 | ❌ | P2 |
| L4-4 | Graph Tab 生产就绪（移除 feature flag） | 🟡 | P2 |
| L4-5 | Identity/Strategy 全链路 | ❌ | P3 |

---

## 代码锚点

- `internal/biz/memory_l4_usecase.go` — L4GraphUsecase（WriteFromUserText + RunDecayWithConfig + Name Conflict）
- `internal/biz/memory_l4_cascade.go` — L4CascadeUsecase（4 步 Saga + 补偿）
- `internal/biz/memory_l4.go` — CascadeProposalStore/CascadeGraphReader/CascadeFactMutator/CascadeSagaStore/L4EntityWriter 子接口
- `internal/biz/memory_debug_recall.go` — MemoryDebugRecaller/MemoryFactIndexCounter 端口
- `internal/data/memory_l4.go` — 实体/关系存储 + Business Decay + reinforcement + Neighborhood 查询
- `internal/data/memory_shim_cascade.go` — Cascade 持久化（Proposal + Saga Steps）
- `internal/data/memory_l4.go` — Data 层适配器
- `internal/data/memory_debug_recall.go` — Debug Recall 适配器
- `internal/agent/l4_prompt.go` — L4MemoryCue prompt 注入
- `internal/cronrunner/jobs/memory_l4_decay.go` — Decay Worker
- `cmd/admin/wire.go` — provideL4CascadeUsecase + provideMemoryService
