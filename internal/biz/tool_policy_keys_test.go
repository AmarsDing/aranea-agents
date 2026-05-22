package biz

import "testing"

func TestNormalizeToolPolicyKey_editFileAlias(t *testing.T) {
	t.Parallel()
	if got := normalizeToolPolicyKey("edit_file"); got != "diff_edit" {
		t.Fatalf("edit_file: got %q want diff_edit", got)
	}
	if got := normalizeToolPolicyKey("shell"); got != "shell_exec" {
		t.Fatalf("shell: got %q want shell_exec", got)
	}
}

func TestPropagateAllowAliases_editFile(t *testing.T) {
	t.Parallel()
	m := map[string]bool{"edit_file": true}
	propagateAllowAliases(m)
	if !m["diff_edit"] {
		t.Fatal("expected diff_edit after allow edit_file")
	}
}
