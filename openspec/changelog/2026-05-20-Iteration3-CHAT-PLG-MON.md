# 2026-05-20 — 迭代 3：Chat 多模态 / RunStatus 持久化 / PluginRun / Monitor 告警

## 摘要

完成迭代 2 收尾三项：Chat 附件经 Artifact 装配进 tRPC 多模态 UserMessage、RunStatus 写入 `sessions.state_json`；Plugin 回调审计表与 `ListPluginRuns`；Monitor 可配置告警规则（runner 错误率）+ 前端 Alerts 页签。

## 变更

### Chat（I2-CHAT-01）

- `internal/service/run_status_store.go`：`runtime.run_*` 键持久化；终态清理
- `internal/service/chat_attachments.go` + `trpc_turn.go`：按 `attachment_ids` 加载制品并构建 text/image/file parts
- `cmd/admin/wire.go`：`ChatServiceDeps.Artifacts` 注入
- 前端 `useChatWorkspace.ts`：真实 `uploadArtifact`（需已选会话）

**MVP 限制**：`awaiting_user` 的 `awaitChans` 仍在进程内；重启后 UI 可从 DB 恢复状态展示，但提交回复仍需同进程或后续 EventBuffer 方案。

### Plugin（I2-PLG-01）

- 表 `plugin_runs`（`docs/sql/13_plugin_run.sql`，运行时 DDL：`internal/data/sql/plugin_run.sql`）
- 会话记忆链等 SQLite 合集（`docs/sql/16_memory_chain.sql` 说明，`internal/data/sql/memory_chain.sql`；Usage 段与 `08_usage.sql` 同步）
- `GET /v1/plugins/runs`；`RepoStatsRecorder` 异步写入运行行

### Monitor（MON-01）

- 表 `monitor_alert_rules`；`GET/PUT /v1/monitor/alert-rules`
- `biz.EvaluateAlerts`：`runner.error_rate` 超阈写入 `alert.fired` 事件
- 前端 `MonitorPage` Alerts 页签 + `MonitorAlertRules.vue`

## 验证

- `make api && make wire && make build && make test`
