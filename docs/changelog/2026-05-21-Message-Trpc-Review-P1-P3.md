# Message / Tool / Eval — trpc 对齐与 Review P1–P3（2026-05-21）

## 摘要

按 `docs/README.md` 与 review 结论，完成 trpc-agent-go 术语对齐、P1–P3 代码与文档同步。

## P1

| 项 | 变更 |
|----|------|
| Team 成员落库 | `options_json.team_member` + `member_agent_key` + `model_name: team/member`；与 `useChatMessageRow` / WS 一致 |
| Tool 记录 | 冲突时 **Update** 终态字段，**保留** `created_at`；运行时 `source=trpc`（常量 `biz.ToolInvocationSourceRuntime`），Bus 为 `event_bus` |
| 消息搜索 | 仍强制 `session_id`（上轮已做） |

## P2

| 项 | 变更 |
|----|------|
| Eval 解耦 | `EvalTurnGateway` + `ChatService.RunEvalAgentTurn`；`NewEvaluationRunner` 依赖接口而非整包 Chat |
| 文件命名 | `chat_webhook.go` → `chat_enqueue.go`（仅 enqueue 错误映射） |
| Webhook | 仍仅 `callbackConsumer` → `WebhookDispatcher` |

## P3

| 项 | 变更 |
|----|------|
| 成员消息 ID | `env.ID` 为空时用内容哈希后缀，避免碰撞 |
| 前端 | `role=member` 与 `schema: chat.team_member/v1` 识别为 Team 成员气泡 |
| Flow log | TTL 清理与 `since/until`（上轮已做） |

## Wire

- `wire.Bind(EvalTurnGateway, *ChatService)`；`ProvideEvaluationRunner(chat, turns, ...)`
- 若 `make wire` 因仓库其他 Provider（如 `EmbeddingService`）失败，可暂用手工更新 `wire_gen.go` 中 `ProvideEvaluationRunner` 第二参为 `chatService`

## 验证

```bash
go build ./internal/...
go test ./internal/biz/... -count=1 -run TestTeamMember
```
