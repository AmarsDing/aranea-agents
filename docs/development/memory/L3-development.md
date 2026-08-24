# L3 — 开发计划

> **需求**：[`L3.md`](./L3.md) · **设计**：[`L3.design.md`](./L3.design.md)
> **进度真相**：以本文为准；需求/设计正文不写修复记录。

---

## 现状（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| Fact CRUD | ✅ | `internal/data/memory_shim_l3.go`（Upsert/Delete/Clear/List） |
| 融合召回 | ✅ | `internal/biz/memory_l3_fused_recall.go`（跨 scope 融合 agent+workspace+global） |
| Embedding 索引 | ✅ | `internal/data/memory_shim_l3.go` + `internal/data/memory_fact_index_sync.go`（UpsertFactEmbedding） |
| 索引同步 (MEM-OPT-01) | ✅ | `internal/data/memory_fact_index_sync.go`（stale/fresh/disabled 状态机 + reconciler） |
| Legacy 回填 | ✅ | `internal/data/memory_migrate.go`（BackfillLegacyTRPCMemoryEntities） |
| Decay | ✅ | `internal/data/memory_shim_l3.go` + `internal/cronrunner/jobs/memory_l3_decay.go`（per-agent importance 衰减） |
| Cascade 联动 | ✅ | `internal/data/memory_shim_cascade.go`（L4 改名时自动替换 fact） |
| Prompt 注入 | ✅ | `internal/agent/l3_prompt.go`（L3MemoryCue，独立 + 融合两种模式） |
| Dead Letter | ✅ | `internal/data/memory_job_deadletter.go`（失败任务记录） |
| 批量写入 | ✅ | `internal/data/memory_shim_l3.go`（事务写入 facts + episode） |
| Facts 唯一写路径 | ✅ | `internal/data/memory_shim_l3.go`（统一通过 UpsertFactRow 写入，ON CONFLICT 幂等） |
| pgvector 索引同步 | ✅ | `internal/data/memory_fact_index_sync.go` + `internal/cronrunner/jobs/memory_fact_index_reconciler.go`（6h reconciler） |
| 冲突检测 | ✅ | `internal/biz/memory_admin_usecase.go`（DetectFactConflicts 子句级否定匹配 + 指纹去重前置 + IncrementConflictCount） |
| 冲突 API | ✅ | `api/kratos/memory/v1/memory.proto`（ListConflictingFacts RPC） |
| quality_score 5维评分 | ✅ | `internal/data/memory_shim_l3.go`（keyword(0.25) + vector(0.30) + importance(0.20) + recency(0.15) + quality(0.10)） |
| 时间衰减（evergreen 豁免） | ✅ | `internal/data/memory_helpers.go`（`isEvergreenFactKind` 经 `CanonicalizeFactKind`，`preference`/`user_preference` 同为常青） |
| 向量优先召回 | ✅ | `RecallL3Facts`：有 embedding 则 pgvector+FTS+trigram RRF，不因 count≤5000 改走全表扫描 |
| recency / decay 时间轴 | ✅ | recency=`last_used_at`/`updated_at`；decay=`valid_from`/`created_at` |
| 问句词法 | ✅ | 停用词 + FTS 内容词 OR；`may`/`will` 保留为人名 |
| CJK trigram | ✅ | DDL 20261242 + `searchL3Trigram` 第三路 RRF |
| 自适应 minScore | ✅ | `AdaptiveRecallMinScore` = max(配置, top1×0.6)；配置 ≤0 关闭 |
| 即时同槽覆盖 | ✅ | `ImmediateFactWriter.SetSlotGovernor` |
| MMR 多样性重排 | ✅ | `internal/data/memory_mmr.go`（相关性-多样性平衡重排，2026-07-20 Grok 借鉴） |
| pgvector HNSW 索引 | ❌ | 仅有 B-tree 索引，无 HNSW 向量近似索引 |
| Conflict UI | ❌ | 冲突检测前端展示 |
| rerank (Cross-Encoder) | ❌ | P3 |

---

## 待办

| # | 任务 | 状态 | 优先级 |
|---|------|------|--------|
| L3-1 | pgvector HNSW 索引（大数据量下性能优化） | ❌ | P2 |
| L3-2 | Conflict UI（冲突检测前端展示 + 仲裁） | ❌ | P2 |
| L3-3 | rerank（Cross-Encoder 重排序） | ❌ | P3 |

---

## 代码锚点

- `internal/data/memory_shim_l3.go` — Fact CRUD + Embedding 索引 + L3 衰减 + 统一写路径 + 批量写入
- `internal/data/memory_shim_cascade.go` — Cascade 联动
- `internal/data/memory_migrate.go` — Legacy 回填
- `internal/data/memory_fact_index_sync.go` — 索引同步状态机
- `internal/data/memory_job_deadletter.go` — Dead Letter
- `internal/data/memory_shim_l3.go` — quality_score 5维评分
- `internal/biz/memory_l3_fused_recall.go` — 融合召回
- `internal/biz/memory_admin_usecase.go` — 冲突检测
- `internal/biz/memory_pii.go` — PII 检测
- `internal/agent/l3_prompt.go` — L3MemoryCue prompt 注入
- `internal/data/memory_helpers.go` — `isEvergreenFactKind` + `factDecayWithKind`（evergreen 豁免时间衰减，Grok Build 借鉴）
- `internal/data/memory_mmr.go` — `mmrRerankTexts` 多样性重排（Grok Build 借鉴）
- `internal/cronrunner/jobs/memory_l3_decay.go` — Decay Worker
- `internal/cronrunner/jobs/memory_fact_index_reconciler.go` — 索引 Reconciler

---

## 附录：原落地阶段 / 运行时（迁移自分层需求文）

## 11. 落地实施阶段

### Phase 1（基础事实库 + Recall，2 周）

- [ ] §3.2 ~ §3.6 表落库；§3.7 ALTER。
- [ ] `MemoryL3Service.{UpsertFact, Get, List, Update, Delete, RollbackFact}`。
- [ ] EmbeddingService 包装 + 异步 worker。
- [ ] `Recall`（vector + BM25 融合）。
- [ ] `MemoryL0Service.Assemble` 接入 `memory.l3` 段。
- [ ] PIIFilter 简版（正则 + 可配置规则）。
- [ ] §6.2、§6.3 接口。
- [ ] §8.2 知识库管理页。

### Phase 2（反馈 + 衰减 + 冲突，1～2 周）

- [ ] `Feedback` API + 自动 confidence 调整。
- [ ] `RunDecayBatch` worker。
- [ ] `DetectConflicts` + UI 仲裁。
- [ ] §8.5 冲突待办。
- [ ] §8.4 Session 记忆使用 Tab。

### Phase 3（巩固管道联调，依赖 L2 Phase 3）

- [ ] L2 Consolidation Worker 调 `BulkUpsert`。
- [ ] 与 L1 字段升档联调（L1 可标 `升档候选` 字段）。
- [ ] §8.3 Recall 调试器。

### Phase 4（ADK 兼容 + 高级）

- [ ] §5.7 ADK MemoryService HTTP 兼容层。
- [ ] 多 embedding 模型并存（迁移时双写）。
- [ ] 向量索引迁移到 pgvector / Milvus（与 `14 ...md` §15 一致）。
- [ ] PII NER 替换。

---


