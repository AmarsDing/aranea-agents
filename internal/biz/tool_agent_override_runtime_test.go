package biz

import "testing"

func TestApplyOverrideToEffectiveItem_deny(t *testing.T) {
	item := EffectiveAgentTool{ToolKey: "shell_exec", Enabled: true, EffectiveState: "allowed", Reason: "profile:full"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "deny"}, true)
	if item.Enabled || item.Reason != "override_deny" {
		t.Fatalf("deny override: enabled=%v reason=%q", item.Enabled, item.Reason)
	}
}

func TestApplyOverrideToEffectiveItem_allow(t *testing.T) {
	item := EffectiveAgentTool{ToolKey: "shell_exec", Enabled: false, EffectiveState: "denied", Reason: "catalog_off"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "allow"}, true)
	if !item.Enabled || item.Reason != "override_allow" {
		t.Fatalf("allow override: enabled=%v reason=%q", item.Enabled, item.Reason)
	}
}

func TestApplyOverrideToEffectiveItem_inheritPreservesAllowed(t *testing.T) {
	// Inherit mode should preserve the original computed state regardless of o.Enabled.
	item := EffectiveAgentTool{ToolKey: "web_fetch", Enabled: true, EffectiveState: "allowed", Reason: "profile:coding"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "inherit", Enabled: false}, true)
	if !item.Enabled || item.EffectiveState != "allowed" || item.Reason != "profile:coding" {
		t.Fatalf("inherit should preserve original state: enabled=%v state=%q reason=%q", item.Enabled, item.EffectiveState, item.Reason)
	}
}

func TestApplyOverrideToEffectiveItem_inheritPreservesDenied(t *testing.T) {
	// Inherit mode should preserve the original denied state regardless of o.Enabled=true.
	item := EffectiveAgentTool{ToolKey: "shell_exec", Enabled: false, EffectiveState: "denied", Reason: "catalog_off"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "inherit", Enabled: true}, true)
	if item.Enabled || item.EffectiveState != "denied" || item.Reason != "catalog_off" {
		t.Fatalf("inherit should preserve original denied state: enabled=%v state=%q reason=%q", item.Enabled, item.EffectiveState, item.Reason)
	}
}

func TestApplyOverrideToEffectiveItem_inheritEmptyMode(t *testing.T) {
	// Empty mode defaults to inherit — should also preserve original state.
	item := EffectiveAgentTool{ToolKey: "read_file", Enabled: true, EffectiveState: "allowed", Reason: "profile:coding"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "", Enabled: false}, true)
	if !item.Enabled || item.EffectiveState != "allowed" || item.Reason != "profile:coding" {
		t.Fatalf("empty mode (inherit) should preserve original state: enabled=%v state=%q reason=%q", item.Enabled, item.EffectiveState, item.Reason)
	}
}

func TestMergeToolConfigJSON_overrideWins(t *testing.T) {
	out := MergeToolConfigJSON(`{"api_key":"global","cx":"old"}`, `{"api_key":"agent","filesystem_dir":"/tmp/ws"}`)
	if out["api_key"] != "agent" {
		t.Fatalf("api_key=%v", out["api_key"])
	}
	if out["cx"] != "old" {
		t.Fatalf("cx=%v", out["cx"])
	}
	if out["filesystem_dir"] != "/tmp/ws" {
		t.Fatalf("filesystem_dir=%v", out["filesystem_dir"])
	}
}
