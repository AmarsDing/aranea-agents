# Memory 文档整合（2026-05-24）

## 变更

将分散的 12–16 / 38 Memory 文档整合至 **`docs/需求/memory/`**，按 **需求 / 设计 / 开发计划** 三分法 + **总 / L0–L4** 分层。

| 新路径 | 类型 |
|--------|------|
| `memory/README.md` | 索引与文档边界 |
| `memory/memory.md` | 总需求（含 Memory Center UX） |
| `memory/memory.design.md` | 总设计（含目标架构：Ledger/Views/Policy、存储收敛、级联 bi-temporal） |
| `memory/memory-development.md` | 总开发计划 |
| `memory/theory.md` | 理论体系（原 `38 memory.md`） |
| `memory/L0.md` … `L4-development.md` | 分层三件套 × 5 |

原 `12-16 memory*.md`、`12–16 memory-L*.md`、`38 memory.md` **已删除**；正文已迁入 `memory/`（含附录 A 保留原 design 全文）。Git 历史可查旧版。

Review 重命名：`review/12-16-memory-review.md` → [`review/memory-review.md`](../review/memory-review.md)。

## 设计定稿要点（写入 memory.design.md §二）

1. 单一 Ledger，L1–L4 为 Derived Views
2. L3 权威在 SQLite `memory_facts`；pgvector 降为可选索引
3. Memory-Agent = 异步 Consolidator，非第六层
4. L4 级联 + valid_from/valid_to + CascadeProposal 审核

## 索引更新

- `docs/README.md` §5.2
- `docs/需求/README-development.md`
- `docs/review/memory-review.md`（原 `12-16-memory-review.md`）

## 代码

本次 **仅文档**；无行为变更。
