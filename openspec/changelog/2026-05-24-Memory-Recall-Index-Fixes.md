# Memory 系统修复 — L3 向量双写 / Recall 注入 / Auto-memory

**日期**：2026-05-24

## 变更摘要

1. **L3 向量索引统一**：`data.NewMemoryFactIndexSync` 单次 embed 双写 pgvector（可选）与 `memory_facts.embedding_blob`；新增 `UpsertFactEmbedding`（对称 L2 episode）。
2. **Recall 分数修复**：`RecallL2EpisodesScored` / `RecallL3FactsScored` 返回完整 breakdown；`CompositeSearchMemories` 按真实 `Total` 排序。
3. **Inject 路径收敛**：`MemoryL2RecallUsecase` / `MemoryL3RecallUsecase` 接入 `MemorySet` 与 `TRPCBuilderDeps`；移除 `Store.SetEmbedder` 双入口。
4. **Auto-memory 失败策略**：任一 fact upsert 失败则 job 返回 error 触发重试；episode 仅在全部 upsert 成功且 `added > 0` 时创建。
5. **Admin 端口**：`SessionAdminStore` 拆为 L0/L1/L2/L3/L4 子接口；biz 引入 `RecallHit` / `RecallScoreBreakdown`（渐进替换 `[][]byte`）。

## 涉及文件

- `internal/data/memory_fact_index_sync.go`
- `internal/data/sessionmemory/store_fact_embedding.go`
- `internal/data/sessionmemory/store_l2_recall.go`
- `internal/data/sessionmemory/store_l3_recall.go`
- `internal/data/sessionmemory/store_recall_debug.go`
- `internal/biz/memory_l3_recall.go`
- `internal/biz/memory_admin_store.go`
- `internal/agent/memory_inject.go`
- `internal/runtime/memory_set.go`
- `cmd/admin/wire_memory.go`
- `internal/cronrunner/jobs/auto_memory.go`

## 文档

- `docs/需求/memory/memory.design.md` §2.3、§4.2
- `docs/需求/memory/L3.design.md` §3.6 实现注记
- `docs/需求/memory/memory-development.md` §1.2、§2
