package biz

import "testing"

func TestComputePolicyAllowedSet_googleSearchAlias(t *testing.T) {
	m := computePolicyAllowedSet("read_only", []string{"google_search"}, nil)
	if !m["google_search"] {
		t.Fatalf("expected google_search in allowed set; got keys %v", m)
	}
}

func TestComputePolicyAllowedSet_shellAlias(t *testing.T) {
	m := computePolicyAllowedSet("read_only", []string{"shell"}, nil)
	if !m["shell_exec"] {
		t.Fatalf("expected shell to imply shell_exec; got keys %v", m)
	}
}

func TestEffectiveToolState_optInWhenCatalogDisabled(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"}
	trow := Tool{Key: "shell_exec", Enabled: false}
	allowed := map[string]bool{"shell_exec": true}
	deny := map[string]bool{}
	state, _, en := computeEffectiveToolState(settings, trow, "full", allowed, deny)
	if !en || state != "allowed" {
		t.Fatalf("want shell_exec enabled under full+opt-in, got state=%q enabled=%v", state, en)
	}
}

func TestEffectiveToolState_catalogOffNotPolicyNamed(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"}
	trow := Tool{Key: "shell_exec", Enabled: false}
	allowed := profileAllowSet("read_only", nil) // no runtime
	deny := map[string]bool{}
	_, _, en := computeEffectiveToolState(settings, trow, "read_only", allowed, deny)
	if en {
		t.Fatal("shell_exec should not enable without policy naming it")
	}
}

func TestBuildAgentEffectiveTools_syntheticShellWhenMissingFromCatalog(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"}
	// Catalog without shell_exec row (simulates unmigrated DB) but full profile allows group:runtime.
	cat := []Tool{
		{Key: "read_file", DisplayName: "读取文件", Category: "filesystem", Source: "builtin", Enabled: true},
	}
	eff := buildAgentEffectiveTools(settings, cat)
	var shell *EffectiveAgentTool
	for i := range eff.Items {
		if eff.Items[i].ToolKey == "shell_exec" {
			shell = &eff.Items[i]
			break
		}
	}
	if shell == nil {
		t.Fatal("expected synthetic shell_exec in effective items when policy allows runtime but catalog omits row")
	}
	if !shell.Enabled || shell.EffectiveState != "allowed" {
		t.Fatalf("want shell_exec allowed under full profile, got %#v", shell)
	}
}

func TestBuildAgentEffectiveTools_noSyntheticShellWhenNotInPolicy(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}
	cat := []Tool{{Key: "read_file", DisplayName: "读取文件", Category: "filesystem", Source: "builtin", Enabled: true}}
	eff := buildAgentEffectiveTools(settings, cat)
	for _, it := range eff.Items {
		if it.ToolKey == "shell_exec" {
			t.Fatalf("did not expect shell_exec in items without runtime in policy, got %#v", it)
		}
	}
}
