package provider

// shrink_retry_test.go — A7 验收矩阵：恰好 3 次递减（0.7/0.5/0.35）用尽回
// 原错；429/5xx 不触发；流式 overflow 首帧窥视透明重试；请求副本冻结（
// 防 tailoring 双重裁剪）；与 IsContextOverflow marker 单测 pin（billing/
// safety/瞬时均恒 false，与 ClassifyRetry 触发集互斥）。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ---------------------------------------------------------------------------
// stubs
// ---------------------------------------------------------------------------

// shrinkStubModel 模拟 provider 模型；mutate=true 时模拟框架 token
// tailoring 的 request.Messages 就地改写（modeltailoring.ApplyResult 同构
// 危害），验证装饰器每次尝试在请求副本上执行。
type shrinkStubModel struct {
	name    string
	err     error
	frames  []*trpcmodel.Response
	calls   int
	msgLens []int
	mutate  bool
}

func (m *shrinkStubModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: m.name} }

func (m *shrinkStubModel) GenerateContent(_ context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	m.calls++
	m.msgLens = append(m.msgLens, len(request.Messages))
	if m.mutate && len(request.Messages) > 1 {
		request.Messages = request.Messages[:1] // 就地裁剪危害模拟
	}
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, len(m.frames))
	for _, r := range m.frames {
		ch <- r
	}
	close(ch)
	return ch, nil
}

var shrinkOverflowErr = errors.New("Error 400: this model's maximum context length is 8192 tokens; request too large")

func newOverflowStub(name string) *shrinkStubModel {
	return &shrinkStubModel{name: name, err: shrinkOverflowErr}
}

func streamOverflowErrFrame(message string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Done:  true,
		Error: &trpcmodel.ResponseError{Message: message, Type: trpcmodel.ErrorTypeAPIError},
	}
}

func contentFrame(text string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Choices: []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: text}}},
	}
}

func drainResponses(ch <-chan *trpcmodel.Response) []*trpcmodel.Response {
	var out []*trpcmodel.Response
	for r := range ch {
		out = append(out, r)
	}
	return out
}

// captureWarnLogger 是 loggateway.Logger 的内存实现，仅收 Warn（A7 审定：
// 每步收缩只打 Warn，不刷 Info）。
type captureWarnLogger struct{ warns []string }

func (l *captureWarnLogger) Debug(string, ...loggateway.Field)          {}
func (l *captureWarnLogger) Info(string, ...loggateway.Field)           {}
func (l *captureWarnLogger) Warn(msg string, _ ...loggateway.Field)     { l.warns = append(l.warns, msg) }
func (l *captureWarnLogger) Error(string, ...loggateway.Field)          {}
func (l *captureWarnLogger) With(...loggateway.Field) loggateway.Logger { return l }

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// 默认零行为变更：Rebuild=nil 或 Baseline<=0 时原样返回 inner（不开关即零开销）。
func TestWrapModelWithShrinkRetry_DisabledPassThrough(t *testing.T) {
	inner := newOverflowStub("inner")
	if got := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{}); got != trpcmodel.Model(inner) {
		t.Fatal("zero options must pass inner through unchanged")
	}
	if got := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
	}); got != trpcmodel.Model(inner) {
		t.Fatal("nil Rebuild must pass inner through unchanged")
	}
	if got := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		Rebuild: func(int) (trpcmodel.Model, error) { return inner, nil },
	}); got != trpcmodel.Model(inner) {
		t.Fatal("Baseline<=0 must pass inner through unchanged")
	}
	if got := WrapModelWithShrinkRetry(nil, ShrinkRetryOptions{}); got != nil {
		t.Fatal("nil inner must stay nil")
	}
}

