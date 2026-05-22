package testexec

import "testing"

func TestAssemblyForCatalogKey(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("read_file", nil)
	if !ok || len(cfg.EnabledTools) != 1 || cfg.EnabledTools[0] != "file" {
		t.Fatalf("read_file cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("shell_exec", map[string]any{"base_dir": "/tmp/ws"})
	if !ok || cfg.EnabledTools[0] != "hostexec" || cfg.ShellExecDir != "/tmp/ws" {
		t.Fatalf("shell_exec cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("diff_edit", map[string]any{"filesystem_dir": "/tmp/proj"})
	if !ok || cfg.EnabledTools[0] != "file" || cfg.FilesystemDir != "/tmp/proj" {
		t.Fatalf("diff_edit cfg=%+v ok=%v", cfg, ok)
	}
	cfg, ok = AssemblyForCatalogKey("patch_file", nil)
	if !ok || cfg.EnabledTools[0] != "file" {
		t.Fatalf("patch_file cfg=%+v ok=%v", cfg, ok)
	}
	_, ok = AssemblyForCatalogKey("mcp_tool_set", nil)
	if ok {
		t.Fatal("mcp should not be testable here")
	}
}
