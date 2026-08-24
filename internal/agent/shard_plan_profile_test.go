package agent

import (
	"reflect"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// profile 别名归一化：general（canonical=coding）的核心/延迟分离必须与 coding
// 完全一致——否则编码核心工具（read_file→file、shell_exec→hostexec）被错误降级
// 为 deferred，模型失去全 schema 仅剩目录 cue（展示层却声称 profile=coding）。
func TestResolveDeferredToolNames_GeneralProfileMatchesCoding(t *testing.T) {
	eff := map[string]bool{"read_file": true, "shell_exec": true, "web_fetch": true, "datetime": true}
	lg := loggateway.NewNoop()

	general := resolveDeferredToolNames(biz.Agent{ID: "a", Settings: &biz.AgentRuntimeSettings{ToolsProfile: "general"}}, eff, lg)
	coding := resolveDeferredToolNames(biz.Agent{ID: "b", Settings: &biz.AgentRuntimeSettings{ToolsProfile: "coding"}}, eff, lg)
	if !reflect.DeepEqual(general, coding) {
		t.Fatalf("general profile must split exactly like coding:\ngeneral=%v\ncoding =%v", general, coding)
	}
	// 且编码核心工具（shell_exec→hostexec）不得出现在延迟名单。
	for _, n := range general {
		if n == "hostexec" {
			t.Fatalf("coding-core tool shell_exec must stay resident, got %v", general)
		}
	}
}

// 行为保持：未知 profile 仍回退 defaultCoreResidentTools（仅 datetime 常驻）。
func TestResolveDeferredToolNames_UnknownProfileUnchanged(t *testing.T) {
	eff := map[string]bool{"read_file": true, "web_fetch": true, "datetime": true}
	lg := loggateway.NewNoop()

	out := resolveDeferredToolNames(biz.Agent{ID: "b", Settings: &biz.AgentRuntimeSettings{ToolsProfile: "nonexistent"}}, eff, lg)
	seen := map[string]bool{}
	for _, n := range out {
		seen[n] = true
	}
	// read_file→file、web_fetch→httpfetch 必须降级；datetime 必须常驻。
	if !seen["file"] || !seen["httpfetch"] {
		t.Fatalf("unknown profile must defer non-default-core tools, got %v", out)
	}
	if seen["datetime"] {
		t.Fatalf("datetime must stay resident for unknown profile, got %v", out)
	}
}