// 主验收：overflow 连发 → 恰好 3 次递减重建（700/500/350），用尽回原错；
// 总尝试数 = 1 原始 + 3 收缩 = 4。
func TestShrinkRetry_ExactlyThreeShrinksThenOriginalError(t *testing.T) {
	inner := newOverflowStub("base")
	steps := []*shrinkStubModel{newOverflowStub("s0.7"), newOverflowStub("s0.5"), newOverflowStub("s0.35")}
	var budgets []int
	i := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild: func(budget int) (trpcmodel.Model, error) {
			budgets = append(budgets, budget)
			m := steps[i]
			i++
			return m, nil
		},
		Logger: loggateway.NewNoop(),
	})

	_, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{
		Messages: []trpcmodel.Message{trpcmodel.NewUserMessage("hi")},
	})
	if !errors.Is(err, shrinkOverflowErr) {
		t.Fatalf("exhausted retry must return the original error, got %v", err)
	}
	if len(budgets) != 3 || budgets[0] != 700 || budgets[1] != 500 || budgets[2] != 350 {
		t.Fatalf("shrink budgets=%v, want exactly [700 500 350] (0.7/0.5/0.35 of 1000)", budgets)
	}
	if inner.calls != 1 || steps[0].calls != 1 || steps[1].calls != 1 || steps[2].calls != 1 {
		t.Fatalf("each attempt must fire exactly once: %d/%d/%d/%d", inner.calls, steps[0].calls, steps[1].calls, steps[2].calls)
	}
}

// 第二级收缩成功后内容原样透出；第 3 级不再触发。
func TestShrinkRetry_SucceedsAfterSecondShrink(t *testing.T) {
	inner := newOverflowStub("base")
	step1 := newOverflowStub("s0.7")
	step2 := &shrinkStubModel{name: "s0.5", frames: []*trpcmodel.Response{
		contentFrame("hello"), {Done: true},
	}}
	var budgets []int
	i := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild: func(budget int) (trpcmodel.Model, error) {
			budgets = append(budgets, budget)
			m := []trpcmodel.Model{step1, step2}[i]
			i++
			return m, nil
		},
	})
	ch, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{
		Messages: []trpcmodel.Message{trpcmodel.NewUserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("second shrink must succeed, got err=%v", err)
	}
	got := drainResponses(ch)
	if len(got) != 2 || got[0].Choices[0].Delta.Content != "hello" || !got[1].Done {
		t.Fatalf("stream content must pass through after successful shrink: %+v", got)
	}
	if len(budgets) != 2 || budgets[0] != 700 || budgets[1] != 500 {
		t.Fatalf("budgets=%v, want [700 500] (success on second step)", budgets)
	}
}

// 流式 overflow（provider 拒绝 oversize 的主流形态）：首帧窥视透明重试，
// 成功模型的完整流原样交付。
func TestShrinkRetry_StreamedOverflowRetried(t *testing.T) {
	inner := &shrinkStubModel{name: "base", frames: []*trpcmodel.Response{
		streamOverflowErrFrame("maximum context length exceeded"),
	}}
	step1 := &shrinkStubModel{name: "s0.7", frames: []*trpcmodel.Response{
		contentFrame("shrunk-ok"), {Done: true},
	}}
	var budgets []int
	calls := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild: func(budget int) (trpcmodel.Model, error) {
			budgets = append(budgets, budget)
			calls++
			return step1, nil
		},
	})
	ch, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("streamed overflow must be retried, err=%v", err)
	}
	got := drainResponses(ch)
	if len(got) != 2 || got[0].Choices[0].Delta.Content != "shrunk-ok" {
		t.Fatalf("shrunk stream must be delivered verbatim: %+v", got)
	}
	if calls != 1 || budgets[0] != 700 {
		t.Fatalf("expect exactly one rebuild at 700, got calls=%d budgets=%v", calls, budgets)
	}
}

