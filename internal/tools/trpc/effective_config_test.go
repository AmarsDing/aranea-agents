package trpc

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestToolsetConfigFromEffectiveKeys_filesystemGroup(t *testing.T) {
	cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{"read_file": true})
	if !cfg.Filesystem || cfg.ShellExec {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestToolsetConfigFromEffectiveKeys_integrationKeys(t *testing.T) {
	cfg := ToolsetConfigFromEffectiveKeys(map[string]bool{
		biz.ToolKeyKnowledgeSearch: true,
		biz.ToolKeyCallAgent:       true,
	})
	if !cfg.KnowledgeSearch || !cfg.CallAgent {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}
