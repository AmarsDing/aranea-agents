package service

import (
	"aranea-agents/pkg/apierror"
)

type TurnErrorCode string

const (
	TurnErrAgentBuildFailed      TurnErrorCode = "AGENT_BUILD_FAILED"
	TurnErrAttachmentFailed      TurnErrorCode = "ATTACHMENT_FAILED"
	TurnErrAttachmentUnsupported TurnErrorCode = "ATTACHMENT_UNSUPPORTED"
	TurnErrLLMCallFailed         TurnErrorCode = "LLM_CALL_FAILED"
	TurnErrTurnTimeout           TurnErrorCode = "TURN_TIMEOUT"
	TurnErrEmptyReply            TurnErrorCode = "EMPTY_REPLY"
	TurnErrFirstByteTimeout      TurnErrorCode = "FIRST_BYTE_TIMEOUT"
	TurnErrAgentForbidden        TurnErrorCode = "AGENT_FORBIDDEN"
	TurnErrStreamPreviewFailed   TurnErrorCode = "STREAM_PREVIEW_FAILED"
)

var turnErrorMessages = map[TurnErrorCode]string{
	TurnErrAgentBuildFailed:      "智能体构建失败，请检查智能体配置后重试",
	TurnErrAttachmentFailed:      "附件处理失败，请检查文件是否有效",
	TurnErrAttachmentUnsupported: "当前模型不支持该附件类型",
	TurnErrLLMCallFailed:         "模型调用失败，请稍后重试或切换模型",
	TurnErrTurnTimeout:           "响应超时，请稍后重试",
	TurnErrEmptyReply:            "智能体未产生响应，请调整提问后重试",
	TurnErrFirstByteTimeout:      "模型响应过慢，请稍后重试或切换模型",
	TurnErrAgentForbidden:        "无权访问该智能体",
	TurnErrStreamPreviewFailed:   "流式回复预览更新失败",
}

const metaKeyTurnErrorCode = "turn_error_code"

func TurnError(code TurnErrorCode, detail string) error {
	msg := turnErrorMessages[code]
	if detail != "" {
		msg = msg + " (" + detail + ")"
	}
	var ae *apierror.Error
	switch code {
	case TurnErrAgentForbidden:
		ae = apierror.Forbidden("CHAT_AGENT", msg)
	case TurnErrAttachmentFailed, TurnErrAttachmentUnsupported:
		ae = apierror.BadRequest("CHAT_AGENT", msg)
	default:
		ae = apierror.Internal("CHAT_AGENT", msg)
	}
	return ae.WithMeta(metaKeyTurnErrorCode, string(code))
}

func TurnErrorCodeFromErr(err error) TurnErrorCode {
	if err == nil {
		return ""
	}
	if ae, ok := apierror.From(err); ok {
		if v, exists := ae.Meta[metaKeyTurnErrorCode]; exists {
			return TurnErrorCode(v)
		}
	}
	return ""
}

// markTurnError sets the turn status, error, and error message from an error.
func markTurnError(status *string, turnErr *error, turnErrMsg *string, err error) {
	*status = "error"
	*turnErr = err
	if err != nil {
		*turnErrMsg = err.Error()
	}
}
