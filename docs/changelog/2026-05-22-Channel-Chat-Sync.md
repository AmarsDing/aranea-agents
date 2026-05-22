# Channel 入站与 Web Chat 同步

**日期**：2026-05-22

## 问题

飞书入站已收到模型回复，但平台 Chat 界面不显示：Channel 在独立 `session_id` 上落库，前端仅订阅用户手动选中的会话，且未刷新会话列表。

## 后端

- `sessions.metadata_json`：Channel 建会话时写入 `source=channel`、platform、channel_key 等（`biz.BuildChannelSessionMetadataJSON`）。
- `UpdateChannel`：routing 目标变化时清除 `channel_peer_session`（新 peer 按新路由建会话）。

## 前端

- `useChatInboundSync`：全局 WS（`session_id=*`）监听 `text_delta` / `runner_completion`，刷新会话列表；当前 Session 打开时流式 Markdown（沿用 `renderStreamingMarkdown`）。
- 会话侧栏：渠道会话徽章（metadata 或标题前缀）。
- Channel 编辑：`dm_scope` 下拉。
- Agent 设置：`AgentChannelRefsSection` 展示引用该 Agent 的 Channel。

## 文档

- 更新 `17-channel-agent-team-integration.md` / `.design.md` 差距表。
