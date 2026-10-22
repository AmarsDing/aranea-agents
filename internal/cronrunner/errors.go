package cronrunner

import "errors"

var (
	errRunAtRequired = errors.New("run_at is required for once cron task")
	errInvalidRunAt  = errors.New("invalid run_at")
	errCronFields    = errors.New("cron expression must have 5 fields")
	errCronNoSlot    = errors.New("unable to find next cron time within one year")

	// errCronSkippedMissed 是 skipped_missed 的固定字节原因：滞后时长等动态信息只进
	// 流程日志/诊断，原因串保持稳定字节，连续同字节跳过才能折叠进同一行。
	errCronSkippedMissed = errors.New("cron: skipped missed run beyond grace window")
)
