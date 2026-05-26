package agent

import (
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestAnalyzePromptRequest(t *testing.T) {
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("You are bot.\n\n## Runtime capability policy (system)\n- Tools: enabled"),
		trpcmodel.NewSystemMessage("## L3 semantic memory (user facts)\n- likes coffee"),
		trpcmodel.NewUserMessage("hello"),
	}
	r := analyzePromptRequest(msgs)
	if r.SystemMsgs != 2 || r.UserMsgs != 1 {
		t.Fatalf("counts: %#v", r)
	}
	if r.EstTokens <= 0 {
		t.Fatalf("est tokens: %d", r.EstTokens)
	}
	if r.Sections["runtime_cue"] == 0 {
		t.Fatalf("sections: %#v", r.Sections)
	}
}

func TestClassifySystemSections_Intent(t *testing.T) {
	sec := classifySystemSections("Derived intent (align your plan and tools to this JSON):\n{\"refined_goal\":\"x\"}")
	if sec["intent"] == 0 {
		t.Fatalf("intent section: %#v", sec)
	}
}

func TestPromptSnapshotEnabled(t *testing.T) {
	t.Setenv("ARANEA_PROMPT_SNAPSHOT", "0")
	if promptSnapshotEnabled() {
		t.Fatal("expected disabled")
	}
	t.Setenv("ARANEA_PROMPT_SNAPSHOT", "")
	if !promptSnapshotEnabled() {
		t.Fatal("expected default enabled")
	}
}
