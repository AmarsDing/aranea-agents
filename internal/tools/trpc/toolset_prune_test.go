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

func TestPruneUnconfiguredToolFlags_browserMissingConfig(t *testing.T) {
	cfg := &ToolsetConfig{BrowserEnabled: true}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if cfg.BrowserEnabled {
		t.Error("BrowserEnabled should be pruned when Browser config is nil")
	}
	found := false
	for _, s := range skipped {
		if s == "browser" {
			found = true
		}
	}
	if !found {
		t.Errorf("browser should be in skipped list, got %v", skipped)
	}
}

func TestPruneUnconfiguredToolFlags_messageMissingRouter(t *testing.T) {
	cfg := &ToolsetConfig{Message: true}
	skipped := PruneUnconfiguredToolFlags(cfg)
	if cfg.Message {
		t.Error("Message should be pruned when OutboundRouter is nil")
	}
	found := false
	for _, s := range skipped {
		if s == "message" {
			found = true
		}
	}
	if !found {
		t.Errorf("message should be in skipped list, got %v", skipped)
	}
}
