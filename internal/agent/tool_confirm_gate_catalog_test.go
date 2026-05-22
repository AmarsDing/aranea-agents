package agent

import "testing"

func TestCatalogRequiresConfirm_execCommandAlias(t *testing.T) {
	t.Parallel()
	catalog := map[string]bool{"shell_exec": true}
	if !catalogRequiresConfirm(catalog, "exec_command") {
		t.Fatal("expected exec_command to require confirm when shell_exec does")
	}
	if catalogRequiresConfirm(catalog, "read_file") {
		t.Fatal("did not expect read_file")
	}
}

func TestToolConfirmGate_needsConfirm_execCommand(t *testing.T) {
	t.Parallel()
	g := &toolConfirmGate{catalog: map[string]bool{"shell_exec": true, "exec_command": true}}
	if !g.needsConfirm("exec_command", nil) {
		t.Fatal("expected needsConfirm for exec_command")
	}
}
