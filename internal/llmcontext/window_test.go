package llmcontext

import "testing"

func TestResolveWindow_agentFallback(t *testing.T) {
	win := ResolveWindow(ResolveInput{AgentWindow: 32000})
	if win != 32000 {
		t.Fatalf("got %d want 32000", win)
	}
}

func TestResolveWindow_minCap(t *testing.T) {
	// Local cap (agent) is smaller than catalog → local cap wins.
	win := ResolveWindow(ResolveInput{
		ProviderModelConfigJSON: `{"context_window_k":200}`,
		AgentWindow:             32000,
	})
	if win != 32000 {
		t.Fatalf("got %d want 32000", win)
	}
	// Catalog is smaller than local cap → catalog wins.
	win = ResolveWindow(ResolveInput{
		ProviderModelConfigJSON: `{"context_window_k":64}`,
		AgentWindow:             128000,
	})
	if win != 64000 {
		t.Fatalf("got %d want 64000", win)
	}
	// Session default sits between catalog and agent.
	win = ResolveWindow(ResolveInput{
		ProviderModelConfigJSON: `{"context_window_k":200}`, // 200K
		SessionDefaultWindow:    128000,
		AgentWindow:             256000,
	})
	if win != 128000 {
		t.Fatalf("got %d want 128000", win)
	}
}

func TestResolveWindow_globalDefault(t *testing.T) {
	win := ResolveWindow(ResolveInput{})
	if win != DefaultWindowTokens {
		t.Fatalf("got %d want %d", win, DefaultWindowTokens)
	}
}
