# CodeExecutor 文档同步 — 架构图与 Factory 命名

> **日期**：2026-05-21

## 变更

- **`32 codeexecutor.design.md` §2.1**：写入当前实现架构（Wire 单例 Factory、Resolve 链、回退、capabilities）及 **Mermaid 架构图**
- 删除过时的「当前 vs 目标」双架构；`ExecutorRegistry` 统一为 **Factory**（`factory.go`）
- **`32 codeexecutor.md`**：验收 4.3 / 4.4 / 4.7 与后端接入度表更新
- **`32-codeexecutor-development.md`**：代码锚点、差距分析（仅剩余 P2/P3）
- **`execution-plan.md`** 迭代 11：I11-CEX-02/03/04 标记 ✅
- **`README-development.md`** CodeExecutor 接入度列更新

## 架构图位置

权威架构图见 [32 codeexecutor.design.md §2.1](../需求/32%20codeexecutor.design.md#21-当前架构已实现-phase-12--review-修复)（含 Mermaid flowchart）。
