# Session Turns 编排追踪 + Detail 页 + Restore/Archive

**日期**：2026-05-17
**模块**：M5 Session 管理
**变更摘要**：新增 session_turns 编排追踪表、Session Detail 独立页面、ChatSessionSidebar Restore/Archive 操作

---

## 变更内容

### 后端

1. **session_turns Ent Schema**：新增 `session_turns` 表，记录每轮对话的指标（token 用量、延迟、工具/技能调用次数、内容预览、错误信息等）
2. **Proto 新增**：`SessionTurn` message + `ListSessionTurns` RPC（`GET /v1/sessions/{session_id}/turns`）
3. **Biz 层**：`SessionTurn` 领域模型 + `SessionTurnUpdateFields` + `SessionRepository` 新增 `CreateSessionTurn`/`UpdateSessionTurn`/`ListSessionTurns`/`GetSessionTurn`
4. **Data 层**：`session_turn_repo.go` 实现 Ent CRUD
5. **Service 层**：`ListSessionTurns` RPC 实现 + `toProtoSessionTurn` 转换
6. **ChatService 集成**：`runSingleAgentViaTRPC` 完成后自动写入 `SessionTurn` 记录

### 前端

1. **API 层**：新增 `restoreSession`、`updateSession`、`listSessionTurns` + `SessionTurn` 类型
2. **SessionDetailPage**：独立页面，包含 Turns/Messages/Timeline 三个 Tab
3. **SessionTurnsPanel**：展示 Turn 列表，含分页、状态标记、token 统计
4. **SessionMessagesPanel**：展示消息列表，区分用户/助手角色
5. **SessionTimelinePanel**：展示 Timeline 事件，支持类型过滤和排序
6. **ChatSessionSidebar**：新增"恢复会话"、"归档"、"详情页"菜单项
7. **路由更新**：`/sessions/:sessionId` 指向 `SessionDetailPage`

## 影响范围

- 后端：`internal/data/ent/schema/`、`internal/biz/`、`internal/data/`、`internal/service/`、`api/kratos/session/v1/`
- 前端：`web/src/pages/`、`web/src/components/sessions/`、`web/src/components/chat/`、`web/src/features/`

## 关联文件

详见 [devlog/2026-05-17-Session-Turns.md](../devlog/2026-05-17-Session-Turns.md)
