package plugintrpc

import (
	"context"
	"strings"
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

// ---------- P4（2026-08-30）：标记剥离 / 终态解耦 / 防御性白名单 ----------

// P4 标记剥离：兜底文案不得暴露插件名/命中模式等内部细节（S14 实证
// "output_policy: blocked content matching dangerous_command" 泄漏给用户）。
func TestOutputPolicy_DefaultReplacementMessage_StripsInternalMarker(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	args := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "执行 rm -rf / 即可清理"},
			}},
		},
	}
	res, err := o.afterModel(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse == nil {
		t.Fatal("expected block")
	}
	msg := res.CustomResponse.Choices[0].Message.Content
	for _, internal := range []string{"output_policy", "dangerous_command", "blocked content"} {
		if strings.Contains(msg, internal) {
			t.Fatalf("user-visible message must not leak internal marker %q, got %q", internal, msg)
		}
	}
}

// P4 终态解耦：拦截响应必须是完整终态（Done + finish_reason=content_filter），
// 否则调用侧可能按残缺输出触发续写/重试（S14 h3 89s 内被反复拦截 5 次）。
func TestBlockedModelResponse_IsTerminal(t *testing.T) {
	resp := blockedModelResponse("x")
	if !resp.Done {
		t.Fatal("blocked response must be Done=true")
	}
	if resp.Object != trpcmodel.ObjectTypeChatCompletion {
		t.Fatalf("blocked response Object must be chat_completion, got %q", resp.Object)
	}
	fr := resp.Choices[0].FinishReason
	if fr == nil || *fr != "content_filter" {
		t.Fatalf("blocked response must carry finish_reason=content_filter, got %v", fr)
	}
}

// P4 防御性白名单：讲解/警示语境提及危险命令不拦截。
func TestOutputPolicy_DefensiveMention_Exempt(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	defensive := []string{
		"千万不要执行 rm -rf /，它会递归删除所有文件，非常危险。",
		"rm -rf 是危险命令，为什么危险？因为它不做任何确认。",
		"什么是 drop table？这是一种高危 SQL 操作，务必避免在生产执行。",
	}
	for _, text := range defensive {
		args := &trpcmodel.AfterModelArgs{
			Response: &trpcmodel.Response{
				Choices: []trpcmodel.Choice{{
					Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: text},
				}},
			},
		}
		res, err := o.afterModel(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.CustomResponse != nil {
			t.Fatalf("defensive/educational mention must not be blocked: %q", text)
		}
	}
}

// P4 白名单边界：无防御语境时，代码块 / shell 提示符行 / 祈使句形态的
// 危险命令照常拦截（保守方向）。
func TestOutputPolicy_DangerousCommand_StillBlocked(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	blocked := []string{
		"清理磁盘：\n```\nrm -rf /tmp/data\n```",
		"$ rm -rf /",
		"执行 rm -rf / 即可释放空间",
	}
	for _, text := range blocked {
		args := &trpcmodel.AfterModelArgs{
			Response: &trpcmodel.Response{
				Choices: []trpcmodel.Choice{{
					Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: text},
				}},
			},
		}
		res, err := o.afterModel(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.CustomResponse == nil {
			t.Fatalf("actionable dangerous command must be blocked: %q", text)
		}
	}
}

// P4-r6 粘性豁免：防御语境下的代码块示范属正常科普内容（S14 h3 实证：
// 「如何防范 rm -rf 误删」的回答天然带 fenced 示例，240 字窗口滑出防御
// 标记后误拦）。防御标记见过一次，本响应后续命中一律豁免。
func TestOutputPolicy_DefensiveContext_FencedExampleExempt(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	// 非流式：整段判定，防御标记 + fenced 示范 => 放行。
	args := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "rm -rf 非常危险，千万不要执行。错误示范：\n```\nrm -rf /tmp/data\n```"},
			}},
		},
	}
	res, err := o.afterModel(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse != nil {
		t.Fatal("defensive context with fenced example must pass (P4-r6)")
	}
}

