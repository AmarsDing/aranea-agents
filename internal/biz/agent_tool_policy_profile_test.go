package biz

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

// profile 别名归一化守卫：canonicalToolProfile 已声明 general→coding、空串→coding，
// 但 profileAllowSet 此前按原始字符串直接查 toolProfiles——别名 profile 会静默
// 得到空工具面（而 API 展示的 Profile 却是 canonical 值，形成"显示 coding、实际
// 全 denied"的误导）。
func TestProfileAllowSet_CanonicalAliasFallback(t *testing.T) {
	catalog := []Tool{{Key: "read_file"}, {Key: "shell_exec"}, {Key: "datetime"}}

	coding := profileAllowSet("coding", catalog)
	if !coding["read_file"] || !coding["shell_exec"] || !coding["datetime"] {
		t.Fatalf("precondition: coding profile must allow fs/shell/datetime, got %v", coding)
	}
	for _, alias := range []string{"general", ""} {
		got := profileAllowSet(alias, catalog)
		if !got["read_file"] || !got["shell_exec"] || !got["datetime"] {
			t.Fatalf("profile %q must fall back to canonical (coding) allow-set, got %v", alias, got)
		}
	}
}

// 行为保持：system_admin 有独立条目（cli_admin 小工具面），canonical 虽映射到
// full，但原始条目必须优先——归一化回退不得把它放大成 full 大工具面。
func TestProfileAllowSet_SystemAdminKeepsOwnEntry(t *testing.T) {
	catalog := []Tool{{Key: "read_file"}, {Key: "web_fetch"}, {Key: "datetime"}}
	got := profileAllowSet("system_admin", catalog)
	if !got["web_fetch"] || !got["datetime"] {
		t.Fatalf("system_admin must keep web_fetch/datetime, got %v", got)
	}
	if got["read_file"] {
		t.Fatal("system_admin must NOT gain full-profile filesystem tools")
	}
}

// 端到端：general profile 的 agent 应得到与 coding 相同的有效工具面。
func TestBuildAgentEffectiveTools_GeneralProfileMatchesCoding(t *testing.T) {
	catalog := []Tool{
		{Key: "read_file", Enabled: true},
		{Key: "shell_exec", Enabled: true},
		{Key: "datetime", Enabled: true},
	}
	lg := loggateway.NewNoop()
	out := buildAgentEffectiveTools(AgentRuntimeSettings{
		ToolsEnabled: true,
		ToolsProfile: "general",
	}, catalog, lg)
	enabled := map[string]bool{}
	for _, it := range out.Items {
		enabled[it.ToolKey] = it.Enabled
	}
	if !enabled["read_file"] || !enabled["shell_exec"] {
		t.Fatalf("general profile must enable coding tools, got %+v", out.Items)
	}
	if out.Profile != "coding" {
		t.Fatalf("general must display canonical profile coding, got %q", out.Profile)
	}
}
