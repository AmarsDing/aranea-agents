package service

import (
	"testing"

	"aranea-agents/internal/event"
)

func TestEnvelopeErrorFromTurn(t *testing.T) {
	tests := []struct {
		name      string
		code      TurnErrorCode
		detail    string
		wantType  string
		wantCode  string
		wantHint  string
		wantInMsg string
	}{
		{
			name:      "empty_code_fallback",
			code:      "",
			detail:    "",
			wantType:  "run_error",
			wantCode:  "",
			wantHint:  "持续失败请查看 Monitor 中的运行日志",
			wantInMsg: "请求处理失败，请稍后重试",
		},
		{
			name:      "agent_build_failed",
			code:      TurnErrAgentBuildFailed,
			detail:    "",
			wantType:  "AGENT_BUILD_FAILED",
			wantCode:  "AGENT_BUILD_FAILED",
			wantHint:  "检查智能体配置、模型与工具绑定后重试",
			wantInMsg: "智能体构建失败",
		},
		{
			name:      "llm_call_failed_with_detail",
			code:      TurnErrLLMCallFailed,
			detail:    "timeout 5s",
			wantType:  "LLM_CALL_FAILED",
			wantCode:  "LLM_CALL_FAILED",
			wantHint:  "可切换模型或稍后重试；持续失败请查看 Monitor 用量事件",
			wantInMsg: "(timeout 5s)",
		},
		{
			name:      "turn_timeout",
			code:      TurnErrTurnTimeout,
			detail:    "",
			wantType:  "TURN_TIMEOUT",
			wantCode:  "TURN_TIMEOUT",
			wantHint:  "缩短提问或切换响应更快的模型",
			wantInMsg: "响应超时",
		},
		{
			name:      "first_byte_timeout",
			code:      TurnErrFirstByteTimeout,
			detail:    "",
			wantType:  "FIRST_BYTE_TIMEOUT",
			wantCode:  "FIRST_BYTE_TIMEOUT",
			wantHint:  "缩短提问或切换响应更快的模型",
			wantInMsg: "模型响应过慢",
		},
		{
			name:      "agent_forbidden",
			code:      TurnErrAgentForbidden,
			detail:    "",
			wantType:  "AGENT_FORBIDDEN",
			wantCode:  "AGENT_FORBIDDEN",
			wantHint:  "确认当前账号有权访问该智能体",
			wantInMsg: "无权访问该智能体",
		},
		{
			name:      "empty_reply",
			code:      TurnErrEmptyReply,
			detail:    "",
			wantType:  "EMPTY_REPLY",
			wantCode:  "EMPTY_REPLY",
			wantHint:  "调整提问方式或关闭过于严格的工具限制",
			wantInMsg: "智能体未产生响应",
		},
		{
			name:      "attachment_failed",
			code:      TurnErrAttachmentFailed,
			detail:    "file too large",
			wantType:  "ATTACHMENT_FAILED",
			wantCode:  "ATTACHMENT_FAILED",
			wantHint:  "确认附件格式与大小，或移除附件后重试",
			wantInMsg: "(file too large)",
		},
		{
			name:      "attachment_unsupported",
			code:      TurnErrAttachmentUnsupported,
			detail:    "",
			wantType:  "ATTACHMENT_UNSUPPORTED",
			wantCode:  "ATTACHMENT_UNSUPPORTED",
			wantHint:  "移除不支持的附件，或切换到支持视觉/多模态的模型",
			wantInMsg: "当前模型不支持该附件类型",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envelopeErrorFromTurn(tt.code, tt.detail)
			if got == nil {
				t.Fatal("expected non-nil EnvelopeError")
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", got.Hint, tt.wantHint)
			}
		})
	}
}

func TestEnvelopeErrorFromTurn_EmptyCodeReturnsRunError(t *testing.T) {
	got := envelopeErrorFromTurn("", "some detail")
	if got.Type != "run_error" {
		t.Errorf("Type = %q, want run_error", got.Type)
	}
	if got.Code != "" {
		t.Errorf("Code = %q, want empty", got.Code)
	}
	if got.Message != "请求处理失败，请稍后重试" {
		t.Errorf("Message = %q, want fallback message", got.Message)
	}
}

func TestEnvelopeErrorFromTurn_DetailAppended(t *testing.T) {
	got := envelopeErrorFromTurn(TurnErrLLMCallFailed, "conn refused")
	if got.Message != "模型调用失败，请稍后重试或切换模型 (conn refused)" {
		t.Errorf("Message = %q", got.Message)
	}
}

func TestEnvelopeErrorFromTurn_NoDetailNoParens(t *testing.T) {
	got := envelopeErrorFromTurn(TurnErrLLMCallFailed, "")
	if got.Message != "模型调用失败，请稍后重试或切换模型" {
		t.Errorf("Message = %q", got.Message)
	}
}

func TestTurnErrorHint(t *testing.T) {
	tests := []struct {
		name string
		code TurnErrorCode
		want string
	}{
		{"agent_build_failed", TurnErrAgentBuildFailed, "检查智能体配置、模型与工具绑定后重试"},
		{"attachment_failed", TurnErrAttachmentFailed, "确认附件格式与大小，或移除附件后重试"},
		{"attachment_unsupported", TurnErrAttachmentUnsupported, "移除不支持的附件，或切换到支持视觉/多模态的模型"},
		{"llm_call_failed", TurnErrLLMCallFailed, "可切换模型或稍后重试；持续失败请查看 Monitor 用量事件"},
		{"turn_timeout", TurnErrTurnTimeout, "缩短提问或切换响应更快的模型"},
		{"first_byte_timeout", TurnErrFirstByteTimeout, "缩短提问或切换响应更快的模型"},
		{"empty_reply", TurnErrEmptyReply, "调整提问方式或关闭过于严格的工具限制"},
		{"agent_forbidden", TurnErrAgentForbidden, "确认当前账号有权访问该智能体"},
		{"unknown_code", TurnErrorCode("UNKNOWN"), ""},
		{"stream_preview_failed", TurnErrStreamPreviewFailed, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := turnErrorHint(tt.code)
			if got != tt.want {
				t.Errorf("turnErrorHint(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestEnvelopeErrorFromTurn_AllKnownCodesProduceNonNil(t *testing.T) {
	codes := []TurnErrorCode{
		TurnErrAgentBuildFailed,
		TurnErrAttachmentFailed,
		TurnErrAttachmentUnsupported,
		TurnErrLLMCallFailed,
		TurnErrTurnTimeout,
		TurnErrEmptyReply,
		TurnErrFirstByteTimeout,
		TurnErrAgentForbidden,
		TurnErrStreamPreviewFailed,
	}
	for _, code := range codes {
		got := envelopeErrorFromTurn(code, "")
		if got == nil {
			t.Errorf("envelopeErrorFromTurn(%q) = nil, want non-nil", code)
		}
		if got.Type != string(code) {
			t.Errorf("Type = %q, want %q", got.Type, code)
		}
		if got.Code != string(code) {
			t.Errorf("Code = %q, want %q", got.Code, code)
		}
	}
}

func TestEnvelopeErrorFromTurn_ReturnType(t *testing.T) {
	got := envelopeErrorFromTurn(TurnErrAgentBuildFailed, "")
	if _, ok := interface{}(got).(*event.EnvelopeError); !ok {
		t.Fatal("expected *event.EnvelopeError return type")
	}
}