// 用尽（3 级全部 overflow）回放"原始第一帧"——不是最后一级的新文案。
func TestShrinkRetry_StreamedExhaustionReplaysOriginalError(t *testing.T) {
	inner := &shrinkStubModel{name: "base", frames: []*trpcmodel.Response{
		streamOverflowErrFrame("ORIGINAL: maximum context length exceeded"),
	}}
	mk := func(name string) *shrinkStubModel {
		return &shrinkStubModel{name: name, frames: []*trpcmodel.Response{
			streamOverflowErrFrame("RETRY-" + name + ": maximum context length exceeded"),
		}}
	}
	steps := []*shrinkStubModel{mk("s0.7"), mk("s0.5"), mk("s0.35")}
	i := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { m := steps[i]; i++; return m, nil },
	})
	ch, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("streamed exhaustion must surface as stream error, err=%v", err)
	}
	got := drainResponses(ch)
	if len(got) != 1 || got[0].Error == nil || !strings.HasPrefix(got[0].Error.Message, "ORIGINAL:") {
		t.Fatalf("exhausted stream must replay the ORIGINAL first error frame: %+v", got)
	}
	if i != 3 {
		t.Fatalf("must exhaust exactly 3 shrink steps, rebuilt %d", i)
	}
}

// 非 overflow（429 / 5xx / safety / billing / 一般 4xx）永不触发收缩——
// 与 ClassifyRetry 的 RetryWithBackoff / RetryFatal 路径互斥（不双重重试）。
func TestShrinkRetry_NonOverflowNeverTriggers(t *testing.T) {
	cases := map[string]error{
		"429 rate limited":               errors.New("Error 429: too many requests, rate limit exceeded"),
		"500 server error":               errors.New("Error 500: internal server error"),
		"502 bad gateway":                errors.New("Error 502: bad gateway"),
		"billing":                        errors.New("Error: Insufficient Balance"),
		"content filter":                 errors.New("blocked by content_filter policy"),
		"generic 400 (no overflow text)": errors.New("Error 400: invalid request: bad json schema"),
		"network EOF":                    errors.New("unexpected EOF"),
	}
	for name, cause := range cases {
		t.Run(name, func(t *testing.T) {
			inner := &shrinkStubModel{name: "base", err: cause}
			rebuilt := 0
			wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
				BaselineInputTokens: 1000,
				Rebuild:             func(int) (trpcmodel.Model, error) { rebuilt++; return inner, nil },
			})
			_, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
			if !errors.Is(err, cause) {
				t.Fatalf("non-overflow error must pass through untouched, got %v", err)
			}
			if rebuilt != 0 || inner.calls != 1 {
				t.Fatalf("non-overflow must never rebuild: rebuilt=%d calls=%d", rebuilt, inner.calls)
			}
		})
	}
}

// 流式非 overflow 错误（safety）原样透出，不重建。
func TestShrinkRetry_StreamedNonOverflowPassesThrough(t *testing.T) {
	frame := streamOverflowErrFrame("blocked by content_filter policy")
	inner := &shrinkStubModel{name: "base", frames: []*trpcmodel.Response{frame}}
	rebuilt := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { rebuilt++; return inner, nil },
	})
	ch, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("non-overflow stream must pass through, err=%v", err)
	}
	got := drainResponses(ch)
	if len(got) != 1 || got[0] != frame {
		t.Fatalf("frame must pass through identity-equal, got %+v", got)
	}
	if rebuilt != 0 {
		t.Fatalf("content_filter must never shrink-retry, rebuilt=%d", rebuilt)
	}
}

// ctx 已取消：不重建，原错直接上浮。
func TestShrinkRetry_CancelledCtxNeverRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inner := newOverflowStub("base")
	rebuilt := 0
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { rebuilt++; return inner, nil },
	})
	_, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if !errors.Is(err, shrinkOverflowErr) {
		t.Fatalf("cancelled ctx must surface the original error, got %v", err)
	}
	if rebuilt != 0 {
		t.Fatalf("cancelled ctx must never rebuild, rebuilt=%d", rebuilt)
	}
}

// Rebuild 失败：原错上浮（不能收缩 = 保持原失败形态）。
func TestShrinkRetry_RebuildFailureReturnsOriginal(t *testing.T) {
	inner := newOverflowStub("base")
	rebuildErr := errors.New("rebuild is not wired")
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { return nil, rebuildErr },
	})
	_, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if !errors.Is(err, shrinkOverflowErr) {
		t.Fatalf("rebuild failure must surface the ORIGINAL error, got %v", err)
	}
}

