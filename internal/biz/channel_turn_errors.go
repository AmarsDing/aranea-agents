package biz

// ChannelTurnErrorKind classifies provider/runtime failures into an IM-safe taxonomy (CH-BOR-09).
type ChannelTurnErrorKind string

const (
	ChannelTurnErrNone            ChannelTurnErrorKind = ""
	ChannelTurnErrBusy            ChannelTurnErrorKind = "busy"
	ChannelTurnErrTimeout         ChannelTurnErrorKind = "timeout"
	ChannelTurnErrRateLimit       ChannelTurnErrorKind = "rate_limit"
	ChannelTurnErrContextOverflow ChannelTurnErrorKind = "context_overflow"
	ChannelTurnErrBilling         ChannelTurnErrorKind = "billing"
	ChannelTurnErrAuth            ChannelTurnErrorKind = "auth"
	ChannelTurnErrGeneric         ChannelTurnErrorKind = "generic"
)

// User-visible error message constants for Channel turn errors.
const (
	ChannelTurnErrorRateLimitMsg       = "模型调用过于频繁，请稍后再试。"
	ChannelTurnErrorContextOverflowMsg = "对话上下文过长，请发送 /background 转入后台继续，或在 Web 端查看。"
	ChannelTurnErrorSyncCapMsg         = "任务执行较慢，建议使用 /background 转入后台继续。"
	ChannelTurnErrorGenericMsg         = "任务执行失败，请稍后重试。"
	ChannelTurnErrorBusyMsg            = "上一条仍在处理中，请稍候再试。"
	ChannelTurnErrorBillingMsg         = "模型账户余额不足，请充值后再试。"
	ChannelTurnErrorAuthMsg            = "模型鉴权失败，请检查供应商密钥配置。"
	ChannelTurnEmptyReplyMsg           = "助手未返回内容，请稍后重试。"
)

// FormatChannelTurnErrorMessage maps an error kind to a user-visible IM message.
func FormatChannelTurnErrorMessage(kind ChannelTurnErrorKind) string {
	switch kind {
	case ChannelTurnErrBusy:
		return ChannelTurnErrorBusyMsg
	case ChannelTurnErrTimeout:
		return ChannelTurnErrorSyncCapMsg
	case ChannelTurnErrRateLimit:
		return ChannelTurnErrorRateLimitMsg
	case ChannelTurnErrContextOverflow:
		return ChannelTurnErrorContextOverflowMsg
	case ChannelTurnErrBilling:
		return ChannelTurnErrorBillingMsg
	case ChannelTurnErrAuth:
		return ChannelTurnErrorAuthMsg
	case ChannelTurnErrNone:
		return ""
	default:
		return ChannelTurnErrorGenericMsg
	}
}
