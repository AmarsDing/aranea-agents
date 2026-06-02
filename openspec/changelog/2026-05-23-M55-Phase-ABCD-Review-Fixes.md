# M55 Phase A–D · Review P0–P2 闭合

> **日期**：2026-05-23 · **模块**：Chat × Channel (M55)

## 摘要

Review 后闭合 P0–P2：消除 Jobs API N+1、补 service 测试、统一 session_revision bump、Jobs 面板 WS 刷新与 Graph 深链、replay hydrate 门控、i18n、SQL 迁移脚本执行。

## 后端

- `ListFiltered` JOIN `sessions.agent_id` + `graph_executions.graph_id`，消除 `ListChatBackgroundJobs` N+1
- `event.BumpAndPublishSessionRevision`：Agent/Team 共用；bump 失败 FlowLog warn
- `ChatBackgroundJob.graph_id` proto 字段
- 测试：`chat_jobs_test.go`、`session_messages_test.go`（after_revision）

## 前端

- Jobs 面板：`refreshNonce` + turn 完成 WS 触发 reload；Graph run 深链
- `useChatInboundSync`：`wsReplaying` 期间跳过 debounced hydrate
- i18n：`chat.job.*`、`chat.turn.block.*`

## 数据库

- `cmd/sqlmigrate` 支持多语句 SQL（`;` 分隔）
- 执行：`03_session_orchestration_steps.sql`、`03_session_task_dead_letters.sql`、`03_session_team_run_trace_id.sql`

## 验证

```bash
make api && go test ./internal/service/ -run 'ListChatBackgroundJobs|afterRevision' -count=1
go run ./cmd/sqlmigrate docs/sql/03_session_orchestration_steps.sql
```
