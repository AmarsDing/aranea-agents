# Chat 对话 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用（WS/EventBus 主通道；HTTP unary 兼容）
> **需求**：[1 chat.md](./1%20chat.md) · **设计**：[1 chat.design.md](./1%20chat.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。

**代码锚点**：
- `api/kratos/chat/v1/chat.proto` — Chat RPC 契约（发送、选项、停止、待执行、RunStatus、AwaitUserReply）
- `internal/server/ws.go` — WebSocket 主实时通道（订阅、取消、上行 user_message、事件回放）
- `internal/service/chat.go` — ChatService 主结构 + RunStatus/AwaitReply/pending queue
- `internal/service/chat_native.go` — 原生对话入口（HTTP unary + WS 上行复用）
- `internal/service/trpc_turn.go` — trpc-agent-go 单 Agent turn 执行 + EventBus 投影
- `internal/agent/event_projector.go` — trpc event → Envelope 投影
- `internal/event/envelope.go` — WS Envelope 协议与 channel 路由
- `internal/service/session_compress.go` — L0 上下文压缩
- `internal/service/session_title_llm.go` — LLM 标题生成
- `internal/service/chat_usage_ingress.go` — 用量记录

---

## 2. 现状评估（2026-05-17 复核）

| 项 | 状态 | 证据 |
|----|------|------|
| WS 实时对话 | ✅ | `/v1/ws` + `user_message` 上行 + EventBus Envelope 下行 |
| HTTP unary 对话 | ✅ | `SendChatMessage` / `RunNativeTurnUnary`（Channel、Cron、Evaluation 可复用） |
| 对话选项 | ✅ | `nativeGetChatOptions` → dialog_mode / provider / model |
| 停止生成 | 🟡 | 单 Agent：`StopGeneration` + `activeRuns.Delete` + runner cancel；Team turn 未进入 `activeRuns` |
| 待执行队列 | 🟡 | 单 Agent：`pendingQueue` / `pendingCancels` + `processPendingQueue`；Team 会话串行保护待补 |
| RunStatus | 🟡 | 后端 `runStatuses` sync.Map + `GetRunStatus` RPC；前端 Chat 页未形成完整 UI |
| AwaitUserReply | 🟡 | 单 Agent：`awaitChans` + `makeAwaitReplyFunc` + `AwaitUserReply` RPC；Team Builder 未注入 AwaitHook |
| 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| LLM 标题生成 | ✅ | `LLMSessionTitleGenerator.Generate` |
| 用量记录 | ✅ | `chat_usage_ingress.go` → `UsageUsecase` |
| Plugin 注入 | ✅ | `s.pluginRT.Plugins()` → `trpcrunner.WithPlugins` |
| Team 对话 | ✅ | `owner_type == "team"` → `teamsNative.RunTurn` |
| SSE `/v1/chat/messages/stream` | ✅ 已移除 | `register_chat.go` 仅注册 proto HTTP；`chat.proto` 注释标明 no SSE route |

---

## 3. 差距与优化

1. **P1**：Chat 文档曾长期保留 SSE 路径描述；代码已切换到 WS/EventBus，需统一需求、设计和开发计划口径。
2. **P1**：Team turn 未登记到 `ChatService.activeRuns`，停止生成、待执行队列、会话级串行保护与单 Agent 路径不一致。
3. **P1**：AwaitUserReply 后端 API 已有，但 `AwaitHook` 仅注入单 Agent builder，Team 路径和前端 Chat 页回复 UI 未闭环。
4. **P2**：Team 子 Agent 实时流仍不完整；`member_message_start/delta/done` 类型和前端处理已存在，但 Team Runner 目前主要投影 `text_delta/text_done` 与 team run 事件，缺少稳定的成员级 start/delta/done 发射。
5. **P2**：工具执行展示仍偏粗粒度；当前有 `tool_call/tool_result` Envelope，但前端 Chat 面板只展示简化文本，缺少 before/after、参数、结果、耗时、错误的结构化折叠视图。
6. **P2**：pending 失败 `pending_id` 当前写在 Envelope metadata，前端 `error` handler 未消费；应统一为 `error.pending_id` 或明确 metadata 约定。
7. **P3**：多模态附件仅为前端占位和 proto 引用计数；后端无 attachment 持久化、权限校验、对象存储、病毒/大小校验与 LLM Vision 输入装配。
8. **P3**：模型选择来源不完全统一；后端 `GetChatOptions("provider"|"model")` 已动态化，但 Chat 前端模型列表主要读取 `platform` 资源。
9. **P3**：RunStatus/AwaitUserReply 使用进程内 `sync.Map`，服务重启后不可恢复；生产级长任务需要持久化或 EventBuffer 恢复策略。

---

## 4. 开发阶段

- **Phase 1（文档对齐）**：统一为 WS/EventBus 主通道，移除 SSE 端点与 `proxyNativeStream` 相关设计口径
- **Phase 2（运行正确性）**：Team turn 接入会话级 active run/cancel/pending 串行保护
- **Phase 3（人工等待）**：Team Builder 注入 AwaitHook，前端 Chat 页接入 RunStatus/AwaitUserReply UI
- **Phase 4（体验优化）**：Team 对话发射稳定的 `member_message_start/delta/done` 事件，前端实时展示子 Agent 流
- **Phase 5（工具可观测）**：基于 `tool_call/tool_result` 完善结构化工具事件卡片（参数、结果、耗时、错误）
- **Phase 6（扩展）**：多模态附件支持（上传/持久化/权限校验 → LLM Vision API）
- **Phase 7（一致性）**：统一 Chat 模型选项来源（Chat Options 或 Platform Resource 二选一）
- **Phase 8（可靠性）**：RunStatus/AwaitUserReply 持久化或接入可恢复事件缓冲

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 文档移除 SSE 主路径，校准为 WS/EventBus | P1 | — |
| 2 | Team turn 接入 activeRuns/cancel/pending 串行保护 | P1 | — |
| 3 | Team 路径注入 AwaitHook，Chat 页接入人工回复 UI | P1 | — |
| 4 | pending 失败 `pending_id` 统一到 `error.pending_id` 或前端消费 metadata | P2 | — |
| 5 | Team turn 中发射稳定 member_* WS Envelope | P2 | — |
| 6 | 工具事件结构化展示：tool_call/tool_result → 前端卡片 | P2 | — |
| 7 | 多模态附件：后端 attachment 持久化 + LLM 输入 | P3 | — |
| 8 | 统一 Chat 模型选项来源，避免 provider/model 双口径 | P3 | — |
| 9 | RunStatus/AwaitUserReply 持久化或恢复策略 | P3 | — |
| 10 | 单测覆盖 ChatService / WS handler 关键路径 | P1 | EP-TEST-01 |

---

## 6. 验收标准

- [ ] 文档中不再声明 `/v1/chat/messages/stream` 为当前可用端点
- [ ] Team 对话停止生成、待执行队列与单 Agent 行为一致
- [ ] AwaitUserReply 在单 Agent 与 Team 路径均可触发，前端可提交回复
- [ ] Team 对话时前端可实时看到子 Agent 的增量输出
- [ ] 工具执行可展示结构化参数、结果、耗时和错误
- [ ] `go test ./internal/service/... -run TestChat` 通过
- [ ] 附录 A Chat 行状态与实际一致
- [ ] changelog 引用对应 EP

---

## 7. 依赖与风险

- M2 多租户可能触及 Session 写路径（workspace_id 注入）
- Phase 2 依赖 trpc-agent-go Team 事件 author/invocation 元数据稳定
- Phase 3 依赖 EventProjector 对 tool result 的关联 ID、耗时和错误字段完善
- Phase 4 依赖 LlmProvider 多模态能力（Vision API）与 Artifact/对象存储能力
