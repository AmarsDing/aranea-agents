package llmcontext

import (
	"strings"
	"testing"
)

func TestRoughTokenEstimate_Empty(t *testing.T) {
	if got := RoughTokenEstimate(""); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestRoughTokenEstimate_Short(t *testing.T) {
	if got := RoughTokenEstimate("hi"); got != 1 {
		t.Fatalf("got %d want 1", got)
	}
}

func TestRoughTokenEstimate_Exact(t *testing.T) {
	if got := RoughTokenEstimate("abcdefgh"); got != 2 {
		t.Fatalf("got %d want 2", got)
	}
}

func TestRoughTokenEstimate_Large(t *testing.T) {
	s := strings.Repeat("a", 400)
	if got := RoughTokenEstimate(s); got != 100 {
		t.Fatalf("got %d want 100", got)
	}
}

func TestRoughTokenEstimate_Whitespace(t *testing.T) {
	if got := RoughTokenEstimate("   "); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestRoughTokenEstimate_Unicode(t *testing.T) {
	if got := RoughTokenEstimate("你好世界"); got != 1 {
		t.Fatalf("got %d want 1", got)
	}
}

func TestContextWindowFromConfigJSON_Empty(t *testing.T) {
	if got := contextWindowFromConfigJSON(""); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestContextWindowFromConfigJSON_EmptyObject(t *testing.T) {
	if got := contextWindowFromConfigJSON("{}"); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestContextWindowFromConfigJSON_Valid(t *testing.T) {
	if got := contextWindowFromConfigJSON(`{"context_window_k":128}`); got != 128000 {
		t.Fatalf("got %d want 128000", got)
	}
}

func TestContextWindowFromConfigJSON_Zero(t *testing.T) {
	if got := contextWindowFromConfigJSON(`{"context_window_k":0}`); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestContextWindowFromConfigJSON_Invalid(t *testing.T) {
	if got := contextWindowFromConfigJSON("not json"); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

func TestResolveWindow_SessionDefaultIgnored(t *testing.T) {
	win := ResolveWindow(ResolveInput{
		SessionDefaultWindow: 64000,
		AgentWindow:          32000,
	})
	if win != DefaultWindowTokens {
		t.Fatalf("got %d want %d", win, DefaultWindowTokens)
	}
	win = ResolveWindow(ResolveInput{SessionDefaultWindow: 64000})
	if win != DefaultWindowTokens {
		t.Fatalf("got %d want %d", win, DefaultWindowTokens)
	}
}

func TestContextRatio_ZeroWindow(t *testing.T) {
	if got := ContextRatio(100, 0); got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}

func TestContextRatio_NegativePrompt(t *testing.T) {
	if got := ContextRatio(-1, 1000); got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}

func TestContextStatusForRatio_BoundaryNormal(t *testing.T) {
	if got := ContextStatusForRatio(0.59); got != "normal" {
		t.Fatalf("got %q want %q", got, "normal")
	}
}

func TestContextStatusForRatio_BoundaryWarning(t *testing.T) {
	if got := ContextStatusForRatio(0.60); got != "warning" {
		t.Fatalf("got %q want %q", got, "warning")
	}
}

func TestContextStatusForRatio_BoundaryCritical(t *testing.T) {
	if got := ContextStatusForRatio(0.80); got != "critical" {
		t.Fatalf("got %q want %q", got, "critical")
	}
}

func TestContextStatusForRatio_BoundaryExceeded(t *testing.T) {
	if got := ContextStatusForRatio(0.95); got != "exceeded" {
		t.Fatalf("got %q want %q", got, "exceeded")
	}
}

func TestDefaultWindowTokens(t *testing.T) {
	if DefaultWindowTokens != 256000 {
		t.Fatalf("got %d want 256000", DefaultWindowTokens)
	}
	if MaxWindowTokens != 256000 {
		t.Fatalf("max: got %d want 256000", MaxWindowTokens)
	}
}
