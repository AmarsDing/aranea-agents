package llmcontext

import (
	"context"
	"testing"
)

func TestResolveWindow_ignoresProviderAndLocalCaps(t *testing.T) {
	// Provider catalog, session, and agent windows must not change the
	// product chat-context budget.
	cases := []ResolveInput{
		{AgentWindow: 32000},
		{ProviderModelConfigJSON: `{"context_window_k":200}`, AgentWindow: 32000},
		{ProviderModelConfigJSON: `{"context_window_k":64}`, AgentWindow: 128000},
		{ProviderModelConfigJSON: `{"context_window_k":200}`, SessionDefaultWindow: 128000, AgentWindow: 256000},
		{ProviderModelConfigJSON: `{"context_window_k":1000}`},
		{},
	}
	for i, in := range cases {
		if win := ResolveWindow(in); win != DefaultWindowTokens {
			t.Fatalf("case %d: got %d want %d", i, win, DefaultWindowTokens)
		}
	}
}

func TestWindowFromContext(t *testing.T) {
	if got := WindowFromContext(context.Background()); got != DefaultWindowTokens {
		t.Fatalf("empty ctx: got %d want %d", got, DefaultWindowTokens)
	}
	ctx := ContextWithWindow(context.Background(), 120)
	if got := WindowFromContext(ctx); got != 120 {
		t.Fatalf("override: got %d want 120", got)
	}
}

func TestClampWindow(t *testing.T) {
	if got := ClampWindow(0); got != DefaultWindowTokens {
		t.Fatalf("zero: got %d want %d", got, DefaultWindowTokens)
	}
	if got := ClampWindow(-1); got != DefaultWindowTokens {
		t.Fatalf("negative: got %d want %d", got, DefaultWindowTokens)
	}
	if got := ClampWindow(128000); got != 128000 {
		t.Fatalf("below cap: got %d want 128000", got)
	}
	if got := ClampWindow(1_000_000); got != MaxWindowTokens {
		t.Fatalf("above cap: got %d want %d", got, MaxWindowTokens)
	}
}
