package service

import "time"

const (
	channelTurnErrorSyncCapMsg   = "任务执行较慢，建议使用 /background 转入后台继续。"
	channelTurnErrorGenericMsg   = "任务执行失败，请稍后重试。"
	channelTurnErrorBusyMsg      = "上一条仍在处理中，请稍候再试。"
	channelPreviewThinkingHint   = "正在思考与执行工具，请稍候…"
	channelStreamEmptyPreviewMsg = "处理已完成。若未看到完整回复，请发送 /background 或在 Web 端查看。"
)

const (
	channelTurnBusyRetries = 3
	channelTurnBusyBackoff = 250 * time.Millisecond
)
