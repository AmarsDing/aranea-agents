# 51 Message / WebSocket / Envelope Review

> **评分**：85 / 100 | **风险等级**：P1  
> **文档**：[51 消息机制.md](../需求/51%20消息机制.md) · [51a 后端消息机制.md](../需求/51a%20后端消息机制.md) · [51b 前端消息机制.md](../需求/51b%20前端消息机制.md) · [message-development.md](../需求/message-development.md)  
> **代码锚点**：`internal/event/` · `internal/server/ws.go` · `web/src/features/chat/ws-transport.ts` · `web/src/features/chat/useEnvelopeStream.ts`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | 原 8 个已知问题全部解决；31 种 EnvelopeType 覆盖 Chat/Team/Monitor/Graph/System；背压三级策略完整 |
| 架构一致性 | 23 | 25 | 单 WS 连接多路复用 ✅；EventBus 替代所有独立 Broker ✅；EventProjector 清晰隔离；SSE 仅外部协议 ✅ |
| 后端实现质量 | 18 | 20 | Bus/Buffer/Consumer 三者 SRP 明确；TraceEmitter 独立；事件落库 + FlowLog 侧消费者已完成 |
| 前端实现质量 | 14 | 15 | `ws-transport.ts` 心跳/重连/控制消息完善；`useEnvelopeStream.ts` 订阅模型清晰；`useChatWorkspace` 整合层偏重 |
| 测试与验证 | 7 | 10 | Bus/Buffer/Projector 均有单测；WS 协议层集成测试缺失 |
| 文档一致性 | 6 | 10 | 需求与开发计划对齐良好；文档开发计划命名为 `message-development.md`（缺少模块编号 51 前缀）；`trpc Review P1–P3` changelog 已同步 |

---

## 架构总览

```
trpc event.Event
    ↓
EventProjector.ProjectAndPublish
    ↓ Envelope (type + channel + payload)
EventBus.Publish(sessionID, envelope)
    ├─ EventBuffer (环形缓冲 + WS replay 同步屏障)
    ├─ EventBusConsumer (buffer/runner/state 三 handler)
    └─ WSServer (channel 过滤 → 客户端推送)
```

### Envelope 频道模型

| channel | 订阅者 |
|---------|-------|
| `chat:{session_id}` | ChatPage, SessionDetailPage |
| `team:{session_id}` | ChatPage (Team 模式) |
| `monitor` | MonitorPage |
| `graph:{exec_id}` | GraphRunPage |
| `system` | 全局通知 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| WS 单连接多路复用 | ✅ |
| 31 种 EnvelopeType | ✅ |
| 背压三级策略（DropOldest/DropNewest/BlockUpTo） | ✅ |
| Reliable 关键事件保障 | ✅ |
| 事件缓冲 + WS 重放同步屏障 | ✅ |
| WS 上行（cancel/user_message/enqueue_message） | ✅ |
| 动态订阅（subscribe/unsubscribe/enable_log） | ✅ |
| FlowLog v2（TraceEmitter → EnvelopeTypeFlowLog） | ✅ |
| SlogBridge 已移除 | ✅ |
| 独立 Chat SSE / TeamRunEventBroker 已删除 | ✅ |
| EventBusConsumer 三拆 | ✅ |
| Prometheus 丢弃计数 | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| MSG-P1-01 | WS 协议层无集成测试（连接握手 → Envelope 投递 → 断连重放） | 补充 WS 集成测试套件 |
| MSG-P1-02 | FlowLogger Phase 2（落库 + HTTP 历史查询 `ListFlowLogs`）未实现 | 见 [52-flowlogger-review.md](./52-flowlogger-review.md) |
| MSG-P1-03 | `message-development.md` 缺少模块编号前缀 `51-`，不符合文档命名规范 | 重命名或在 README-development.md 中标注 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| MSG-P2-01 | Graph channel 的 `graph:{exec_id}` 订阅路径需确认与 GraphService 的 Envelope 产出对齐 | 补 Graph 频道集成测试 |
| MSG-P2-02 | A2A / MCP 的外部协议 SSE 路径与内部 WS 路径的边界文档需更新 | 在 51a/51b 设计文档中明确标注 SSE-only 路径 |

---

## 前端 WS 分层

```
ws-transport.ts       — 纯传输层（心跳/重连/控制消息）
useEnvelopeStream.ts  — Envelope 解析与按类型分发
useChatStream.ts      — Chat 场景订阅（消息流/tool/状态）
useTeamStream.ts      — Team 场景成员流
```

**状态**：层次清晰；`useChatWorkspace.ts` 混入了过多编排逻辑，与 WS 层耦合偏重。

---

## 建议优化路径

1. 补 WS 协议层集成测试（优先）。
2. 实现 FlowLogger Phase 2 落库（见 FlowLogger review）。
3. `message-development.md` 添加 `51-` 编号前缀（文档规范）。
