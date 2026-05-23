# M54 Hermes Kanban 文档同步（Hermes UI 对照）

> **日期**：2026-05-24  
> **范围**：文档 only；无代码变更

## 摘要

对照 Hermes Agent Kanban（Dashboard `/kanban`、kanban.db、Dispatcher spawn）全面修订 M54 三件套，明确 **已实现 Phase 0–3** 与 **待办 Phase 4–5**（G14 spawn、UI 对标）。

## 变更文件

| 文件 | 说明 |
|------|------|
| `docs/需求/54-hermes-kanban.md` | Hermes Dashboard UI 全量规格；Aranea 入口/组件；US-HK-07–15 |
| `docs/需求/54-hermes-kanban.design.md` | Hermes 参考架构；UI 组件映射表；G14/Phase 5 实现设计 |
| `docs/需求/54-hermes-kanban-development.md` | Phase 4 spawn · Phase 5 UI · Phase 6 M53 收敛 |
| `docs/需求/frontend-pages.md` | Observatory 双 Kanban 说明 |
| `docs/guides/execution-plan.md` | EP-HK-01 增补 HK-FE-05/06 |
| `docs/需求/README-development.md` | M54 索引行 |
| `docs/README.md` | M54 状态摘要 |

## 关键结论

- **不克隆** Hermes 独立 `kanban.db` / CLI；**GraphExecution = Board**
- **后端缺口 P1**：HK-INT-02 真 spawn Worker（当前 Dispatcher 仅写 TaskRun）
- **前端缺口 P1–P2**：依赖 Tab、Toolbar 筛选、Inline Create、Diagnostics、批量操作
