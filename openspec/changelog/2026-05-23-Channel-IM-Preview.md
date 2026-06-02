# Channel IM Preview 投影层（2026-05-23）

## 摘要

飞书/Telegram/Slack 流式 Channel 出站改为 **Envelope → Transcript → Render → StreamSender** 单管道，消除 `OnReplyDelta`（仅正文）与 `ChannelProgressProjector`（覆盖 PATCH）双轨问题；ACK 与流式 preview 合并为一条消息。

## 变更

| 层 | 新增/变更 |
|----|-----------|
| `internal/biz/channel_im_render.go` | `ChannelIMRenderPolicy`、`ChannelACKDeferredToPreview` |
| `internal/biz/channel_web_origin.go` | `ResolveChannelWebOrigin`（`web_app_origin` 优先） |
| `internal/channel/preview/` | `Transcript`、`RenderPlainText`、`feishu_card`、`split` |
| `internal/channel/lark/interactive_card.go` | 飞书 Interactive Card HTTP |
| `internal/service/channel_turn_preview.go` | `TurnPreviewCoordinator` |
| `internal/service/channel_ingress_*.go` | streaming 走 Unary + EventBus 订阅；Accept ACK defer |
| 删除 | `channel_progress_projector.go` |

## 架构图

见 [17 channel.design.md §12.9](../需求/17%20channel.design.md#129-im-preview-投影turnpreviewcoordinator2026-05-23)。

## 影响域

- **Channel 流式/Unary 出站**（feishu / slack / telegram）
- **Session 详情页**：IM Card「Web 详情」深链 `?focus=tool&tool_id=`
- **不影响** Web Chat WS、`internal/agent` EventProjector、ChatService RPC 契约

## 验证

```bash
go test ./internal/biz/... ./internal/channel/preview/... ./internal/channel/lark/... -count=1
go test ./internal/service/ -run "TurnPreview|Feishu|Interactive" -count=1
```

## P1–P3（2026-05-23 续）

| 项 | 内容 |
|----|------|
| P1 E2E | `channel_turn_preview_scenario_test.go`：ACK defer、正文→工具→正文顺序、overflow、tool card hook |
| P2 I-Eb-7 | `preview/feishu_card.go` + `lark/interactive_card.go`：`im_tool_card_mode=feishu_append` 工具**终态**时追加 Interactive Card |
| P3 | `preview/split.go` + `im_split_overflow`：Turn 结束首条 PATCH + 后续 enqueue；preset `feishu_ops_reasoning` |
| Transcript | 新工具插入时 `breakTextSegment`，保证工具前后正文分段顺序 |

## Review 优化闭合（2026-05-23）

| ID | 修复 |
|----|------|
| IM-P1-01 | 心跳 PATCH 改为 `rendered + "\n\n" + heartbeat`，不再覆盖 transcript |
| IM-P1-02 | `lark/interactive_card_test.go` httptest 契约 |
| IM-P2-01 | Card 仅在终态 `tool_result` 发送（`IsTerminalToolStatus`） |
| IM-P2-02 | Session 页解析 `focus=tool&tool_id`，Timeline 高亮滚动 |
| IM-P2-03 | `metadata.web_app_origin` + `ResolveChannelWebOrigin` |
| IM-P2-04 | Card 失败 FlowLog + `aranea_channel_tool_card_total` |
| IM-P3-01 | `logTurnFlow` 迁至 `channel_ingress_flow.go` |

详评：[2026-05-23-Channel-IM-Preview-Review.md](../review/2026-05-23-Channel-IM-Preview-Review.md)

## Review 优化闭合（2026-05-23 二轮）

| ID | 修复 |
|----|------|
| IM-P0-01 | 心跳 ticker 并入 EventBus `select`，不再阻塞 consume |
| IM-P1-E2E | [channel-im-preview-e2e.md](../需求/17-channel-development.md#12-im-preview--e2e-验收清单lt-0107) LT-01–07 |
| IM-P2-HTTP-BLOCK | Card 异步 `safego` + segment 快照 + `cardSerial` |
| IM-P2-CRED-SILENT | 凭据/配置失败 FlowLog warn |
| IM-P2-CARD-UPDATE | `UpsertToolCard`：create + PATCH 同 `message_id` |
| IM-P3-DEDUP | `preview/tool_status.go` |
| IM-P3-FLOW-CONST | `service/channel_flow_steps.go` |
| IM-P3-URL-ENCODE | `url.QueryEscape(tool_id)` |

## 出站格式化与思考/正文标签（2026-05-23 · R-UX）

| 项 | 内容 |
|----|------|
| API | `FormatAssistantReplyForIM`（raw 回复）· `FormatRenderedTranscriptForIM`（transcript PATCH）· `FormatIMSectionedReply` |
| Transcript | `render.go` 段前缀 `【思考过程】` / `【正文】`（`transcript_with_reasoning`） |
| 完成通知 | `NotifyRunCompleted` + `reasoning_markdown` → 飞书 **Agent 回复** Card 2.0（`feishu_reply_card.go`） |
| 详述 | [M55 Phase R-UX — 格式化 / 思考 UX](./2026-05-23-M55-Phase-R-UX-Channel-Format-Reasoning.md) |

## Card 样式（单行模板）

- **布局**：单行 `column_set` — emoji · **类型** · label · summary · 状态 · duration | **Web 详情** 按钮
- **状态**：绿色 ✓ · 橙色 🔄 进行中（preview PATCH 静态 emoji，Card 仅终态）· 红色 ✕
- **类型**：MCP 📡 · Skill ⚡ · Agent 🤖 · 知识库 📚 · 记忆 🧠 · 工具 🔧
- **Web 详情**：`{web_app_origin}/sessions/{sessionId}?focus=tool&tool_id={id}`（回退 `public_webhook_origin`）
- **生命周期**：`tool_call` POST 进行中 Card → 同 message PATCH 终态（每工具 1 条 IM 消息）

## 后续

- 飞书真实链路 E2E 手工验收 LT-01–07（需 tenant 凭证）
- 可选：Card `message_id` 复用 + update API，减少多工具 Turn 消息数

## 前端（2026-05-23 补充）

- `channelImPreviewDefaults.ts`：飞书推荐默认
- `channelLongTaskPresets`：预设「飞书 · IM Preview（推荐）」、`feishu_ops_reasoning`
- Channel 编辑器 LONG TASK：`im_render_mode` / `im_tool_detail` / `im_team_mode` / `web_app_origin` 等
- `SessionDetailPage` + `SessionTimelinePanel`：IM Card 深链
