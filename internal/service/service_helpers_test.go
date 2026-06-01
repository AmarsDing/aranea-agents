package service_test

import (
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestFirstNonEmptyString(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, ""},
		{"all empty", []string{"", " ", "  "}, ""},
		{"first non empty", []string{"hello", "", "world"}, "hello"},
		{"middle non empty", []string{"", "middle", "last"}, "middle"},
		{"whitespace only counts as empty", []string{"  ", "\t", "\n"}, ""},
		{"trimmed whitespace returns inner", []string{"  spaced  "}, "spaced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.FirstNonEmptyString(tt.args...); got != tt.want {
				t.Errorf("firstNonEmptyString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateObservatoryPayload(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"empty string", "", ""},
		{"under limit", "hello", "hello"},
		{"at limit", string(make([]byte, service.ObservatoryPayloadMaxBytes)), string(make([]byte, service.ObservatoryPayloadMaxBytes))},
		{"over limit", string(make([]byte, service.ObservatoryPayloadMaxBytes+10)), string(make([]byte, service.ObservatoryPayloadMaxBytes)) + "…[truncated]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.TruncateObservatoryPayload(tt.s); got != tt.want {
				if len(got) > 80 {
					t.Errorf("truncateObservatoryPayload() len=%d, want len=%d", len(got), len(tt.want))
				} else {
					t.Errorf("truncateObservatoryPayload() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestPickTitleModel(t *testing.T) {
	tests := []struct {
		name   string
		models []biz.ProviderModel
		want   string
	}{
		{
			"matching mini",
			[]biz.ProviderModel{
				{Model: "gpt-4o"},
				{Model: "gpt-4o-mini"},
			},
			"gpt-4o-mini",
		},
		{
			"matching flash",
			[]biz.ProviderModel{
				{Model: "claude-3-opus"},
				{Model: "gemini-2.0-flash"},
			},
			"gemini-2.0-flash",
		},
		{
			"no matching model falls back to first",
			[]biz.ProviderModel{
				{Model: "gpt-4o"},
				{Model: "claude-3-opus"},
			},
			"gpt-4o",
		},
		{
			"matching lite",
			[]biz.ProviderModel{
				{Model: "deepseek-lite"},
				{Model: "deepseek-v3"},
			},
			"deepseek-lite",
		},
		{
			"matching small",
			[]biz.ProviderModel{
				{Model: "llama-small"},
				{Model: "llama-large"},
			},
			"llama-small",
		},
		{
			"case insensitive match",
			[]biz.ProviderModel{
				{Model: "GPT-4O-MINI"},
			},
			"GPT-4O-MINI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.PickTitleModel(tt.models)
			if got.Model != tt.want {
				t.Errorf("pickTitleModel() = %q, want %q", got.Model, tt.want)
			}
		})
	}
}

func TestErrString(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, ""},
		{"non nil error", errors.New("something failed"), "something failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.ErrString(tt.err); got != tt.want {
				t.Errorf("errString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatIngressRecording(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		enabled bool
	}{
		{"enabled with 1", "1", true},
		{"enabled with true", "true", true},
		{"enabled with yes", "yes", true},
		{"enabled with on", "on", true},
		{"disabled with 0", "0", false},
		{"disabled with random", "maybe", false},
		{"disabled with empty", "", false},
		{"case insensitive TRUE", "TRUE", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CHAT_RECORD_USAGE_INGRESS", tt.envVal)
			if got := service.ChatIngressRecordingEnabled(); got != tt.enabled {
				t.Errorf("chatIngressRecordingEnabled() = %v, want %v", got, tt.enabled)
			}
			if got := service.ChatIngressRecordingDisabled(); got != !tt.enabled {
				t.Errorf("chatIngressRecordingDisabled() = %v, want %v", got, !tt.enabled)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		def    int
		want   int
	}{
		{"valid value", "42", 10, 42},
		{"invalid value", "abc", 10, 10},
		{"empty uses default", "", 10, 10},
		{"zero uses default", "0", 10, 10},
		{"negative uses default", "-5", 10, 10},
		{"whitespace trimmed", "  7  ", 10, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_ENV_INT_KEY", tt.envVal)
			if got := service.EnvInt("TEST_ENV_INT_KEY", tt.def); got != tt.want {
				t.Errorf("envInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHasFileAttachment(t *testing.T) {
	tests := []struct {
		name string
		refs []artifactbiz.Ref
		want bool
	}{
		{"no refs", nil, false},
		{"image only refs", []artifactbiz.Ref{
			{MimeType: "image/png"},
			{MimeType: "image/jpeg"},
		}, false},
		{"document ref", []artifactbiz.Ref{
			{MimeType: "application/pdf"},
		}, true},
		{"mixed refs", []artifactbiz.Ref{
			{MimeType: "image/png"},
			{MimeType: "text/plain"},
		}, true},
		{"empty mime", []artifactbiz.Ref{
			{MimeType: ""},
		}, false},
		{"image with uppercase prefix", []artifactbiz.Ref{
			{MimeType: "Image/PNG"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.HasFileAttachment(tt.refs); got != tt.want {
				t.Errorf("hasFileAttachment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphExecutionFinishErr(t *testing.T) {
	tests := []struct {
		name    string
		exec    *biz.GraphExecution
		wantNil bool
		wantMsg string
	}{
		{"nil input", nil, true, ""},
		{"failed with message", &biz.GraphExecution{Status: "failed", ErrorMessage: "node crashed"}, false, "node crashed"},
		{"failed without message", &biz.GraphExecution{Status: "failed"}, false, "graph execution failed"},
		{"cancelled with message", &biz.GraphExecution{Status: "cancelled", ErrorMessage: "user abort"}, false, "user abort"},
		{"cancelled without message", &biz.GraphExecution{Status: "cancelled"}, false, "graph execution cancelled"},
		{"running status", &biz.GraphExecution{Status: "running"}, true, ""},
		{"status with whitespace", &biz.GraphExecution{Status: " failed "}, false, "graph execution  failed "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.GraphExecutionFinishErr(tt.exec)
			if tt.wantNil {
				if err != nil {
					t.Errorf("graphExecutionFinishErr() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("graphExecutionFinishErr() = nil, want error with message %q", tt.wantMsg)
			}
			ke, ok := err.(*kerrors.Error)
			if !ok {
				t.Fatalf("graphExecutionFinishErr() = %T, want *kerrors.Error", err)
			}
			if ke.Message != tt.wantMsg {
				t.Errorf("graphExecutionFinishErr() message = %q, want %q", ke.Message, tt.wantMsg)
			}
		})
	}
}
