# L3 — 开发计划

> **需求**：[`L3.md`](./L3.md) · **设计**：[`L3.design.md`](./L3.design.md)

---

## 现状

| 项 | 状态 |
|----|------|
| memory_facts 表 + Admin API | ✅ |
| ListMemoryFacts / Upsert | ✅ |
| Legacy trpc_memory → facts backfill | ✅（启动管道；见 [`memory-development.md`](./memory-development.md) §7） |
| 冲突元数据 l4ConflictMeta 复用钩子 | 🟡 |
| pgvector MemoryUsecase | 🟡 可选 |
| 衰减 Job | ❌ |
| rerank | ❌ |
| 冲突仲裁 UI | ❌ |
| 存储三写收敛 | ❌ |

---

## 待办

| # | 任务 | 状态 |
|---|------|------|
| L3-1 | facts 为唯一写路径；adapter 对齐 | ❌ P1 |
| L3-2 | pgvector 降为索引同步 | ❌ P1 |
| L3-3 | 衰减 cron Job | ❌ |
| L3-4 | Conflict List/Resolve API + UI | ❌ |
| L3-5 | Cross-Encoder rerank | ❌ P3 |
| L3-6 | PII 管道 | ❌ P3 |

---

## 代码锚点

- `internal/biz/memory_admin_*.go`
- `internal/biz/memory.go`（pgvector）
- `internal/data/sessionmemory/store*.go`
- `internal/memory/trpc/sqlite_adapter.go`

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


