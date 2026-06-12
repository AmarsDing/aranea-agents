package testexec

import (
	"testing"

	"aranea-agents/pkg/loggateway"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

func TestAssemblyForCatalogKey(t *testing.T) {
	noop := loggateway.NewNoop()
	cfg, ok, _ := AssemblyForCatalogKey("read_file", nil, nil, noop)
	if !ok || len(cfg.EnabledTools) != 1 || cfg.EnabledTools[0] != "file" {
		t.Fatalf("read_file cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok, _ = AssemblyForCatalogKey("shell_exec", map[string]any{"base_dir": "/tmp/ws"}, nil, noop)
	if !ok || cfg.EnabledTools[0] != "hostexec" || cfg.ShellExec.Dir != "/tmp/ws" {
		t.Fatalf("shell_exec cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok, _ = AssemblyForCatalogKey("diff_edit", map[string]any{"filesystem_dir": "/tmp/proj"}, nil, noop)
	if !ok || cfg.EnabledTools[0] != "file" || cfg.FilesystemDir != "/tmp/proj" {
		t.Fatalf("diff_edit cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok, _ = AssemblyForCatalogKey("patch_file", nil, nil, noop)
	if !ok || cfg.EnabledTools[0] != "file" {
		t.Fatalf("patch_file cfg=%+v ok=%v", cfg, ok)
	}
	_, ok, _ = AssemblyForCatalogKey("mcp_tool_set", nil, nil, noop)
	if ok {
		t.Fatal("mcp should not be testable here")
	}
}

func TestAssemblyForCatalogKey_webResearchPlatform(t *testing.T) {
	noop := loggateway.NewNoop()
	_, ok, _ := AssemblyForCatalogKey("web_research", map[string]any{"provider": "tavily"}, nil, noop)
	if ok {
		t.Fatal("expected not ready without platform or agent key")
	}
	platform := &webresearchpkg.PlatformFields{HasAPIKey: true, APIKey: "k", Provider: "tavily"}
	cfg, ok, _ := AssemblyForCatalogKey("web_research", map[string]any{"provider": "tavily"}, platform, noop)
	if !ok || len(cfg.Session.CustomTools) != 1 {
		t.Fatalf("cfg=%+v ok=%v", cfg, ok)
	}
}
