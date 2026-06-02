# M55 · 飞书入站修复 + Cursor UX 差距分析 · 文档同步

> **日期**：2026-05-23 · **模块**：Channel 入站 · Chat UX (M55)

## 摘要

1. **飞书入站 P0 修复**：`channel_peer_session` 指向已软删 Session 时自动 rebind；消除 WS 层重复技术错误 IM。
2. **M55 Phase A–D Review 闭合**：Jobs N+1、revision bump、Jobs 面板、i18n、SQL 迁移等（见 [M55-Phase-ABCD-Review-Fixes](./2026-05-23-M55-Phase-ABCD-Review-Fixes.md)）。
3. **Cursor 对标差距分析**：思考过程空壳、双 ToolStrip/双思考标签、与 Cursor 体验差距 ~40–45%（UX 视角）。

---

## 已解决问题

| # | 问题 | 根因 | 修复 | 文件 |
|---|------|------|------|------|
| H-01 | 飞书发消息立即 `session not found` | `ensureChannelSession` 直接使用 peer bind，未校验 Session 是否仍存在（Web 删 Session 后 bind 过期） | 校验失败 → 新建 Session + `UpdateSessionID` | `channel_ingress_session.go` · `channel_peer_session.go` |
| H-02 | 飞书收到 3 条错误（技术错误 + ACK + 友好失败） | `HandleWSInbound` 同步 `SendText` raw error；`executeInboundTurn` 又 enqueue 友好错误 | Turn 失败不再向 WS 层返回 error（用户错误已由 LT-06 投递） | `channel_ingress_inbound.go` · `ws_inbound.go` |
| H-03 | `ChatBackgroundJob.summary` invalid UTF-8 | DB 历史脏数据 | `strutil.ValidUTF8()` 净化 proto 字段 | `chat_jobs.go` · `channel.go` |
| H-04 | Jobs 面板 import 路径错误 | `useChatBackgroundJobs.ts` 误用 `../api` | 改为 `./api` | `useChatBackgroundJobs.ts` |
| H-05 | TurnBlock i18n 缺失 | `chat.turn.block.toolsSummary/toolsAria/failed` 未注册 | `zh-CN.ts` / `en-US.ts` | i18n |
| H-06 | ListChatBackgroundJobs N+1 | 循环查 agent_id / graph_id | `ListFiltered` JOIN | `channel_turn_job` repo |
| H-07 | session_revision bump 分散 | Agent/Team 各自 bump | `event.BumpAndPublishSessionRevision` | `trpc_turn.go` · `runner_team_trpc.go` |
| H-08 | replay 期间重复 hydrate | WS 回放与 revision debounce 竞态 | `wsReplaying` 门控 | `useChatInboundSync.ts` |

**DB 实证（H-01）**：`channel_peer_session.session_id` 指向 `sessions.deleted_at != ''` 的行（peer `ou_083b3e94…` → 已删 Session）。

---

## 后期优化列表（按优先级）

### P0 · 思考/工具 UX（1–2 周）— 对应截图问题

| ID | 任务 | 说明 | 验收 |
|----|------|------|------|
| CC-C-UX-01 | 思考/ReAct 互斥呈现 | `reasoning_markdown` 与 `ChatReactSteps` 不同时展示；空 reasoning 不渲染 `<details>` | 无空「思考过程」壳 |
| CC-C-UX-02 | 流式思考状态 | 无 reasoning 时显示单行「正在思考…」，非空折叠块 | 首字节前 UX 清晰 |
| CC-C-UX-03 | 双 ToolStrip 排查 | 失败重试多 turn、hydrate 重复 activity 去重 | 单轮仅一条 ToolStrip |
| CC-C-UX-04 | `TurnAssistantBubble` 拆分 | 从 `ChatMessageRow` 抽出 assistant 编排，统一 reasoning/ReAct/body | CC-C-01 补完 |

### P1 · M55 收口（~1 周）

| ID | 任务 | 说明 | 验收 |
|----|------|------|------|
| CC-B-07 | UserBubble 来源徽标 | Web / 飞书 / Cron chip（§5.1 蓝图） | M55-SYNC 可视 |
| CC-C-05 | 虚拟列表 benchmark | 500 条 TurnBlock @55fps | M55-UI-01 |
| CC-C-06 | completion 增量 merge | `runner_completion` 不全量 replace，revision hydrate only | 无流式+DB 双态 |
| CC-C-07 | Session 顶栏诊断 | `N msgs · WS · rev=R · ctx%` | §5.3 |
| CC-HOT-02 | 删 Session 清 peer bind | 可选：`DeleteSession` 时清理 `channel_peer_session` | 减少 stale bind |
| CC-E2E-01 | 手工验收脚本 | M55-SYNC-01/02 · M55-UI-01/02 · M55-JOB-01 | channel e2e 文档 |

### P2 · Cursor 对齐（2–4 周）

| ID | 任务 | 说明 |
|----|------|------|
| CC-E-01 | Composer `@` 引用 | 文件/知识库 mention picker |
| CC-E-03 | diff Apply 卡片 | TurnBlock `ArtifactRow` + fragment edit API |
| CHAT-R2-03 | TurnExecutor 抽象 | Agent/Team/Channel 统一 admission/stream/persist |
| CC-F-01~04 | 24h Durable Job | Worker deadline 替代 asyncWatch 2h |

### P3 · 产品 polish

| ID | 任务 |
|----|------|
| CC-C-08 | Team Session 启用 TurnBlock（当前 `!isTeamSession` 禁用） |
| CC-D-05 | Job 完成与 TurnBlock 时间线联动（Artifact 行） |
| CC-E-04 | Reasoning 侧栏模式（可选） |

---

## 与 Cursor 差距摘要

| 维度 | Cursor | Aranea（2026-05-23） |
|------|--------|----------------------|
| 思考链 | 单块流式、有内容才展示 | 三路（reasoning/ReAct/占位）易空壳 |
| 工具 | 内联紧凑 | ToolStrip 已折叠，但与 ReAct 卡片仍有重复 |
| 一轮时间线 | 严格单块 | TurnBlock 有；失败重试可产生多块 |
| @ 上下文 | 成熟 | 未做 |
| Apply diff | 成熟 | 未做 |
| 后台任务 | 内嵌对话流 | 独立面板，未与 Turn 融合 |

**M55 完成度（估算）**：后端协议 ~75% · Web 聊天 UX ~55% · 整体 Cursor 对标 ~40–45%。

---

## 验证

```bash
# 飞书 rebind 单测
go test ./internal/service/ -run 'EnsureChannelSession' -count=1

# M55 回归
make api && go test ./internal/service/ -run 'ListChatBackgroundJobs|afterRevision|EnsureChannelSession' -count=1
cd web && pnpm test -- groupMessagesByTurn
```

重启 admin 后飞书入站验证：ACK → 正常回复；失败时仅一条友好 IM 错误。
