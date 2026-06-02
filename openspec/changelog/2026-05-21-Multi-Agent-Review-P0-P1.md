# Multi-Agent Review P0–P1 — 2026-05-21

## P0 数据库迁移

- 新增 `docs/sql/03_session_team_run_steps_tool_call_count.sql`
- `docs/sql/03_session.sql` 注释指向已有库 ALTER 脚本

对已有 SQLite 库执行：

```bash
sqlite3 data/aranea.db < docs/sql/03_session_team_run_steps_tool_call_count.sql
```

## P1 Summary 映射下沉

- `internal/team/summary_proto.go`：`RunSummary` / `RunMemberSummary` 强类型 + `SummaryToProto`
- `internal/team/summary.go`：`buildRunSummary` 单一聚合源；WS `BuildTeamRunSummary` 与 RPC 共用
- `internal/service/team.go`：删除 `teamSummaryToProto` / `memberSummaryFromMap`，Service 薄委托
