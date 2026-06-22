package service

import "time"

const (
	channelPreviewThinkingHint   = "正在思考与执行工具，请稍候…"
	channelStreamEmptyPreviewMsg = "处理已完成。若未看到完整回复，请发送 /background 或在 Web 端查看。"
)

const (
	channelAsyncGraphDoneSummary   = "Graph 任务已完成。"
	channelAsyncCronSkippedSummary = "Cron 任务已跳过。"
)

const (
	channelCardActionServiceUnavailable = "服务未就绪"
	channelCardActionUnknownOperation   = "未知操作"
	channelCardActionFailedRetry        = "操作失败，请稍后重试"
)

const (
	channelAccessDeniedDefault    = "暂无使用权限，请联系管理员。"
	channelAccessDeniedWithReason = "暂无使用权限："
)

const (
	channelStatusPhaseTemplate = "当前任务阶段：%s"
)

const (
	channelTurnBusyRetries = 3
	channelTurnBusyBackoff = 250 * time.Millisecond
)
