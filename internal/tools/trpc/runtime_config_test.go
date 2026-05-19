package trpc

import "testing"

func TestApplyRuntimeConfigMaps_filesystemAndGoogle(t *testing.T) {
	cfg := &ToolsetConfig{}
	ApplyRuntimeConfigMaps(cfg, map[string]map[string]any{
		"read_file":        {"filesystem_dir": "/data/agent"},
		"google_search":    {"api_key": "k", "cx": "cx1"},
		"gemini_web_fetch": {"model": "gemini-2.0"},
	})
	if cfg.FilesystemDir != "/data/agent" {
		t.Fatalf("FilesystemDir=%q", cfg.FilesystemDir)
	}
	if cfg.GoogleAPIKey != "k" || cfg.GoogleCX != "cx1" {
		t.Fatalf("google: key=%q cx=%q", cfg.GoogleAPIKey, cfg.GoogleCX)
	}
	if cfg.GeminiModel != "gemini-2.0" {
		t.Fatalf("GeminiModel=%q", cfg.GeminiModel)
	}
}
