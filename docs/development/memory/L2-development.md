# L2 — 开发计划

> **需求**：[`L2.md`](./L2.md) · **设计**：[`L2.design.md`](./L2.design.md)

---

## 现状

| 项 | 状态 |
|----|------|
| messages / traces 事实源 | ✅ |
| memory_episodes 表 | ✅ |
| ListEvents 统一视图 | 🟡 |
| BM25/向量索引 | ❌ |
| LLM 巩固管道 | ❌ |
| ConsolidateEpisode API | ❌ |

---

## 待办

| # | 任务 | 状态 |
|---|------|------|
| L2-1 | ListEvents 跨表视图完善 | 🟡 |
| L2-2 | memory_l2_index 异步建索引 | ❌ |
| L2-3 | ConsolidateEpisode + Worker | ❌ |
| L2-4 | Session 时间线前端 | 🟡 |
| L2-5 | Retention job | ❌ |

---

## 代码锚点

- `internal/data/sessionmemory/store*.go`
- `internal/biz/memory_worker.go`（巩固入队）
- `internal/service/memory.go`
