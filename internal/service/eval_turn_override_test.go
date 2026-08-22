package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestApplyEvalOverrideToAgentCopiesSettings(t *testing.T) {
	orig := &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsAllowJSON: `[]`}
	ag := biz.Agent{Settings: orig}
	applyEvalOverrideToAgent(&ag, biz.EvalRunOverride{Tools: "none"})
	if !orig.ToolsEnabled {
		t.Fatal("must not mutate the shared settings pointer")
	}
	if ag.Settings == nil || ag.Settings.ToolsEnabled {
		t.Fatal("override copy must disable tools")
	}
}

func TestApplyEvalOverrideAllowlist(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsAllowJSON: `[]`}}
	applyEvalOverrideToAgent(&ag, biz.EvalRunOverride{Tools: "knowledge_search,read_file"})
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		t.Fatal("allowlist must keep tools enabled")
	}
	if ag.Settings.ToolsAllowJSON == `[]` {
		t.Fatalf("allowlist not applied: %s", ag.Settings.ToolsAllowJSON)
	}
}
