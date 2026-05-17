# Message 消息 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：消息机制 · **设计**：消息设计
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Message 消息：统一的消息模型和传输机制，支持 SSE、WebSocket 双通道消息推送。

**代码锚点**：
- `internal/server/ws.go` — WebSocket 服务
- `internal/service/chat.go` — SSE 消息推送
- `internal/event/bus.go` — 事件总线

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| SSE 消息推送 | ✅ | Chat SSE 事件流 |
| WS 消息推送 | ✅ | WebSocket 事件流 |
| 消息持久化 | ✅ | Session Message 表 |
| 消息类型 | ✅ | text / tool_call / tool_result / error |
| 消息搜索 | ❌ | 无全文检索 |
| 消息引用 | ❌ | 无消息引用/回复 |

---

## 3. 差距与优化

1. **P2**：消息无搜索功能，无法在历史消息中搜索关键词。
2. **P3**：消息无引用/回复功能，无法引用历史消息。

---

## 4. 开发阶段

- **Phase 1**：消息搜索功能
- **Phase 2**：消息引用/回复

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | SearchMessages RPC | P2 | — |
| 2 | 消息引用字段 + UI | P3 | — |

---

## 6. 验收标准

- [ ] 可搜索历史消息
- [ ] 可引用历史消息

---

## 7. 依赖与风险

- 搜索需评估 SQLite FTS5
