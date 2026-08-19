package evolution

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// stubModel replays a fixed response sequence.
type stubModel struct {
	responses []*trpcmodel.Response
	err       error
}

func (s *stubModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stub"} }

func (s *stubModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	ch := make(chan *trpcmodel.Response, len(s.responses))
	for _, r := range s.responses {
		ch <- r
	}
	close(ch)
	return ch, nil
}

func usageResp(prompt, completion, cached int) *trpcmodel.Response {
	return &trpcmodel.Response{
		Usage: &trpcmodel.Usage{
			PromptTokens:        prompt,
			CompletionTokens:    completion,
			TotalTokens:         prompt + completion,
			PromptTokensDetails: trpcmodel.PromptTokensDetails{CachedTokens: cached},
		},
	}
}

type captureRecorder struct {
	mu    sync.Mutex
	calls []biz.AuxLLMUsageInput
}

func (c *captureRecorder) record(_ context.Context, in biz.AuxLLMUsageInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, in)
	return nil
}

func (c *captureRecorder) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func (c *captureRecorder) last() biz.AuxLLMUsageInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[len(c.calls)-1]
}

func drain(t *testing.T, ch <-chan *trpcmodel.Response) int {
	t.Helper()
	n := 0
	for range ch {
		n++
	}
	return n
}

// P1-2: 计量 wrapper 必须透传全部响应、按轮求和计费 token、并落
// aux_evolution 用量（归因到构造时的 provider/model）。
func TestMeteringModel_RecordsAuxEvolution(t *testing.T) {
	inner := &stubModel{responses: []*trpcmodel.Response{
		usageResp(100, 10, 40),  // round 1 cumulative
		usageResp(100, 25, 40),  // round 1 final
		usageResp(180, 30, 60),  // round 2 (tool loop, prompt grew)
		{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: "ok"}}}},
	}}
	rec := &captureRecorder{}
	m := &meteringModel{inner: inner, record: rec.record, prov: "deepseek", mod: "deepseek-chat", lg: loggateway.NewNoop()}

	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if n := drain(t, ch); n != 4 {
		t.Fatalf("forwarded %d responses, want 4", n)
	}
	if rec.len() != 1 {
		t.Fatalf("record calls = %d, want 1", rec.len())
	}
	got := rec.last()
	if got.Kind != biz.UsageKindAuxEvolution {
		t.Errorf("Kind = %q, want %q", got.Kind, biz.UsageKindAuxEvolution)
	}
	if got.Provider != "deepseek" || got.Model != "deepseek-chat" {
		t.Errorf("attribution = %q/%q", got.Provider, got.Model)
	}
	if got.PromptTok != 280 || got.CompletionTok != 55 || got.CachedTok != 100 {
		t.Errorf("tokens = (%d,%d,%d), want (280,55,100)", got.PromptTok, got.CompletionTok, got.CachedTok)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.UsageSource != biz.UsageSourceResponse {
		t.Errorf("UsageSource = %q, want %q", got.UsageSource, biz.UsageSourceResponse)
	}
}

func TestMeteringModel_APIErrorMarksFailed(t *testing.T) {
	inner := &stubModel{responses: []*trpcmodel.Response{
		usageResp(50, 5, 0),
		{Error: &trpcmodel.ResponseError{Message: "rate limited"}},
	}}
	rec := &captureRecorder{}
	m := &meteringModel{inner: inner, record: rec.record, prov: "p", mod: "m", lg: loggateway.NewNoop()}
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	drain(t, ch)
	if rec.len() != 1 {
		t.Fatalf("record calls = %d, want 1", rec.len())
	}
	if rec.last().Status != "failed" || rec.last().ErrMsg != "rate limited" {
		t.Errorf("got status=%q err=%q", rec.last().Status, rec.last().ErrMsg)
	}
}

func TestMeteringModel_Skips(t *testing.T) {
	t.Run("zero usage", func(t *testing.T) {
		inner := &stubModel{responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: "ok"}}}},
		}}
		rec := &captureRecorder{}
		m := &meteringModel{inner: inner, record: rec.record, prov: "p", mod: "m", lg: loggateway.NewNoop()}
		ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		drain(t, ch)
		if rec.len() != 0 {
			t.Fatalf("record calls = %d, want 0", rec.len())
		}
	})

	t.Run("inner error propagates without record", func(t *testing.T) {
		inner := &stubModel{err: errors.New("dial fail")}
		rec := &captureRecorder{}
		m := &meteringModel{inner: inner, record: rec.record, prov: "p", mod: "m", lg: loggateway.NewNoop()}
		if _, err := m.GenerateContent(context.Background(), &trpcmodel.Request{}); err == nil {
			t.Fatal("expected error")
		}
		if rec.len() != 0 {
			t.Fatalf("record calls = %d, want 0", rec.len())
		}
	})
}

// 迟绑定：ref 未 Set（usecase 尚未就绪）时记录静默跳过、不 panic。
func TestMeteringModel_LateBoundRefNotReady(t *testing.T) {
	ref := biz.NewUsageUsecaseRef()
	inner := &stubModel{responses: []*trpcmodel.Response{usageResp(10, 5, 0)}}
	m := newMeteringModel(inner, ref, "p", "m", loggateway.NewNoop())
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if n := drain(t, ch); n != 1 {
		t.Fatalf("forwarded %d responses, want 1", n)
	}
}
