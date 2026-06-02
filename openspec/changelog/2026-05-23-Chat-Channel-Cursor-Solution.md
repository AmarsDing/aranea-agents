# M55 — Chat × Channel × Cursor 对标方案文档化

**日期**：2026-05-23  
**模块**：Chat(1) · Channel(17) · Message(51) · Frontend  
**类型**：规划 / 文档

## 摘要

基于飞书长任务 5 分钟超时与 Web 无法可靠展示 Channel 会话的根因分析，新增 **M55 整体方案** 与 **分阶段开发计划**，并在 execution-plan 建立 **迭代 CC** 任务板。

## 新增文档

| 文档 | 说明 |
|------|------|
| [55-chat-channel-cursor-solution.md](../需求/55-chat-channel-cursor-solution.md) | Cursor vs Aranea 对照、双平面执行模型、Session Sync、TurnBlock、验收标准 |
| [55-chat-channel-cursor-development.md](../需求/55-chat-channel-cursor-development.md) | Phase A–F 任务拆分与排期 |

## 更新文档

- [execution-plan.md](../guides/execution-plan.md) — 迭代 CC 任务板；backlog 优先级
- [docs/README.md](../README.md) — §5.2 M55 索引
- [README-development.md](../需求/README-development.md) — Chat / Channel 链入 M55
- [1-chat-development.md](../需求/1-chat-development.md) — P0 Cursor 对标节
- [17-channel-development.md](../需求/17-channel-development.md) — Phase E 链入 M55
- [17-channel-agent-team-integration.md](../需求/17-channel-agent-team-integration.md) — M55 延伸
- [message-development.md](../需求/message-development.md) — session_revision 待办

## 核心结论（实现前）

1. **5m 超时**：24h 任务不得走 Sync Turn；需 async Graph / Durable Job 双平面。
2. **Web 不同步**：数据多在库；缺 `session_revision`、TurnBlock 与 Channel 入站聚焦。
3. **Cursor 对标**：Turn 容器 + 工具折叠 + Background Job 面板 + @ / Apply。

## 下一步（按 plan）

P0：Phase A 配置路由 → Phase B session_revision → Phase C TurnBlock UI。
