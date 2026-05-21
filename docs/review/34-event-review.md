# 34 Event System Review

> **评分**：84 / 100 | **风险等级**：P1  
> **文档**：[34 event-system.design.md](../需求/34%20event-system.design.md) · [34-event-development.md](../需求/34-event-development.md)  
> **代码锚点**：`internal/event/` · `internal/biz/event_store.go` · `internal/biz/event_persist_handler.go` · `internal/biz/event_bus_*`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | EventBus/Envelope/WS/event_store 全部落地；Monitor RealtimeEvents 和 Chat Inspector Dialog 双 Tab 均已实现 |
| 架构一致性 | 23 | 25 | Bus SRP 拆分（buffer/runner/state 三 handler）符合设计；EventProjector 职责独立 |
| 后端实现质量 | 18 | 20 | Bus 接口设计良好；三级背压策略；事件落库路径完整 |
| 前端实现质量 | 14 | 15 | Monitor Events 页可用；Chat Inspector Dialog 双 Tab；实时订阅模型清晰 |
| 测试与验证 | 6 | 10 | Bus/Buffer 单测已有；Consumer 各 handler 覆盖待补 |
| 文档一致性 | 6 | 10 | 开发计划 2026-05-21 已同步；O1–O7 优化表已列出 |

---

## 模块定位

Event System 是 Aranea-Agents 事件驱动架构的核心，负责：
- `EventBus`：统一事件路由（单实例、多 channel、三级背压）
- `EventBuffer`：环形缓冲 + WS 重放同步屏障
- `EventProjector`：trpc `event.Event` → Envelope 投影
- `EventBusConsumer`：Usage、StateDelta、Buffer 三 handler（已 SRP 拆分）
- `EventBusSideConsumers`：Tool/Callback/MessageStore/FlowLog P3 侧效订阅
- `event_store`：Monitor Events 持久化落库

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| EventBus 接口（单实例） | ✅ |
| 31 种 EnvelopeType | ✅ |
| 三级背压（DropOldest/DropNewest/BlockUpTo） | ✅ |
| Reliable 关键事件 | ✅ |
| EventBuffer 环形 + replay 屏障 | ✅ |
| EventBusConsumer SRP 拆分 | ✅ |
| event_store 持久化 | ✅ |
| Monitor RealtimeEvents | ✅ |
| Chat Inspector Dialog 双 Tab | ✅ |
| TraceEmitter (FlowLog) | ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| EVT-P1-01 | EventBusConsumer 各 handler 缺少独立测试，拆分后的回归验证仅靠手动 | 为 buffer/runner/state 三 handler 补单测 |
| EVT-P1-02 | O1–O7 优化项（见 34-event-development.md）中，O3 持久化压缩与 O7 跨会话查询尚未实现 | 规划优先级，纳入下迭代 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| EVT-P2-01 | `SideConsumers` P3 侧效路径（Tool/Callback/MessageStore）若失败无告警机制 | 补 FlowLogger 侧效失败级别日志 |
| EVT-P2-02 | `EnvelopeType` 定义散落在多处，没有统一注册表方便 review | 建议 `envelope_registry.go` 集中定义和注释 |
