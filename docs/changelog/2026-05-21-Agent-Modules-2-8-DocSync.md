# Agent 模块 2–8 文档与实现对齐（2026-05-21）

## 摘要

对照 `docs/README.md` 文档组织规则，以 **`*-development.md` 为进度真相源**，核对模块 2–8 代码实现，修正过时评估（如列表 `DuplicateAgent`、设置页「待验证」、进化指标「静态零值」等），并在 `0-system-development.md` §8.11 汇总待优化项。

**未改动的文档类型**（保持纯需求/设计边界）：`*.md` 需求正文、`*.design.md` 设计正文。

## 更新的开发计划

| 文档 | 主要修订 |
|------|----------|
| [2-agents-create-development.md](../需求/2-agents-create-development.md) | 维持 2026-05-21 实现状态 |
| [3-agent-list-development.md](../需求/3-agent-list-development.md) | 前端 ✅；移除错误的 `DuplicateAgent`；明确 context_window ≠ last_run_status |
| [4-agent-type-development.md](../需求/4-agent-type-development.md) | Platform 树 + `AgentCategoriesPage`；级联在 create 页 |
| [5-agent-setting-development.md](../需求/5-agent-setting-development.md) | 各 Tab/高级对话框 ✅ |
| [6-agent-setting-file-development.md](../需求/6-agent-setting-file-development.md) | CRUD/默认文件 ✅；AI 编辑占位 |
| [7-agent-evolution-development.md](../需求/7-agent-evolution-development.md) | `evolution_metrics_repo` 真实聚合 ✅；Scanner ❌ |
| [8-agent-title-development.md](../需求/8-agent-title-development.md) | `AgentSettingsHeader`；高级对话框 ✅ |

## 系统级

- [0-system-development.md](../需求/0-system-development.md) §8.11：AGT-01～AGT-16 矩阵 + 模块索引链接
- [README-development.md](../需求/README-development.md) 进度快照
- [execution-plan.md](../guides/execution-plan.md) I8-DOC-02 文档同步项

## 后续迭代（2026-05-21 起）

迭代 9–10 与审查修复已完成，见 [Iteration9](./2026-05-21-Agent-Iteration9.md)、[Iteration10](./2026-05-21-Agent-Iteration10.md)。LIST-02（`created_by` / 模板全字段 / 结构化创建错误）见 [CreatedBy-Templates-Errors](./2026-05-21-Agent-CreatedBy-Templates-Errors.md)。**当前 P2/P3 建议**：

1. LIST-04 批量操作、LIST-06 迁移实现  
2. AGT-16 进化趋势图、AGT-15 `GenerateAgentTitle`  
3. 设置页壳 &lt;300 行（dialogs 外提）、Scanner TTL  
