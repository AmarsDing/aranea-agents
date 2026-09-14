package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// usageEvent 构造一条携带 usage 的框架事件（Response.ID 非空，走 id: 计费键）。
func usageEvent(id, model string, prompt, completion int) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			ID:     id,
			Model:  model,
			Object: trpcmodel.ObjectTypeChatCompletionChunk,
			Usage:  &trpcmodel.Usage{PromptTokens: prompt, CompletionTokens: completion},
		},
	}
}

func TestGraphUsageAccumulator_NilRecorder(t *testing.T) {
	if got := newGraphUsageAccumulator("s", "e", "g", nil, nil); got != nil {
		t.Fatalf("nil recorder should yield nil accumulator, got %v", got)
	}
}

// TestGraphUsageAccumulator_MaxPerBillingKey 验证计量主口径：
// 同一计费键（Response.ID）内 prompt 恒定、completion 按块累计上报，取各自
// 最大值；不同计费键为不同调用；按模型聚合落账。
func TestGraphUsageAccumulator_MaxPerBillingKey(t *testing.T) {
	var recs []biz.GraphRunUsageInput
	rec := biz.GraphUsageRecorderFunc(func(_ context.Context, in biz.GraphRunUsageInput) error {
		recs = append(recs, in)
		return nil
	})
	acc := newGraphUsageAccumulator("twin-x", "exec-1", "g1", rec, nil)

	// 第一次调用（r1）的流式增量快照：completion 单调增长，取 MAX=50。
	acc.observe(usageEvent("r1", "deepseek-chat", 100, 10))
	acc.observe(usageEvent("r1", "deepseek-chat", 100, 50))
	// 第二次调用（r2，工具循环下一轮 prompt 增长）。
	acc.observe(usageEvent("r2", "deepseek-chat", 300, 40))
	// 另一模型节点。
	acc.observe(usageEvent("r3", "qwen-max", 50, 5))
	// 无 usage / 零 token / 空事件：不计。
	acc.observe(&trpcevent.Event{Response: &trpcmodel.Response{ID: "r4", Model: "x"}})
	acc.observe(usageEvent("r5", "x", 0, 0))
	acc.observe(nil)

	acc.flush(context.Background(), "success")

	if len(recs) != 2 {
		t.Fatalf("expect 2 model rows, got %d: %+v", len(recs), recs)
	}
	byModel := map[string]biz.GraphRunUsageInput{}
	for _, r := range recs {
		byModel[r.Model] = r
	}
	ds := byModel["deepseek-chat"]
	// prompt 400 = 100（r1）+ 300（r2）；completion 90 = max(10,50) + 40。
	if ds.PromptTok != 400 || ds.CompletionTok != 90 {
		t.Errorf("deepseek tokens = %d/%d, want 400/90", ds.PromptTok, ds.CompletionTok)
	}
	if ds.LLMCalls != 2 {
		t.Errorf("deepseek calls = %d, want 2", ds.LLMCalls)
	}
	if ds.SessionID != "twin-x" || ds.RunID != "exec-1" || ds.GraphID != "g1" {
		t.Errorf("row identity mismatch: %+v", ds)
	}
	if ds.Status != "success" || ds.UsageSource != "streaming" {
		t.Errorf("row status/source = %q/%q", ds.Status, ds.UsageSource)
	}
	qw := byModel["qwen-max"]
	if qw.PromptTok != 50 || qw.CompletionTok != 5 || qw.LLMCalls != 1 {
		t.Errorf("qwen row mismatch: %+v", qw)
	}
}

