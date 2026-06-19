package tools

import "testing"

// TestClassifyTool_KnownConcurrentSafe verifies that tools marked with
// SupportsConcurrency=true in the registry are classified as ConcurrentSafe.
func TestClassifyTool_KnownConcurrentSafe(t *testing.T) {
	// "file" and "read_document" are registered with SupportsConcurrency=true.
	cacheable := []string{"file", "read_document", "read_spreadsheet"}
	for _, name := range cacheable {
		got := ClassifyTool(name)
		if got != SafetyConcurrentSafe {
			t.Errorf("ClassifyTool(%q) = %v, want SafetyConcurrentSafe", name, got)
		}
		if !IsCacheable(name) {
			t.Errorf("IsCacheable(%q) = false, want true", name)
		}
	}
}

// TestClassifyTool_KnownExclusive verifies that tools without
// SupportsConcurrency are classified as Exclusive.
func TestClassifyTool_KnownExclusive(t *testing.T) {
	// "hostexec" and "email" do not set SupportsConcurrency.
	exclusive := []string{"hostexec", "email", "claudecode", "httpfetch"}
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
