# Chat 对话 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[1 chat.md](./1%20chat.md) · **设计**：[1 chat.design.md](./1%20chat.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 SSE 流式对话、上下文管理、用量记录、停止生成与待执行队列。

**代码锚点**：
- `api/kratos/chat/v1/chat.proto` — 4 RPC + SSE 端点
- `internal/service/chat.go` — ChatService 主结构 + RunStatus/AwaitReply
- `internal/service/chat_native.go` — 原生对话入口（SSE + unary）
- `internal/service/trpc_turn.go` — trpc-agent-go 单 Agent turn 执行
- `internal/service/session_compress.go` — L0 上下文压缩
- `internal/service/session_title_llm.go` — LLM 标题生成
- `internal/service/chat_usage_ingress.go` — 用量记录

---

## 2. 现状评估（2026-05-17 复核）

| 项 | 状态 | 证据 |
|----|------|------|
| SSE 流式对话 | ✅ | `proxyNativeStream` → `runNativeAgentTurn` |
| Unary 对话 | ✅ | `RunNativeTurnUnary`（Evaluation 复用） |
| 对话选项 | ✅ | `nativeGetChatOptions` → dialog_mode / provider / model |
| 停止生成 | ✅ | `StopGeneration` + `activeRuns.Delete` + runner cancel |
| 待执行队列 | ✅ | `pendingQueue` / `pendingCancels` + `processPendingQueue` |
| RunStatus | ✅ | `runStatuses` sync.Map + `GetRunStatus` RPC |
| AwaitUserReply | ✅ | `awaitChans` + `makeAwaitReplyFunc` + `AwaitUserReply` RPC |
| 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| LLM 标题生成 | ✅ | `LLMSessionTitleGenerator.Generate` |
| 用量记录 | ✅ | `chat_usage_ingress.go` → `UsageUsecase` |
| Plugin 注入 | ✅ | `s.pluginRT.Plugins()` → `trpcrunner.WithPlugins` |
| Team 对话 | ✅ | `owner_type == "team"` → `teamsNative.RunTurn` |

---

## 3. 差距与优化

1. **P2**：SSE 与 WS 双通道并存，Chat SSE 仍为独立路径；与 51 消息机制"WS 主通道"目标未完全对齐（WS 已可传输 chat 事件，但前端仍主要走 SSE）。
2. **P2**：`member_message_start` / `member_delta` / `member_message_done` SSE 事件已预留但未发射，Team 对话时前端无法展示子 Agent 实时流。
3. **P3**：多模态附件（图片/文件上传）仅前端有 UI 雏形，后端无 attachment 持久化与 LLM 多模态输入。
4. **P3**：`tool_event` SSE 事件已预留但未发射，工具执行详情前端无法实时展示。

---

## 4. 开发阶段

- **Phase 1（优化）**：Team 对话时发射 `member_message_start/delta/done` 事件，前端实时展示子 Agent 流
- **Phase 2（对齐）**：Chat SSE 与 WS 通道统一；前端 Chat 页面可选 WS 作为主传输
- **Phase 3（扩展）**：多模态附件支持（图片/文件上传 → LLM Vision API）
- **Phase 4（增强）**：发射 `tool_event` 事件，前端实时展示工具执行进度

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Team turn 中发射 member_* SSE 事件 | P2 | — |
| 2 | Chat 页面 WS 传输模式可选开关 | P2 | — |
| 3 | 多模态附件：后端 attachment 持久化 + LLM 输入 | P3 | — |
| 4 | 发射 tool_event SSE 事件 | P3 | — |
| 5 | 单测覆盖 ChatService 关键路径 | P1 | EP-TEST-01 |

---

## 6. 验收标准

- [ ] Team 对话时前端可实时看到子 Agent 的增量输出
- [ ] `go test ./internal/service/... -run TestChat` 通过
- [ ] 附录 A Chat 行状态与实际一致
- [ ] changelog 引用对应 EP

---

## 7. 依赖与风险

- M2 多租户可能触及 Session 写路径（workspace_id 注入）
- Phase 2 依赖 51 消息机制 WS 主通道稳定
- Phase 3 依赖 LlmProvider 多模态能力（Vision API）
