package tools

import "testing"

// TestClassifyTool_KnownConcurrentSafe verifies that tools marked with
// SupportsConcurrency=true in the registry are classified as ConcurrentSafe.
func TestClassifyTool_KnownConcurrentSafe(t *testing.T) {
	// Registry-level ConcurrentSafe names. The "file" ToolSet itself stays
	// ConcurrentSafe, but workspace child tools are not cacheable.
	cacheable := []string{"read_document", "read_spreadsheet", "httpfetch", "duckduckgo", "wikipedia"}
	for _, name := range cacheable {
		got := ClassifyTool(name)
		if got != SafetyConcurrentSafe {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyConcurrentSafe", name, got)
		}
		if !IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = false, want true", name)
		}
	}
	if ClassifyTool("file") != SafetyConcurrentSafe {
		t.Errorf("ClassifyTool(file) = %v, want SafetyConcurrentSafe", ClassifyTool("file"))
	}
	if IsCacheable("file") {
		t.Error("IsCacheable(file) must be false: workspace reads would go stale after writes")
	}
}

func TestClassifyTool_RuntimeNamesInheritRegistrySafety(t *testing.T) {
	concurrent := []string{"read_file", "list_file", "search_content", "read_multiple_files", "web_fetch", "duckduckgo_search"}
	for _, name := range concurrent {
		if got := ClassifyTool(name); got != SafetyConcurrentSafe {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyConcurrentSafe", name, got)
		}
	}
	exclusive := []string{"exec_command", "write_stdin", "kill_session", "shell_exec", "send_email"}
	for _, name := range exclusive {
		if got := ClassifyTool(name); got != SafetyExclusive {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyExclusive", name, got)
		}
		if IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = true, want false", name)
		}
	}
	writes := []string{"save_file", "diff_edit", "patch_file", "replace_content", "write_file"}
	for _, name := range writes {
		if got := ClassifyTool(name); got != SafetyExclusive {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyExclusive (file write)", name, got)
		}
		if IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = true, want false", name)
		}
	}
	for _, name := range []string{"read_file", "list_file", "search_content", "read_multiple_files"} {
		if IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = true, want false (workspace file tools are not cached)", name)
		}
	}
}

// TestClassifyTool_KnownExclusive verifies that tools without
// SupportsConcurrency are classified as Exclusive.
func TestClassifyTool_KnownExclusive(t *testing.T) {
	// "hostexec" and "email" do not set SupportsConcurrency.
	exclusive := []string{"hostexec", "email", "claudecode"}
	for _, name := range exclusive {
		got := ClassifyTool(name)
		if got != SafetyExclusive {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyExclusive", name, got)
		}
		if IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = true, want false", name)
		}
	}
}

// TestClassifyTool_UnknownDefaultsExclusive verifies that unknown tool
// names default to SafetyExclusive (the safe default).
func TestClassifyTool_UnknownDefaultsExclusive(t *testing.T) {
	unknown := []string{"nonexistent_tool", "fake_tool_12345", ""}
	for _, name := range unknown {
		got := ClassifyTool(name)
		if got != SafetyExclusive {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyExclusive (safe default)", name, got)
		}
	}
}

// TestClassifyTool_CaseInsensitive verifies that classification is
// case-insensitive.
func TestClassifyTool_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  ToolSafety
	}{
		{"FILE", SafetyConcurrentSafe},
		{"File", SafetyConcurrentSafe},
		{"HOSTEXEC", SafetyExclusive},
		{"HostExec", SafetyExclusive},
	}
	for _, tt := range tests {
		got := ClassifyTool(tt.input)
		if got != tt.want {
			t.Errorf("ClassifyTool(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