// P4-r6 流式粘性：防御标记先于命令 240+ 字（窗口滑出）后，fenced 示范仍豁免。
func TestOutputPolicy_StreamingChunks_StickyDefensiveBeyondWindow(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	inv := &trpcagent.Invocation{InvocationID: "inv-p4-sticky"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	// chunk1 带防御标记；chunk2 用 300 字填充把标记挤出 240 窗口；chunk3 fenced 命令。
	filler := strings.Repeat("说", 300)
	chunks := []string{"千万不要执行危险命令。" + filler, "```\nrm -rf /tmp/data\n```"}
	for i, c := range chunks {
		args := &trpcmodel.AfterModelArgs{
			Response: &trpcmodel.Response{
				Object:  trpcmodel.ObjectTypeChatCompletionChunk,
				Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: c}}},
			},
		}
		res, err := o.afterModel(ctx, args)
		if err != nil {
			t.Fatalf("chunk %d: unexpected error: %v", i, err)
		}
		if res.CustomResponse != nil {
			t.Fatalf("chunk %d must not be blocked (sticky defensive survives window slide)", i)
		}
	}
}

// P4 流式白名单：防御标记与危险命令分属不同 chunk 时，滚动窗口必须让
// 后到的命令 chunk 豁免（S14 h3 的真实形态——标记先于命令若干 chunk）。
func TestOutputPolicy_StreamingChunks_DefensiveWindowExempt(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	inv := &trpcagent.Invocation{InvocationID: "inv-p4-stream"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	chunks := []string{"千万", "不要执行 ", "rm -rf /", "，它会删除所有文件"}
	for i, c := range chunks {
		args := &trpcmodel.AfterModelArgs{
			Response: &trpcmodel.Response{
				Object:  trpcmodel.ObjectTypeChatCompletionChunk,
				Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: c}}},
			},
		}
		res, err := o.afterModel(ctx, args)
		if err != nil {
			t.Fatalf("chunk %d: unexpected error: %v", i, err)
		}
		if res.CustomResponse != nil {
			t.Fatalf("chunk %d %q must not be blocked (defensive context in window)", i, c)
		}
	}
}

// P4 流式拦截后窗口重置：已拦截内容不再参与后续 chunk 判定，防同一命令
// 残留在窗口内导致后续干净 chunk 被连锁误拦。
func TestOutputPolicy_StreamingWindow_ResetsAfterBlock(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	inv := &trpcagent.Invocation{InvocationID: "inv-p4-reset"}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	mkChunk := func(c string) *trpcmodel.AfterModelArgs {
		return &trpcmodel.AfterModelArgs{
			Response: &trpcmodel.Response{
				Object:  trpcmodel.ObjectTypeChatCompletionChunk,
				Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: c}}},
			},
		}
	}
	// chunk1：裸指令，拦截。
	res, err := o.afterModel(ctx, mkChunk("rm -rf /"))
	if err != nil || res.CustomResponse == nil {
		t.Fatalf("bare command chunk must be blocked, res=%+v err=%v", res, err)
	}
	// chunk2：命令已重置出窗口，干净文本不得被连锁误拦。
	res, err = o.afterModel(ctx, mkChunk("后续说明文字"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse != nil {
		t.Fatal("clean chunk after block must pass (window was reset)")
	}
}

// P4 白名单不放宽严格清单：管理员配置的 blocked_patterns 即使伴随防御
// 标记也照常拦截。
func TestOutputPolicy_BlockedPatterns_RemainStrict(t *testing.T) {
	o := newTestOutputPolicy(outputPolicyConfig{
		BlockedPatterns:       []string{"secret_key"},
		BlockOnViolation:      true,
		DangerousCommandCheck: true,
	})
	args := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "注意：千万不要泄露 secret_key，这很危险"},
			}},
		},
	}
	res, err := o.afterModel(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse == nil {
		t.Fatal("admin blocked_patterns must remain strict (no defensive exemption)")
	}
}
