package llmcontext

import "testing"

func TestResolveWindow_agentFallback(t *testing.T) {
	win := ResolveWindow(ResolveInput{AgentWindow: 32000})
	if win != 32000 {
		t.Fatalf("got %d want 32000", win)
	}
}

func TestResolveWindow_providerConfig(t *testing.T) {
	win := ResolveWindow(ResolveInput{
		ProviderModelConfigJSON: `{"context_window_k":200}`,
		AgentWindow:             32000,
	})
	if win != 200_000 {
		t.Fatalf("got %d want 200000", win)
	}
}

func TestResolveWindow_globalDefault(t *testing.T) {
	win := ResolveWindow(ResolveInput{})
	if win != DefaultWindowTokens {
		t.Fatalf("got %d want %d", win, DefaultWindowTokens)
	}
}
