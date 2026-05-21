# Event 事件系统 — 实现与文档同步（2026-05-21）

## 摘要

对照 `internal/event`、`internal/biz/event_bus_*`、`internal/server/ws.go` 与前端 Envelope/Monitor 组件，同步 M18 Event 三份文档。核心 Bus/Envelope/WS/StateDelta 已实现；持久化与 Chat 可视化仍待开发。

## 文档与代码主要偏差（已修正）

| 偏差 | 文档旧态 | 代码真相 |
|------|----------|----------|
| Tag 分隔符 | 需求写「分号」 | Envelope `ContainsTag` 为逗号；trpc Event 为 `;` |
| EnvelopeType | 缺 run_status / flow_log / team_summary / knowledge_ingest 等 | `envelope.go` 已定义 |
| EventBusConsumer | 单文件消费 StateDelta+Usage | I5-SYS-03 拆为 buffer / runner / state 三 handler |
| 前端可视化 | 「全无」 | Monitor `RealtimeEvents.vue` 已接入 Events Tab |
| EventTimeline | 计划在 chat/components | 实际为 `components/monitor/EventTimeline.vue` 且**未挂载** |
| WS monitor 通道 | 仅 log | flow_log 不经 log 门控；含 mcp/alert 类型 |
| Channel | 无 knowledge | `knowledge_ingest` → knowledge 通道 |

## 代码优化（本轮）

- `web/src/features/chat/envelope.ts` — 补全 `mcp.session.reconnect` / `alert.notify`
- `web/src/components/monitor/EventTimeline.vue` — 对齐全量 RUNTIME_ENVELOPE_TYPES（原型仍待 O1 合并/删除）
- `internal/biz/domain_event_adapter.go` — 输出缓冲满时 `SysLogWarn(domain_event.adapter.drop)`（O4 部分）

## 文档同步

- `docs/需求/34 event-system.md` — 能力表、验收 §9
- `docs/需求/34 event-system.design.md` — 架构、EnvelopeType、Consumer SRP、WS、前端 §10.4、文件清单
- `docs/需求/34-event-development.md` — 现状评估、§3 优化表、Phase 任务、O1–O7

## 待办（文档已记录）

- P2：event_store 持久化 + 回放 API + 独立 event_persist_handler
- P3：Chat 侧边栏 BranchTree / StateDeltaIndicator / useEventFilter
- P3：合并或删除未挂载 `EventTimeline.vue`（O1）
