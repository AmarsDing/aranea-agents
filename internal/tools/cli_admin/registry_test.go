package cli_admin

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestIsCLIAdminAllowed_SystemAdminKey(t *testing.T) {
	if !IsCLIAdminAllowed(biz.SystemAdminAgentKey) {
		t.Fatal("expected __system_admin__ to be allowed")
	}
}

func TestIsCLIAdminAllowed_RegularKey(t *testing.T) {
	if IsCLIAdminAllowed("my-agent") {
		t.Fatal("expected regular agent key to be denied")
	}
}

func TestIsCLIAdminAllowed_EmptyKey(t *testing.T) {
	if IsCLIAdminAllowed("") {
		t.Fatal("expected empty key to be denied")
	}
}

func TestDeps_String_TokenMasking(t *testing.T) {
	d := Deps{APIBaseURL: "http://localhost:8080", APIToken: "secret-token"}
	s := d.String()
	if !strings.Contains(s, "***") {
		t.Fatalf("expected token to be masked, got %q", s)
	}
	if strings.Contains(s, "secret-token") {
		t.Fatalf("expected token to be masked, but raw token visible in %q", s)
	}
	if !strings.Contains(s, "http://localhost:8080") {
		t.Fatalf("expected base URL to be visible, got %q", s)
	}
}

func TestDeps_String_EmptyToken(t *testing.T) {
	d := Deps{APIBaseURL: "http://localhost:8080", APIToken: ""}
	s := d.String()
	if strings.Contains(s, "***") {
		t.Fatalf("expected no masking for empty token, got %q", s)
	}
}

func TestRegisterAll_ReturnsExpectedToolNames(t *testing.T) {
	tools := RegisterAll(Deps{})
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(tools))
	}

	expected := []string{
		"cli_admin_skill_list",
		"cli_admin_skill_get",
		"cli_admin_skill_install_from_url",
		"cli_admin_agent_list",
		"cli_admin_agent_get",
		"cli_admin_pkg_install_from_url",
	}

	var names []string
	for _, tool := range tools {
		if decl := tool.Declaration(); decl != nil {
			names = append(names, decl.Name)
		}
	}

	for _, want := range expected {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected tool %q in registered tools, got %v", want, names)
		}
	}
}
