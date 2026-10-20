package provider

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestNormalizeUsageTokens_Nil(t *testing.T) {
	in, out, cached, write, reasoning := NormalizeUsageTokens(nil)
	if in != 0 || out != 0 || cached != 0 || write != 0 || reasoning != 0 {
		t.Fatalf("nil usage must normalize to all zeros, got in=%d out=%d cached=%d write=%d reasoning=%d",
			in, out, cached, write, reasoning)
	}
}

// Anthropic/Bedrock explicit-cache semantics: input_tokens excludes
// cache_read/cache_creation; the billing input total must add both back.
func TestNormalizeUsageTokens_AnthropicExplicitCache(t *testing.T) {
	u := &trpcmodel.Usage{
		PromptTokens:     100, // fresh only
		CompletionTokens: 40,
		TotalTokens:      140,
		PromptTokensDetails: trpcmodel.PromptTokensDetails{
			CachedTokens:        80, // framework maps CachedTokens=CacheReadInputTokens
			CacheReadTokens:     80,
			CacheCreationTokens: 20,
		},
		CompletionTokensDetails: trpcmodel.CompletionTokensDetails{ReasoningTokens: 15},
	}
	in, out, cached, write, reasoning := NormalizeUsageTokens(u)
	if in != 200 {
		t.Fatalf("billingInput = %d, want 100+80+20=200", in)
	}
	if out != 40 {
		t.Fatalf("output = %d, want 40", out)
	}
	if cached != 80 || write != 20 {
		t.Fatalf("cached=%d write=%d, want 80/20", cached, write)
	}
	if reasoning != 15 {
		t.Fatalf("reasoning = %d, want 15", reasoning)
	}
	// Subset invariants the whole accounting chain relies on.
	if cached > in || write > in || cached+write > in {
		t.Fatalf("cached/write must be subsets of billingInput: in=%d cached=%d write=%d", in, cached, write)
	}
	if reasoning > out {
		t.Fatalf("reasoning must be a subset of output: out=%d reasoning=%d", out, reasoning)
	}
}

// OpenAI/DeepSeek semantics: PromptTokens already INCLUDES the cached part;
// read/write are zero → normalization must be a no-op on the input total.
func TestNormalizeUsageTokens_OpenAICompatibleNoOp(t *testing.T) {
	u := &trpcmodel.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: trpcmodel.PromptTokensDetails{
			CachedTokens: 40,
		},
	}
	in, out, cached, write, reasoning := NormalizeUsageTokens(u)
	if in != 100 || out != 50 || cached != 40 || write != 0 || reasoning != 0 {
		t.Fatalf("OpenAI-shape usage must pass through unchanged: in=%d out=%d cached=%d write=%d reasoning=%d",
			in, out, cached, write, reasoning)
	}
}

// --- IsGenuineOpenAIProvider (P1 cache-key gate, 2026-10-20) ---

type stubTeamModelCatalog struct {
	pm  biz.ProviderModel
	err error
}

func (s stubTeamModelCatalog) GetByProviderAndModel(context.Context, string, string) (biz.ProviderModel, error) {
	return s.pm, s.err
}
func (s stubTeamModelCatalog) List(context.Context) ([]biz.ProviderModel, error) { return nil, nil }

func TestIsGenuineOpenAIProvider(t *testing.T) {
	ctx := context.Background()
	row := func(configJSON string) stubTeamModelCatalog {
		return stubTeamModelCatalog{pm: biz.ProviderModel{Model: "m", ConfigJSON: configJSON}}
	}

	if !IsGenuineOpenAIProvider(ctx, row(`{"provider_type":"openai"}`), "openai", "gpt-5") {
		t.Fatal("provider_type=openai must be genuine")
	}
	if IsGenuineOpenAIProvider(ctx, row(`{"provider_type":"deepseek"}`), "deepseek", "deepseek-chat") {
		t.Fatal("deepseek variant must NOT be genuine openai")
	}
	if IsGenuineOpenAIProvider(ctx, row(`{"provider_type":"anthropic"}`), "anthropic", "claude") {
		t.Fatal("anthropic must NOT be genuine openai")
	}
	// Resolution failures degrade to false (no prompt_cache_key injected).
	if IsGenuineOpenAIProvider(ctx, row(`{bad json`), "openai", "m") {
		t.Fatal("config parse failure must degrade to false")
	}
	if IsGenuineOpenAIProvider(ctx, stubTeamModelCatalog{err: errors.New("db down")}, "openai", "m") {
		t.Fatal("catalog error must degrade to false")
	}
}
