package agent

import (
	"strings"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestAsDynamicCue_UserRoleAndSentinel(t *testing.T) {
	msg := asDynamicCue("cue body")
	if msg.Role != trpcmodel.RoleUser {
		t.Fatalf("role = %s, want user", msg.Role)
	}
	if msg.ToolName != dynamicCueToolName {
		t.Fatalf("ToolName = %q, want sentinel", msg.ToolName)
	}
	if msg.Content != "cue body" {
		t.Fatalf("content = %q", msg.Content)
	}
	if !isDynamicCueMessage(msg) {
		t.Fatal("asDynamicCue must be identified as a dynamic cue")
	}
	if isDynamicCueMessage(trpcmodel.NewUserMessage("real user")) {
		t.Fatal("plain user message must not be a dynamic cue")
	}
}

func TestAppendDynamicCue_LandsAtEnd(t *testing.T) {
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("hello"),
	}
	out := appendDynamicCue(msgs, "tail cue")
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if !isDynamicCueMessage(out[2]) || out[2].Content != "tail cue" {
		t.Fatalf("tail = %+v", out[2])
	}
}

func TestReplaceDynamicCue_StripsThenAppends(t *testing.T) {
	msgs := []trpcmodel.Message{
		trpcmodel.NewUserMessage("hello"),
		asDynamicCue("<!-- aranea:knowledge_cue -->\nold"),
		trpcmodel.NewAssistantMessage("ok"),
	}
	out := replaceDynamicCue(msgs, "<!-- aranea:knowledge_cue -->\n", "<!-- aranea:knowledge_cue -->\nnew")
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3 (strip old + append new)", len(out))
	}
	if !isDynamicCueMessage(out[2]) || out[2].Content != "<!-- aranea:knowledge_cue -->\nnew" {
		t.Fatalf("replaced cue = %+v", out[2])
	}
	for _, m := range out[:2] {
		if strings.Contains(m.Content, "old") {
			t.Fatalf("old cue leaked: %+v", m)
		}
	}
}
