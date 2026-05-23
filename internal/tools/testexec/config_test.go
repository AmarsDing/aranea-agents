package testexec

import (
	"testing"

	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

func TestAssemblyForCatalogKey(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("read_file", nil, nil)
	if !ok || len(cfg.EnabledTools) != 1 || cfg.EnabledTools[0] != "file" {
		t.Fatalf("read_file cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("shell_exec", map[string]any{"base_dir": "/tmp/ws"}, nil)
	if !ok || cfg.EnabledTools[0] != "hostexec" || cfg.ShellExecDir != "/tmp/ws" {
		t.Fatalf("shell_exec cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("diff_edit", map[string]any{"filesystem_dir": "/tmp/proj"}, nil)
	if !ok || cfg.EnabledTools[0] != "file" || cfg.FilesystemDir != "/tmp/proj" {
		t.Fatalf("diff_edit cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("patch_file", nil, nil)
	if !ok || cfg.EnabledTools[0] != "file" {
		t.Fatalf("patch_file cfg=%+v ok=%v", cfg, ok)
	}
	_, ok = AssemblyForCatalogKey("mcp_tool_set", nil, nil)
	if ok {
		t.Fatal("mcp should not be testable here")
	}
}

func TestAssemblyForCatalogKey_webResearchPlatform(t *testing.T) {
	_, ok := AssemblyForCatalogKey("web_research", map[string]any{"provider": "tavily"}, nil)
	if ok {
		t.Fatal("expected not ready without platform or agent key")
	}
	platform := &webresearchpkg.PlatformFields{HasAPIKey: true, APIKey: "k", Provider: "tavily"}
	cfg, ok := AssemblyForCatalogKey("web_research", map[string]any{"provider": "tavily"}, platform)
	if !ok || len(cfg.CustomTools) != 1 {
		t.Fatalf("cfg=%+v ok=%v", cfg, ok)
	}
}
