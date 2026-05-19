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

func TestApplyOverrideToEffectiveItem_inheritDisabled(t *testing.T) {
	item := EffectiveAgentTool{ToolKey: "web_fetch", Enabled: true, EffectiveState: "allowed"}
	applyOverrideToEffectiveItem(&item, ToolAgentOverride{Mode: "inherit", Enabled: false}, true)
	if item.Enabled {
		t.Fatalf("inherit disabled: expected off, got enabled")
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