// 请求副本冻结：tailoring 同款就地改写落到副本上；原始 request.Messages
// 全程不变，且每次尝试都能看到完整原始历史（防双重裁剪，设计 §12.2 P0-3②）。
func TestShrinkRetry_RequestFrozenAcrossAttempts(t *testing.T) {
	inner := &shrinkStubModel{name: "base", err: shrinkOverflowErr, mutate: true}
	step1 := &shrinkStubModel{name: "s0.7", frames: []*trpcmodel.Response{{Done: true}}}
	msgs := []trpcmodel.Message{
		trpcmodel.NewUserMessage("u1"),
		trpcmodel.NewAssistantMessage("a1"),
		trpcmodel.NewUserMessage("u2"),
	}
	req := &trpcmodel.Request{Messages: msgs}
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { return step1, nil },
	})
	ch, err := wrapped.GenerateContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	drainResponses(ch)
	if len(req.Messages) != 3 {
		t.Fatalf("caller request must stay frozen, got %d messages", len(req.Messages))
	}
	if inner.msgLens[0] != 3 {
		t.Fatalf("first attempt must see full history, saw %d", inner.msgLens[0])
	}
	if step1.calls != 1 || step1.msgLens[0] != 3 {
		t.Fatalf("retry attempt must start from pristine history, saw %d", step1.msgLens)
	}
}

// 每步收缩一条 Warn（反作用#2：不刷 Info）；logger 构造注入。
func TestShrinkRetry_WarnOncePerStep(t *testing.T) {
	inner := newOverflowStub("base")
	lg := &captureWarnLogger{}
	wrapped := WrapModelWithShrinkRetry(inner, ShrinkRetryOptions{
		BaselineInputTokens: 1000,
		Rebuild:             func(int) (trpcmodel.Model, error) { return newOverflowStub("next"), nil },
		Logger:              lg,
	})
	_, _ = wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if len(lg.warns) != 3 {
		t.Fatalf("expect one Warn per shrink step (3), got %d", len(lg.warns))
	}
}

// marker 分类单测 pin（仿 billingMarkers/stallMarkers 先例）：overflow 族
// 恒真；429/5xx/billing/safety/空串恒假——与 retry_transport 触发集互斥。
func TestIsContextOverflow_MarkerPin(t *testing.T) {
	positives := []string{
		"This model's maximum context length is 8192 tokens",
		"context length exceeded: 126000 > 128000",
		"context_length_exceeded",
		"request too large: 5.2 MB",
		"ERROR 400: token limit exceeded",
		"prompt exceeds context window",
		// E2E-H3: Anthropic 400 规范文案——不得漏出 shrink-retry 触发集。
		`POST /v1/messages: 400 {"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 213452 tokens > 200000 maximum"}}`,
	}
	for _, msg := range positives {
		if !IsContextOverflow(msg) {
			t.Errorf("overflow marker missed: %q", msg)
		}
		if !IsContextOverflowError(errors.New(msg)) {
			t.Errorf("error-form overflow marker missed: %q", msg)
		}
	}
	negatives := []string{
		"Error 429: rate limit exceeded",
		"500 internal server error",
		"Error: Insufficient Balance",
		"blocked by content_filter",
		"invalid api key",
		"",
		"unexpected EOF",
	}
	for _, msg := range negatives {
		if IsContextOverflow(msg) {
			t.Errorf("non-overflow wrongly matched (double-retry hazard): %q", msg)
		}
	}
	if IsContextOverflowError(nil) {
		t.Error("nil error must not match")
	}
	// 协同钉：overflow 在 transport 分类仍是唯一 RetryFatal 缺口（装饰器独占
	// 接管），429/5xx 仍是 RetryWithBackoff（transport 独占）——两侧互不重叠。
	if d := ClassifyRetry(nil, shrinkOverflowErr); d.Type != RetryFatal {
		t.Errorf("transport must keep overflow as RetryFatal (shrink owns retry), got %v", d)
	}
}
