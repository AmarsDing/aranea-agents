package service

import (
	chatagent "aranea-agents/internal/agent"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// defaultTurnTimeout is the maximum wall-clock duration for a single chat turn.
var defaultTurnTimeout = chatagent.DefaultTurnTimeout

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

func TurnError(code TurnErrorCode, detail string) error {
	msg := turnErrorMessages[code]
	if detail != "" {
		msg = msg + " (" + detail + ")"
	}
	switch code {
	case TurnErrAgentForbidden:
		return kerrors.Forbidden("CHAT_AGENT", msg)
	case TurnErrAttachmentFailed, TurnErrAttachmentUnsupported:
		return kerrors.BadRequest("CHAT_AGENT", msg)
	default:
		return kerrors.InternalServer("CHAT_AGENT", msg)
	}
}

func TurnErrorCodeFromErr(err error) TurnErrorCode {
	if err == nil {
		return ""
	}
	if ke, ok := err.(*kerrors.Error); ok {
		for code, msg := range turnErrorMessages {
			if ke.Message == msg || len(ke.Message) > len(msg) && ke.Message[:len(msg)] == msg {
				return code
			}
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
