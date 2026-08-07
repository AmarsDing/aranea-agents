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

// 00:52 会话补充取证：框架对流式响应的每个 chunk 触发一次 afterModel，
// output_policy 每个干净 chunk 写一条 status=ok Info（实测 8287 条/4min），
// 属高频洪泛。ok+chunk 必须采样；blocked（安全事件）与非 chunk 响应逐条。
func TestOutputPolicy_AfterModel_ThrottlesChunkOkLogs(t *testing.T) {
	cl := &countingLogger{}
	o := &OutputPolicyPlugin{
		base: basePlugin{
			name:   "output_policy",
			logger: NewPluginSafeLogger("output_policy", nil, cl),
		},
		cfg: outputPolicyConfig{BlockedPatterns: []string{"secret_key"}, BlockOnViolation: true},
	}

	cleanChunk := &trpcmodel.AfterModelArgs{Response: &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletionChunk,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "clean"}}},
	}}
	for i := 0; i < 250; i++ {
		if _, err := o.afterModel(context.Background(), cleanChunk); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 2 {
		t.Fatalf("clean chunk after_model must be sampled (1st + every 200th): expected 2 logs for 250 calls, got %d", got)
	}

	// blocked 是安全事件，逐条保留。
	blockedChunk := &trpcmodel.AfterModelArgs{Response: &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletionChunk,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "the secret_key here"}}},
	}}
	for i := 0; i < 3; i++ {
		if _, err := o.afterModel(context.Background(), blockedChunk); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 5 {
		t.Fatalf("blocked responses must be logged every time: expected 5 total logs, got %d", got)
	}

	// 非 chunk 的完整干净响应逐条保留。
	cleanFull := &trpcmodel.AfterModelArgs{Response: &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletion,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "clean"}}},
	}}
	for i := 0; i < 3; i++ {
		if _, err := o.afterModel(context.Background(), cleanFull); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 8 {
		t.Fatalf("full clean responses must be logged every time: expected 8 total logs, got %d", got)
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
