package service

import (
	"aranea-agents/internal/event"
)

// envelopeErrorFromTurn maps transport/turn failures to WS EnvelopeError payloads.
func envelopeErrorFromTurn(code TurnErrorCode, detail string) *event.EnvelopeError {
	if code == "" {
		return &event.EnvelopeError{
			Type:    "run_error",
			Message: "请求处理失败，请稍后重试",
			Hint:    "持续失败请查看 Monitor 中的运行日志",
		}
	}
	msg := turnErrorMessages[code]
	if detail != "" {
		msg = msg + " (" + detail + ")"
	}
	return &event.EnvelopeError{
		Type:    string(code),
		Code:    string(code),
		Message: msg,
		Hint:    turnErrorHint(code),
	}
}

func turnErrorHint(code TurnErrorCode) string {
	switch code {
	case TurnErrAgentBuildFailed:
		return "检查智能体配置、模型与工具绑定后重试"
	case TurnErrAttachmentFailed:
		return "确认附件格式与大小，或移除附件后重试"
	case TurnErrAttachmentUnsupported:
		return "移除不支持的附件，或切换到支持视觉/多模态的模型"
	case TurnErrLLMCallFailed:
		return "可切换模型或稍后重试；持续失败请查看 Monitor 用量事件"
	case TurnErrTurnTimeout, TurnErrFirstByteTimeout:
		return "缩短提问或切换响应更快的模型"
	case TurnErrEmptyReply:
		return "调整提问方式或关闭过于严格的工具限制"
	case TurnErrAgentForbidden:
		return "确认当前账号有权访问该智能体"
	default:
		return ""
	}
}
