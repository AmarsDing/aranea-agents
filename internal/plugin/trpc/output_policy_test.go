package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func newTestOutputPolicy(cfg outputPolicyConfig) *OutputPolicyPlugin {
	return &OutputPolicyPlugin{
		base: basePlugin{
			name:   "output_policy",
			logger: NewPluginSafeLogger("output_policy", nil, loggateway.NewNoop()),
		},
		cfg: cfg,
	}
}

func TestOutputPolicy_AfterModel_BlocksViolation(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockedPatterns:       []string{"secret_key"},
		BlockOnViolation:      true,
		DangerousCommandCheck: false,
	})
	args := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "here is the secret_key=abc123"},
			}},
		},
	}
	res, err := o.afterModel(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse == nil {
		t.Fatal("expected CustomResponse (blocked), got nil")
	}
	if len(res.CustomResponse.Choices) == 0 {
		t.Fatal("expected at least one choice in blocked response")
	}
	if res.CustomResponse.Choices[0].Message.Content == "here is the secret_key=abc123" {
		t.Fatal("blocked response should not contain original violating text")
	}
}

func TestOutputPolicy_AfterModel_PassesClean(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockedPatterns:       []string{"secret_key"},
		BlockOnViolation:      true,
		DangerousCommandCheck: false,
	})
	args := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "Hello, how can I help?"},
			}},
		},
	}
	res, err := o.afterModel(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse != nil {
		t.Fatal("clean text should not be blocked")
	}
}

func TestOutputPolicy_OnEvent_BlocksStreamingViolation(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockedPatterns:       []string{"secret_key"},
		BlockOnViolation:      true,
		ReplacementMessage:    "[BLOCKED]",
		DangerousCommandCheck: false,
	})
	e := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{Content: "the secret_key is xyz"},
			}},
		},
	}
	out, err := o.onEvent(context.Background(), &trpcagent.Invocation{}, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Response == nil {
		t.Fatal("expected non-nil event with modified response")
	}
	if len(out.Response.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	ch := out.Response.Choices[0]
	if ch.Delta.Content != "[BLOCKED]" {
		t.Fatalf("expected delta content to be replaced with [BLOCKED], got %q", ch.Delta.Content)
	}
	if ch.FinishReason == nil || *ch.FinishReason != "content_filter" {
		t.Fatal("expected finish_reason = content_filter")
	}
}

func TestOutputPolicy_OnEvent_PassesWhenNoBlock(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockedPatterns:       []string{"secret_key"},
		BlockOnViolation:      false,
		DangerousCommandCheck: false,
	})
	e := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{Content: "the secret_key is xyz"},
			}},
		},
	}
	out, err := o.onEvent(context.Background(), &trpcagent.Invocation{}, e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || out.Response == nil {
		t.Fatal("expected event to pass through")
	}
	original := e.Response.Choices[0].Delta.Content
	passed := out.Response.Choices[0].Delta.Content
	if passed != original {
		t.Fatalf("when block_on_violation=false, content should pass unchanged; got %q", passed)
	}
}
