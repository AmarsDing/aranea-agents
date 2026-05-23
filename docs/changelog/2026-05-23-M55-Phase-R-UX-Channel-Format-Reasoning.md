# M55 Phase R-UX — 飞书格式化 · 思考 UX · 入站 Session 同步

> **日期**：2026-05-23 · **模块**：M55 Chat × Channel · **前置**：[Phase R-UX 实现](./2026-05-23-M55-Phase-R-UX-Implementation.md)

## 摘要

收口飞书出站可读性、Web 思考过程 Cursor 式展示、Channel 入站时 Session 列表即时刷新，以及 Review 指出的格式化顺序 / transcript 截断等问题。

---

## 后端 — Channel IM 出站

| ID | 变更 | 代码锚点 |
|----|------|----------|
| CC-CHANNEL-FMT-01 | `FormatAssistantReplyForIM` / `FormatRenderedTranscriptForIM` 拆分：raw 回复 vs 已渲染 transcript | `internal/channel/preview/format_im.go` |
| CC-CHANNEL-FMT-02 | 出站统一入口：`enqueueOutboundText` · transcript overflow 走 `enqueueOutboundTranscript` | `internal/service/channel_delivery_worker.go` |
| CC-CHANNEL-FMT-03 | `NotifyRunCompleted`：**先格式化再 SplitPages**；`skipFormat` 避免二次处理 | `session_run_escalation_notifier.go` |
| CC-CHANNEL-FMT-04 | Transcript 段标签：`【思考过程】` / `【正文】`（`im_render_mode=transcript*`） | `internal/channel/preview/render.go` |
| CC-CHANNEL-FMT-05 | 完成通知读取 `options_json.reasoning_markdown`；`FormatIMSectionedReply` | `preview/format_im.go` · `ReasoningMarkdownFromOptions` |
| CC-CHANNEL-FMT-06 | 飞书/Lark 有思考段时发送 **Card 2.0「Agent 回复」**（思考 + 正文分区，类升格卡片） | `preview/feishu_reply_card.go` |
| CC-FEISHU-02+ | 升格卡片 **Card JSON 2.0** + `card.action.trigger` / v1 flat；取消 ownership 校验 | `feishu_escalate_card.go` · `channel_ingress_card_action.go` |
| CC-FIX-CHANNEL-01+ | Durable 完成/失败 IM；`context.WithoutCancel` 持久化 notify 上下文 | `chat_durable_resume.go` |

**仍待运维**：**CC-FEISHU-OPS-01** — 飞书控制台订阅 `card.action.trigger` 并发布应用版本。

---

## 前端 — Chat 思考 UX

| ID | 变更 | 说明 |
|----|------|------|
| CC-WEB-REASONING-02 | `ChatReasoningPeek.vue` | 思考与正文分离；正文区显示 **「正文」** 标签 |
| CC-WEB-REASONING-03 | **Live tail** | 默认视口 2 行高；流式更新时 **始终显示最后两行**（自动滚底） |
| CC-WEB-REASONING-04 | 交互 | 单击选中 · 滚轮浏览历史 · 双击展开 ~8 行 · Esc 恢复 tail |
| CC-WEB-NOTIFY+ | `useGlobalInboundNotifications` 于 MainLayout | 任意路由铃铛 + toast；`runner_completion` 与 `run_status completed` 去重 toast |
| CC-WEB-SESSION-01 | `channelInboundSessionRefresh.ts` | Channel 入站时 `loadAgentSessions`，**右侧 Session 列表即时出现** |

**数据流**：`envelope.content.reasoning` → `chatStreamingSnapshots` → `options_json.reasoning_markdown` → `ChatReasoningPeek`；正式答案仅 `content_markdown` / `bodyMarkdown`。

---

## Review 闭合（格式化）

| 项 | 修复 |
|----|------|
| P1 先分页后 format | `NotifyRunCompleted` 改为 format → split |
| P1 transcript Flush 误截断 | `patchDeliverable` / 流式 PATCH 用 `FormatRenderedTranscriptForIM` |
| P2 空串静默丢弃 | `flowStepChannelOutbound` warn + fallback `（暂无文本回复）` |
| P2 硬编码 feishu limit | `sessionPlatform` 读 session meta |

---

## 验证

```bash
go test ./internal/channel/preview/... -count=1
go test ./internal/service/ -run "TurnPreview|CardAction|Format" -count=1
cd web && pnpm test -- mergeSessionMessages messagePlannerPresentation
```

**手工 E2E**（见 [17-channel-development.md §12](../需求/17-channel-development.md#12-im-preview--e2e-验收清单lt-0107)）：

1. 飞书发消息 → Web 右侧 Session **无需切页即出现**
2. Chat 流式思考 → peek 区 **最后两行持续刷新**
3. `transcript_with_reasoning` → 飞书 preview 含 `【思考过程】` / `【正文】`
4. Durable 完成 → 飞书收到分区文本或 **Agent 回复** 卡片
