# 域 C 知识库 RAG 评测结果（2026-08-21 重评）

> 对照：2026-08-18 基线见 [result-run1-20260818.md](./result-run1-20260818.md)
> 被测：`aranea-runtime:local` 由 `902fb5f1c`（2026-08-21 23:17）交叉编译后重建；HTTP :8810
> Collection：eval-ops-kb（team，bge-m3，**API 建库 dim=1024，不再 SQL 改维**）
> 埋入：5 篇 sample-doc-*.md → **12 chunks**（上次 29；markdown 切块变粗，top-5 选择性下降，见口径）

## 检索准确率（top-5 命中出处文档）

| 文档组 | 2026-08-18 | 2026-08-21 |
|------|------|------|
| ins / chg / emg / dty / sec | 6/6 ×5 | 6/6 ×5 |
| **合计 hybrid=auto** | **30/30** | **30/30** |

## C-12 混合检索对比

| 模式 | 2026-08-18 命中 | 2026-08-21 命中 | 2026-08-21 均延迟 |
|------|------|------|------|
| dense | 30/30 (100%) | 30/30 (100%) | 2405ms |
| rrf | 30/30 (100%) | 30/30 (100%) | 1987ms |
| sparse 词法 | **12/30 (40%)** | **30/30 (100%)** | 2647ms |

**结论**：中文词法从系统级短板拉到与 dense 持平（lexical_query 扩写 + CJK bigram + recency）。延迟仍 ~2s，RetrievalEvaluator 默认仍开，与 08-17 性能报告同一瓶颈。

口径：本轮仅 12 chunks，top-5 覆盖约 42% 语料，比 29 chunks 时更易命中。sparse 从 40%→100% 仍是方向性胜利，不宜直接当「词法已达万级库 SOTA」。

## 建库 dim（上次 BUG-C-01）

探针 `CreateCollection embedding_model=bge-m3` 返回 **dim=1024**。P0-2 已在线上生效。run.ps1 的 SQL `UPDATE dim=1024` 变为幂等空操作。

## 延迟（hybrid=auto，n=30）

- min=1896ms max=4093ms p95=3528ms（上次 p95=2222ms，评估 LLM 仍在 Search API 主路径）
