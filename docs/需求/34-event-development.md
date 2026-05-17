# Event 事件系统 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[34 event-system.md](./34%20event-system.md) · **设计**：[34 event-system.design.md](./34%20event-system.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Event 事件系统：基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。

**代码锚点**：
- `internal/event/bus.go` — EventBus
- `internal/runtime/deps.go` — EventPipeline
- `internal/service/chat.go` — 事件订阅

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| EventBus | ✅ | 发布/订阅模式 |
| EventPipeline | ✅ | `rt.EventPipeline{Bus: deps.EventBus}` |
| 事件类型 | ✅ | MessageCreated / TurnCompleted 等 |
| SSE 事件推送 | ✅ | Chat SSE 事件流 |
| WS 事件推送 | ✅ | WebSocket 事件流 |
| 事件持久化 | ❌ | 无事件存储 |
| 事件回放 | ❌ | 无事件回放机制 |

---

## 3. 差距与优化

1. **P2**：无事件持久化，系统重启后事件丢失。
2. **P3**：无事件回放机制，无法重现历史事件序列。

---

## 4. 开发阶段

- **Phase 1**：事件持久化（SQLite 存储）
- **Phase 2**：事件回放机制

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `event_store` Ent 表 + 事件写入 | P2 | — |
| 2 | 事件回放 API | P3 | — |

---

## 6. 验收标准

- [ ] 系统重启后可查询历史事件
- [ ] 可按时间范围回放事件

---

## 7. 依赖与风险

- 事件持久化需注意存储膨胀（可加 TTL 清理）
