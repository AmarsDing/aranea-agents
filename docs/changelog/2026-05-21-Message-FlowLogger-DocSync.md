# 消息机制（51/51a/51b）与 FlowLogger（52）— 文档对齐与优化（2026-05-21）

## 摘要

对照 `internal/event`、`internal/biz/event_bus_*`、`internal/server/ws.go` 与 `web/src/features/chat|monitor`，同步 51 系列与 52 系列文档；修正 Envelope 类型数量、EventBusConsumer 职责、FlowLogger v2 状态等偏差。

## 优化列表（文档 + 代码）

| ID | 类别 | 项 | 动作 | 状态 |
|----|------|-----|------|------|
| O1 | 文档 | EnvelopeType 数量写「25 种」 | 对齐 `envelope.go` **31 种**；补 `run_status` / `flow_log` / `team_summary` / `knowledge_ingest` / `mcp.session.reconnect` / `alert.notify` | ✅ |
| O2 | 文档 | EventBusConsumer「单文件多职责」 | 对齐 I5-SYS-03：`buffer` / `runner` / `state` / `persist` 四 handler + 编排器 | ✅ |
| O3 | 文档 | 52-flow-logger.md 仍引用 v1 `flow_logger.go` / `turn_spans.go` | 改为 v2 已落地锚点（`trace_emitter.go` 等） | ✅ |
| O4 | 文档 | 52-development Phase 1c「进行中」、§4 仍写 Phase 3 待做 | 标记完成；PR 红线勾选已验证项 | ✅ |
| O5 | 文档 | 51b Monitor §7.4 仅示例 `log` | 补充 `flow_log` 免 `enable_log`、进程 `log` 门控 | ✅ |
| O6 | 文档 | `docs/README.md` 缺 51/51a/51b 索引 | §5.2 增加消息机制入口 | ✅ |
| O7 | 文档 | `0-system-development.md` Event 行仍写 Consumer 待拆 | 更新为 handler 已拆、P3 独立消费者待做 | ✅ |
| O8 | 代码 | 全局 WS（`session_id=*`）未默认订阅 `knowledge` | `ws.go` globalMode 增加 `knowledge` 通道 | ✅ |

## 文档与代码主要偏差（已修正）

| 偏差 | 文档旧态 | 代码真相 |
|------|----------|----------|
| Envelope 计数 | 25 种 | 31 种 `EnvelopeType` 常量 |
| EventBusConsumer | 单结构体 OnEnvelope 内联 Buffer/State/Usage | `event_bus_*_handler.go` + `event_persist_handler.go` |
| FlowLogger 状态 | 「设计中」+ v1 文件路径 | v2 Phase 1a/1b/1c/3 ✅；`flow_logger.go` / `turn_spans.go` 已删 |
| Monitor 日志 | 仅 `EnvelopeTypeLog` | `flow_log` 始终下发；`log` 受 `enable_log` 门控 |
| RouteChannel | 方法 `(e Envelope) RouteChannel()` | 包函数 `RouteChannel(env Envelope)`；含 `knowledge` 通道 |

## 待办（仍由 development 计划跟踪）

- Message P2：FTS5 消息搜索
- Message P3：`ToolCallConsumer` / `CallbackConsumer` / `MessageStoreConsumer` 独立订阅
- FlowLogger Phase 2：`ListFlowLogs` HTTP 历史查询 + 落库

## 文档同步文件

- `docs/README.md`
- `docs/需求/51 消息机制.md`
- `docs/需求/51a 后端消息机制.md`
- `docs/需求/51b 前端消息机制.md`
- `docs/需求/message-development.md`
- `docs/需求/52-flow-logger.md`
- `docs/需求/52-flow-logger-development.md`
- `docs/需求/0-system-development.md`
