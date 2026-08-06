package agent

import "testing"

func TestCatalogRequiresConfirm_execCommandAlias(t *testing.T) {
	t.Parallel()
	catalog := map[string]confirmCatalogEntry{"shell_exec": {requiresConfirm: true}}
	if !catalogRequiresConfirm(catalog, "exec_command") {
		t.Fatal("expected exec_command to require confirm when shell_exec does (via reverse alias lookup)")
	}
	if catalogRequiresConfirm(catalog, "read_file") {
		t.Fatal("did not expect read_file")
	}
}

func TestCatalogRequiresConfirm_fileToolPrefix(t *testing.T) {
	t.Parallel()
	catalog := map[string]confirmCatalogEntry{
		"save_file":       {requiresConfirm: true},
		"replace_content": {requiresConfirm: true},
	}
	// Runtime name "file_save_file" should match catalog key "save_file" via ToolSet prefix.
	if !catalogRequiresConfirm(catalog, "file_save_file") {
		t.Fatal("expected file_save_file to require confirm when save_file does (via ToolSet prefix)")
	}
	if !catalogRequiresConfirm(catalog, "file_replace_content") {
		t.Fatal("expected file_replace_content to require confirm when replace_content does (via ToolSet prefix)")
	}
	// Exact match still works.
	if !catalogRequiresConfirm(catalog, "save_file") {
		t.Fatal("expected save_file exact match to require confirm")
	}
	// Unrelated runtime name should not match.
	if catalogRequiresConfirm(catalog, "file_read_file") {
		t.Fatal("did not expect file_read_file (read_file not in catalog)")
	}
}

func TestCatalogRequiresConfirm_nilCatalog(t *testing.T) {
	t.Parallel()
	if catalogRequiresConfirm(nil, "shell_exec") {
		t.Fatal("did not expect nil catalog to require confirm")
	}
}

// The hostexec ToolSet mounts its tools under the "hostexec_" prefix while the
// catalog gates the toolset under key "shell_exec" (alias of the mounted
// "exec_command"). Mounted names like "hostexec_exec_command" must inherit the
// catalog policy — previously they bypassed the gate entirely, letting the
// agent run arbitrary host commands after a shell_exec confirmation timed out.
func TestCatalogRequiresConfirm_hostexecMountedExecCommand(t *testing.T) {
	t.Parallel()
	catalog := map[string]confirmCatalogEntry{"shell_exec": {requiresConfirm: true}}
	if !catalogRequiresConfirm(catalog, "hostexec_exec_command") {
		t.Fatal("expected hostexec_exec_command to require confirm when shell_exec does (toolset prefix + reverse alias on suffix)")
	}
	// When the catalog does not gate shell_exec (e.g. an admin override disabled
	// confirmation), the mounted name must likewise stay ungated.
	if catalogRequiresConfirm(map[string]confirmCatalogEntry{}, "hostexec_exec_command") {
		t.Fatal("did not expect hostexec_exec_command gating without a catalog entry")
	}
}

// MCP-mounted toolsets expose sub-tools whose runtime names derive from the
// catalog key: catalog "browser" → "browser_navigate", optionally with an MCP
// ToolPrefix → "playwright_browser_navigate". The gate must treat the catalog
// key appearing as a whole underscore-delimited segment as a match, otherwise
// every MCP sub-tool bypasses the confirmation gate.
func TestCatalogRequiresConfirm_mcpDerivedSubToolNames(t *testing.T) {
	t.Parallel()
	catalog := map[string]confirmCatalogEntry{
		"browser": {requiresConfirm: true},
	}
	cases := []struct {
		name     string
		toolName string
		want     bool
	}{
		{"exact", "browser", true},
		{"sub-tool", "browser_navigate", true},
		{"sub-tool screenshot", "browser_take_screenshot", true},
		{"mcp prefix + sub-tool", "playwright_browser_navigate", true},
		{"other mcp prefix", "bw_browser_snapshot", true},
		{"mcp prefix only", "playwright_browser", true},
		{"segment not delimited left", "webbrowser_open", false},
		{"unrelated", "playwright_click", false},
		{"unrelated with underscore", "web_fetch", false},
	}
	for _, tc := range cases {
		if got := catalogRequiresConfirm(catalog, tc.toolName); got != tc.want {
			t.Fatalf("%s: catalogRequiresConfirm(%q) = %v, want %v", tc.name, tc.toolName, got, tc.want)
		}
	}
}

func TestToolConfirmGate_needsConfirm_execCommand(t *testing.T) {
	t.Parallel()
	g := &toolConfirmGate{catalog: map[string]confirmCatalogEntry{"shell_exec": {requiresConfirm: true}}}
	if !g.needsConfirm("exec_command", nil) {
		t.Fatal("expected needsConfirm for exec_command")
	}
}

func TestToolConfirmGate_needsConfirm_fileSaveFile(t *testing.T) {
	t.Parallel()
	g := &toolConfirmGate{catalog: map[string]confirmCatalogEntry{
		"save_file": {requiresConfirm: true},
	}}
	if !g.needsConfirm("file_save_file", nil) {
		t.Fatal("expected needsConfirm for file_save_file")
	}
	if !g.needsConfirm("save_file", nil) {
		t.Fatal("expected needsConfirm for save_file (exact match)")
	}
}
