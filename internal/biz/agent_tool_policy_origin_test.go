package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

// M81 对账钉：granted_tool 边的 grant_origin 证据完全来自
// EffectiveAgentTool.Origin——本测试钉住运行时侧三种 origin 的产出语义
// （profile 隐含 / 显式 allow / override 抬升），以及 denied 行 origin 为空。
func findEffectiveItem(eff AgentEffectiveTools, key string) *EffectiveAgentTool {
	for i := range eff.Items {
		if eff.Items[i].ToolKey == key {
			return &eff.Items[i]
		}
	}
	return nil
}

func TestBuildAgentEffectiveTools_GrantOrigin(t *testing.T) {
	catalog := []Tool{
		{Key: "read_file", DisplayName: "Read", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "custom_tool", DisplayName: "Custom", Category: "custom", Source: "builtin", Enabled: true},
		{Key: "other_tool", DisplayName: "Other", Category: "custom", Source: "builtin", Enabled: true},
	}
	settings := AgentRuntimeSettings{
		ToolsEnabled:   true,
		ToolsProfile:   "safe", // canonical read_only：隐含 read_file
		ToolsAllowJSON: `["custom_tool"]`,
		ToolsDenyJSON:  "[]",
	}
	eff := buildAgentEffectiveTools(settings, catalog, loggateway.NewNoop())

	if it := findEffectiveItem(eff, "read_file"); it == nil || it.EffectiveState != "allowed" || it.Origin != "profile" {
		t.Fatalf("read_file want allowed/profile, got %+v", it)
	}
	if it := findEffectiveItem(eff, "custom_tool"); it == nil || it.EffectiveState != "allowed" || it.Origin != "allow" {
		t.Fatalf("custom_tool want allowed/allow, got %+v", it)
	}
	if it := findEffectiveItem(eff, "other_tool"); it == nil || it.EffectiveState != "denied" || it.Origin != "" {
		t.Fatalf("other_tool want denied/origin-empty, got %+v", it)
	}
}

func TestApplyAgentToolOverrides_GrantOrigin(t *testing.T) {
	catalog := []Tool{
		{Key: "read_file", DisplayName: "Read", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "other_tool", DisplayName: "Other", Category: "custom", Source: "builtin", Enabled: true},
	}
	settings := AgentRuntimeSettings{
		ToolsEnabled:   true,
		ToolsProfile:   "safe",
		ToolsAllowJSON: "[]",
		ToolsDenyJSON:  "[]",
	}
	eff := buildAgentEffectiveTools(settings, catalog, loggateway.NewNoop())

	ApplyAgentToolOverrides(&eff, catalog, []ToolAgentOverride{
		{ToolKey: "other_tool", Mode: "allow"},
		{ToolKey: "read_file", Mode: "deny"},
	})

	if it := findEffectiveItem(eff, "other_tool"); it == nil || it.EffectiveState != "allowed" || it.Origin != "override" {
		t.Fatalf("override allow want allowed/override, got %+v", it)
	}
	if it := findEffectiveItem(eff, "read_file"); it == nil || it.EffectiveState != "denied" {
		t.Fatalf("override deny want denied, got %+v", it)
	}
}
