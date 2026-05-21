# Chat 会话事件检视 P3 — 实现摘要（2026-05-21）

## 摘要

按设计 §12 实现 Chat **Dialog 双 Tab**（非第四列侧边栏）：扩展 `SessionTimelineDialog`，新增 Envelope Tab 与 Inspector 组件；删除未挂载 `monitor/EventTimeline.vue`。

## 前端交付

- `features/event/api.ts` — `GET /v1/events`
- `features/chat/eventFilter.ts` + composables
- `components/chat/` — EventFilterBar / BranchTree / StateDeltaIndicator / TransferBadge / SessionEventInspectorPanel
- `SessionTimelineDialog` — Tab「历史 Trace | 实时 Envelope」
- `ChatMessagePanel` — 头部「事件」按钮

## 文档同步

- `34 event-system.md` §2.9 Chat 检视需求
- `34 event-system.design.md` §12 架构
- `34-event-development.md` Phase 2 任务
