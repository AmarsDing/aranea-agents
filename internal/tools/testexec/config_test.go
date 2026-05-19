package testexec

import "testing"

func TestAssemblyForCatalogKey(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("read_file", nil)
	if !ok || len(cfg.EnabledTools) != 1 || cfg.EnabledTools[0] != "file" {
		t.Fatalf("read_file cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("shell_exec", nil)
	if !ok || cfg.EnabledTools[0] != "hostexec" {
		t.Fatalf("shell_exec cfg=%+v ok=%v", cfg, ok)
	}
	_, ok = AssemblyForCatalogKey("mcp_tool_set", nil)
	if ok {
		t.Fatal("mcp should not be testable here")
	}
}
