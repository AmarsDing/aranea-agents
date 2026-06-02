# Cron: recordSkipped 语义统一

**日期**: 2026-05-21

## 变更

- 移除 `runner.go` 中仅写 `metadata_json` 的 `recordSkipped`。
- 新增 `recordPreExecuteOutcome` / `recordScheduleFailure`（`execute.go`），调度 tick 在 dispatch 前发现无效 config、空 message 或 schedule 计算错误时：
  - 写入 `cron_task_run`（`failure`）
  - 经 `finishTaskRun` → `finalizeRun` 更新 `metadata_json`（递增 `failure_count`）
- **`skipped`** 仅保留给 Session 忙（`ErrCronSessionBusy`），不递增 dead letter 计数。

## 测试

- `TestRecordScheduleFailure_CreatesRunAndIncrementsFailure`
- `TestFinalizeRun_SkippedDoesNotIncrementFailureCount`