// TestGraphUsageAccumulator_CumulativeChunksSameID 复刻部分提供方「最终块
// 才带完整累计 usage」的行为：同 ID 多块，取 completion 最大值（末块）。
func TestGraphUsageAccumulator_CumulativeChunksSameID(t *testing.T) {
	var recs []biz.GraphRunUsageInput
	rec := biz.GraphUsageRecorderFunc(func(_ context.Context, in biz.GraphRunUsageInput) error {
		recs = append(recs, in)
		return nil
	})
	acc := newGraphUsageAccumulator("twin-x", "exec-1", "g1", rec, nil)

	// 中间块不带 usage（零值被跳过），仅首块与末块带累计快照。
	acc.observe(usageEvent("call-1", "deepseek-chat", 128, 0))
	acc.observe(usageEvent("call-1", "deepseek-chat", 128, 42))
	acc.observe(usageEvent("call-1", "deepseek-chat", 128, 187))

	acc.flush(context.Background(), "success")

	if len(recs) != 1 {
		t.Fatalf("expect 1 row, got %d: %+v", len(recs), recs)
	}
	got := recs[0]
	if got.PromptTok != 128 || got.CompletionTok != 187 || got.LLMCalls != 1 {
		t.Errorf("cumulative chunks mismatch: %+v", got)
	}
}

// TestGraphUsageAccumulator_FallbackRoundKey 验证无 Response.ID 时的退化
// 计费键（author|model|prompt）：prompt 变化 = 新调用；同 prompt 取 MAX。
func TestGraphUsageAccumulator_FallbackRoundKey(t *testing.T) {
	var recs []biz.GraphRunUsageInput
	rec := biz.GraphUsageRecorderFunc(func(_ context.Context, in biz.GraphRunUsageInput) error {
		recs = append(recs, in)
		return nil
	})
	acc := newGraphUsageAccumulator("twin-x", "exec-1", "g1", rec, nil)

	noID := func(author, model string, prompt, completion int) *trpcevent.Event {
		return &trpcevent.Event{
			Author: author,
			Response: &trpcmodel.Response{
				Model:  model,
				Object: trpcmodel.ObjectTypeChatCompletionChunk,
				Usage:  &trpcmodel.Usage{PromptTokens: prompt, CompletionTokens: completion},
			},
		}
	}
	// 同一 agent 同一 round（prompt 恒定）：completion 取 MAX。
	acc.observe(noID("analyst", "deepseek-chat", 200, 20))
	acc.observe(noID("analyst", "deepseek-chat", 200, 66))
	// prompt 变化 → 新计费调用（工具循环下一轮）。
	acc.observe(noID("analyst", "deepseek-chat", 480, 30))
	// 不同 agent 同 prompt 也是不同调用（author 参与键）。
	acc.observe(noID("reviewer", "deepseek-chat", 200, 7))

	acc.flush(context.Background(), "success")

	if len(recs) != 1 {
		t.Fatalf("expect 1 model row, got %d: %+v", len(recs), recs)
	}
	got := recs[0]
	// prompt 880 = 200 + 480 + 200；completion 103 = 66 + 30 + 7。
	if got.PromptTok != 880 || got.CompletionTok != 103 || got.LLMCalls != 3 {
		t.Errorf("fallback key mismatch: %+v", got)
	}
}

// TestGraphUsageAccumulator_ObjectGate 非 chat.completion 对象携带 usage
// 时不计（与 agent.isChatCompletionStreamObject 同口径）。
func TestGraphUsageAccumulator_ObjectGate(t *testing.T) {
	called := false
	rec := biz.GraphUsageRecorderFunc(func(_ context.Context, in biz.GraphRunUsageInput) error {
		called = true
		return nil
	})
	acc := newGraphUsageAccumulator("twin-x", "exec-1", "g1", rec, nil)

	acc.observe(&trpcevent.Event{
		Response: &trpcmodel.Response{
			ID:     "r1",
			Model:  "deepseek-chat",
			Object: "chat.completion.chunk.tool",
			Usage:  &trpcmodel.Usage{PromptTokens: 10, CompletionTokens: 5},
		},
	})
	acc.flush(context.Background(), "success")
	if called {
		t.Fatal("non-chat-completion object must be gated out")
	}
}

func TestGraphUsageAccumulator_NoUsageNoFlush(t *testing.T) {
	called := false
	rec := biz.GraphUsageRecorderFunc(func(_ context.Context, in biz.GraphRunUsageInput) error {
		called = true
		return nil
	})
	acc := newGraphUsageAccumulator("twin-x", "exec-1", "g1", rec, nil)
	acc.observe(usageEvent("r1", "m", 0, 0))
	acc.flush(context.Background(), "success")
	if called {
		t.Fatal("zero-usage run must not record any row")
	}
}
