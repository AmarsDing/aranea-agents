package trpc

import "testing"

func TestPruneUnconfiguredToolFlags(t *testing.T) {
	cfg := &ToolsetConfig{
		GeminiFetch:  true,
		GoogleSearch: true,
		GoogleAPIKey: "k",
	}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if len(skipped) != 2 {
		t.Fatalf("skipped=%v", skipped)
	}
	if cfg.GeminiFetch || cfg.GoogleSearch {
		t.Fatalf("flags should be cleared: %+v", cfg)
	}
}

func TestPruneUnconfiguredToolFlags_googleConfigured(t *testing.T) {
	cfg := &ToolsetConfig{
		GoogleSearch: true,
		GoogleAPIKey: "k",
		GoogleCX:     "cx",
		GeminiFetch:  true,
		GeminiModel:  "gemini-2.5-flash",
	}
	if skipped := PruneUnconfiguredToolFlags(cfg); len(skipped) != 0 {
		t.Fatalf("unexpected skip: %v", skipped)
	}
	if !cfg.GoogleSearch || !cfg.GeminiFetch {
		t.Fatal("configured tools should stay enabled")
	}
}
