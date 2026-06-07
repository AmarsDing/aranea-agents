# L2 — 开发计划

> **需求**：[`L2.md`](./L2.md) · **设计**：[`L2.design.md`](./L2.design.md)
> **进度真相**：以本文为准；需求/设计正文不写修复记录。

---

## 现状（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| Episode 写入 | ✅ | `internal/data/sessionmemory/store_episodes.go`（InsertEpisodeRow，支持 pending/consolidated 状态） |
| 多策略 Recall | ✅ | `internal/data/sessionmemory/store_l2_recall.go`（keyword + vector + importance + session boost 融合） |
| Embedding 索引 | ✅ | `internal/data/sessionmemory/store_episodes.go`（UpsertEpisodeEmbedding） |
| Decay Worker | ✅ | `internal/cronrunner/jobs/memory_l2_decay.go`（定期 importance 衰减 + retention purge） |
| Prompt 注入 | ✅ | `internal/agent/l2_prompt.go`（L2MemoryCue，session 优先策略） |
| 融合召回 | ✅ | `internal/agent/composite_prompt.go`（CompositeMemoryCue，L2+L3 融合） |
| L2 Recall Usecase | ✅ | `internal/biz/memory_l2_recall.go`（业务层编排） |
| Retention 策略 | ✅ | `internal/cronrunner/jobs/memory_l2_decay.go`（L2RetentionDays + PurgeEpisodesOlderThan） |
| ListEvents 基础 | ✅ | `internal/service/event.go` + `internal/biz/event_store.go`（单表 event_store 查询，支持 session/since/until/type/分页） |
| ConsolidateEpisode Worker | ✅ | `internal/cronrunner/jobs/memory_l2_consolidate.go`（pending→consolidated 异步状态机） |
| Episode pending 状态 | ✅ | `internal/data/sessionmemory/store_episodes.go`（ConsolidationStatus 字段） |
| ListEvents 跨表视图 | 🟡 | 当前仅查 event_store 单表，设计要求跨表 UNION ALL 统一视图 |
| 前端时间线 | 🟡 | 事件时间线 UI 待开发 |
| 异步 BM25 索引 | ❌ | BM25 仅在 Knowledge 模块，Memory recall 用简单 token overlap |

---

## 待办

| # | 任务 | 状态 | 优先级 |
|---|------|------|--------|
| L2-1 | ListEvents 跨表视图完善（MemoryL2Repository + 跨表 SQL） | 🟡 | P2 |
| L2-2 | Session 时间线前端 | 🟡 | P2 |
| L2-3 | 异步 BM25 索引 | ❌ | P3 |

---

## 代码锚点

- `internal/data/sessionmemory/store_episodes.go` — Episode 写入 + Embedding
- `internal/data/sessionmemory/store_l2_recall.go` — 多策略 Recall
- `internal/biz/memory_l2_recall.go` — L2 Recall Usecase
- `internal/agent/l2_prompt.go` — L2MemoryCue prompt 注入
- `internal/agent/composite_prompt.go` — CompositeMemoryCue L2+L3 融合
- `internal/cronrunner/jobs/memory_l2_decay.go` — Decay + Retention Worker
- `internal/cronrunner/jobs/memory_l2_consolidate.go` — Consolidate Worker
- `internal/service/event.go` — ListEvents 基础
