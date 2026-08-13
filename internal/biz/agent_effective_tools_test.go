package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

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
	eff := buildAgentEffectiveTools(settings, cat, loggateway.NewNoop())
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

func TestBuildAgentEffectiveTools_catalogDisableBlocksDefaultEnabledTool(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research"}
	cat := []Tool{
		{
			Key:         "gemini_web_fetch",
			DisplayName: "Gemini 抓取",
			Category:    "web",
			Source:      "builtin",
			Enabled:     false,
		},
	}
	eff := buildAgentEffectiveTools(settings, cat, loggateway.NewNoop())
	for _, it := range eff.Items {
		if it.ToolKey != "gemini_web_fetch" {
			continue
		}
		if it.Enabled {
			t.Fatalf("catalog-disabled gemini_web_fetch must not be effective under research profile, got %#v", it)
		}
		if it.Reason != "agent_deny" {
			t.Fatalf("reason=%q want agent_deny", it.Reason)
		}
		return
	}
	t.Fatal("gemini_web_fetch missing from effective items")
}

func TestEffectiveToolState_fullProfileDenyOverridesAllow(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"}
	// read_file is open-by-default and in the full profile allow set.
	trow := Tool{Key: "read_file", Enabled: true}
	allowed := profileAllowSet("full", nil)
	deny := map[string]bool{"read_file": true}

	state, reason, en := computeEffectiveToolState(settings, trow, "full", allowed, deny)
	if en {
		t.Fatalf("read_file must be denied when in deny set, even under full profile; got enabled=%v", en)
	}
	if state != "denied" {
		t.Fatalf("want state=denied, got %q", state)
	}
	if reason != "agent_deny" {
		t.Fatalf("want reason=agent_deny, got %q", reason)
	}
}

func TestEffectiveToolState_fullProfileDenyOverridesOptIn(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "full"}
	// shell_exec is opt-in only (Enabled=false) but in the full profile allow set.
	trow := Tool{Key: "shell_exec", Enabled: false}
	allowed := profileAllowSet("full", nil)
	deny := map[string]bool{"shell_exec": true}

	state, reason, en := computeEffectiveToolState(settings, trow, "full", allowed, deny)
	if en {
		t.Fatalf("shell_exec must be denied when in deny set, even under full profile; got enabled=%v", en)
	}
	if state != "denied" {
		t.Fatalf("want state=denied, got %q", state)
	}
	if reason != "agent_deny" {
		t.Fatalf("want reason=agent_deny, got %q", reason)
	}
}

func TestBuildAgentEffectiveTools_noSyntheticShellWhenNotInPolicy(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}
	cat := []Tool{{Key: "read_file", DisplayName: "读取文件", Category: "filesystem", Source: "builtin", Enabled: true}}
	eff := buildAgentEffectiveTools(settings, cat, loggateway.NewNoop())
	for _, it := range eff.Items {
		if it.ToolKey == "shell_exec" {
			t.Fatalf("did not expect shell_exec in items without runtime in policy, got %#v", it)
		}
	}
}

// computer_use_* 工具组：spirit profile 显式 opt-in（seed 默认 enabled=false，
// 走 registryOptInOnlyKeys 白名单而非全局开启）。
func TestProfileAllowSet_spiritIncludesComputerUseGroup(t *testing.T) {
	allowed := profileAllowSet("spirit", nil)
	for _, key := range []string{
		"computer_use_observe", "computer_use_screenshot",
		"computer_use_act", "computer_use_launch", "computer_use_session",
	} {
		if !allowed[key] {
			t.Fatalf("expected %s in spirit allowed set; got keys %v", key, allowed)
		}
	}
}

func TestProfileAllowSet_nonSpiritProfilesExcludeComputerUse(t *testing.T) {
	for _, profile := range []string{"read_only", "coding", "research", "full"} {
		allowed := profileAllowSet(profile, nil)
		if allowed["computer_use_act"] {
			t.Fatalf("profile %q must not include computer_use_act", profile)
		}
	}
}

func TestApplyRegistryAdminDenials_computerUseOptInNotDenied(t *testing.T) {
	catalog := []Tool{
		{Key: "computer_use_act", Enabled: false},
		{Key: "computer_use_observe", Enabled: false},
		{Key: "computer_use_screenshot", Enabled: false},
		{Key: "computer_use_launch", Enabled: false},
		{Key: "computer_use_session", Enabled: false},
	}
	deny := map[string]bool{}
	applyRegistryAdminDenials(catalog, deny)
	for _, c := range catalog {
		if deny[c.Key] {
			t.Fatalf("opt-in key %s must not be admin-denied; deny=%v", c.Key, deny)
		}
	}
}

func TestBuildAgentEffectiveTools_spiritComputerUseEnabled(t *testing.T) {
	settings := AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "spirit"}
	cat := []Tool{
		{Key: "computer_use_act", DisplayName: "桌面动作", Category: "computeruse", Source: "builtin", Enabled: false},
	}
	eff := buildAgentEffectiveTools(settings, cat, loggateway.NewNoop())
	for _, it := range eff.Items {
		if it.ToolKey != "computer_use_act" {
			continue
		}
		if !it.Enabled || it.EffectiveState != "allowed" {
			t.Fatalf("want computer_use_act allowed under spirit opt-in, got %#v", it)
		}
		return
	}
	t.Fatal("computer_use_act missing from effective items")
}
