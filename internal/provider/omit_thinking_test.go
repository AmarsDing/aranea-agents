package provider

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestOmitThinkingKey(t *testing.T) {
	ollamaCfg := ProviderModelConfig{ProviderType: "ollama"}
	if !omitThinkingKey(biz.ProviderModel{}, ollamaCfg) {
		t.Fatal("non-explicit ollama must omit thinking key")
	}
	explicitOff := biz.ProviderModel{
		CapabilitiesExplicit: true,
		Capabilities:         biz.ModelCapabilities{Vision: true, Thinking: false},
	}
	if !omitThinkingKey(explicitOff, ollamaCfg) {
		t.Fatal("explicit capability_thinking=false must omit thinking key")
	}
	explicitOn := biz.ProviderModel{
		CapabilitiesExplicit: true,
		Capabilities:         biz.ModelCapabilities{Thinking: true},
	}
	if omitThinkingKey(explicitOn, ollamaCfg) {
		t.Fatal("explicit capability_thinking=true must keep thinking key")
	}
	deepseek := biz.ProviderModel{}
	if omitThinkingKey(deepseek, ProviderModelConfig{ProviderType: "deepseek"}) {
		t.Fatal("deepseek without explicit caps must keep thinking key")
	}
}

func TestWrapOmitThinking_StripsThinkingEnabled(t *testing.T) {
	inner := &captureThinkingModel{}
	wrapped := wrapOmitThinking(inner)
	disabled := false
	req := &trpcmodel.Request{
		GenerationConfig: trpcmodel.GenerationConfig{ThinkingEnabled: &disabled, Stream: true},
	}
	ch, err := wrapped.GenerateContent(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	for range ch {
	}
	if inner.got == nil || inner.got.GenerationConfig.ThinkingEnabled != nil {
		t.Fatalf("thinking key must be stripped before inner model, got %+v", inner.got)
	}
}

func TestModelSupportsThinking_OllamaHeuristic(t *testing.T) {
	lg := loggateway.NewNoop()
	if ModelSupportsThinking(context.Background(), nil, "ollama", "qwen2.5vl:7b", lg) {
		t.Fatal("ollama without catalog must not claim thinking support")
	}
	if !ModelSupportsThinking(context.Background(), nil, "deepseek", "deepseek-chat", lg) {
		t.Fatal("deepseek without catalog must keep thinking injection")
	}
}

type captureThinkingModel struct {
	got *trpcmodel.Request
}

func (m *captureThinkingModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "capture"} }

func (m *captureThinkingModel) GenerateContent(_ context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	cp := *req
	m.got = &cp
	ch := make(chan *trpcmodel.Response)
	close(ch)
	return ch, nil
}
