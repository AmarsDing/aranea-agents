package agent

import "testing"

func TestCatalogRequiresConfirm_execCommandAlias(t *testing.T) {
	t.Parallel()
	catalog := map[string]confirmCatalogEntry{"shell_exec": {requiresConfirm: true, registryName: "hostexec"}}
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
		"save_file":       {requiresConfirm: true, registryName: "file"},
		"replace_content": {requiresConfirm: true, registryName: "file"},
	}
	// Runtime name "file_save_file" should match catalog key "save_file" via registry prefix.
	if !catalogRequiresConfirm(catalog, "file_save_file") {
		t.Fatal("expected file_save_file to require confirm when save_file does (via registry prefix)")
	}
	if !catalogRequiresConfirm(catalog, "file_replace_content") {
		t.Fatal("expected file_replace_content to require confirm when replace_content does (via registry prefix)")
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

func TestToolConfirmGate_needsConfirm_execCommand(t *testing.T) {
	t.Parallel()
	g := &toolConfirmGate{catalog: map[string]confirmCatalogEntry{"shell_exec": {requiresConfirm: true, registryName: "hostexec"}}}
	if !g.needsConfirm("exec_command", nil) {
		t.Fatal("expected needsConfirm for exec_command")
	}
}

func TestToolConfirmGate_needsConfirm_fileSaveFile(t *testing.T) {
	t.Parallel()
	g := &toolConfirmGate{catalog: map[string]confirmCatalogEntry{
		"save_file": {requiresConfirm: true, registryName: "file"},
	}}
	if !g.needsConfirm("file_save_file", nil) {
		t.Fatal("expected needsConfirm for file_save_file")
	}
	if !g.needsConfirm("save_file", nil) {
		t.Fatal("expected needsConfirm for save_file (exact match)")
	}
}
