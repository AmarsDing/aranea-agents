package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestNewWorkspaceSkillsBeforeHook_RequiresWorkingContract(t *testing.T) {
	t.Parallel()
	if hook := newWorkspaceSkillsBeforeHook(biz.Agent{}, TRPCBuilderDeps{}); hook != nil {
		t.Fatal("no settings must skip hook")
	}
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}}
	if hook := newWorkspaceSkillsBeforeHook(ag, TRPCBuilderDeps{}); hook != nil {
		t.Fatal("read_only must skip workspace skill FS cue")
	}
	ag.Settings.ToolsProfile = "coding"
	if hook := newWorkspaceSkillsBeforeHook(ag, TRPCBuilderDeps{}); hook == nil {
		t.Fatal("coding face must register workspace skill FS cue")
	}
}
