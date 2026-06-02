# Cron P1–P3 优化

**日期**：2026-05-21  
**模块**：Cron (21)

## 摘要

按 code review 完成 P1–P3 优化，严格遵循分层：`service → biz → cronrunner/data`。

### P1
- **Per-task 互斥锁**：`executeTask` / 异步 manual run 对同一 `task_id` 加锁，避免调度 tick 与手动触发并发双跑。
- **Session busy → `skipped`**：不再记为 `success`；Prometheus 标签 `status=skipped`。
- **`GetCronTaskRun(id)`**：Repo 按 ID 查询，移除 `ListCronTaskRuns` 扫描反查。

### P2
- **Trigger 走 biz**：`CronUsecase.TriggerTask` + `CronTaskTrigger` 接口；`service` 不再 import `cronrunner`。
- **统一错误映射**：`mapCronError`（NotFound / ServiceUnavailable / 409 busy / BadRequest）。
- **Manual 语义**：手动触发不推进 `next_run_at`、不 pause `once`、失败不计入 dead letter 连续失败。

### P3
- **异步 trigger**：`TriggerTask` 立即返回 `pending` run；后台 `safego` 执行。
- **`ResetCronTaskFailures` RPC**：`POST /v1/cron-tasks/{id}/reset-failures`；前端 dead 重置改调专用 API。
- **`execute.go` 拆分**：`insertPendingRun` / `runDispatch` / `finalizeRun` / `executeTask`。

### P2（续）
- **`finishTaskRun`**：dispatch 完成后重新 `GetCronTask`，再 `finalizeRun`，避免 schedule/manual 交错时 metadata lost update。
- **`TriggerTask` insert 持锁**：pending run 在 per-task 锁内创建，与 schedule `executeTask` 互斥。

## 验证

```bash
make api && make wire && go test ./internal/biz/... ./internal/cronrunner/... ./internal/service/...
cd web && pnpm test --run src/features/cron
```
