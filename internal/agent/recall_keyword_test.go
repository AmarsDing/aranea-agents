package agent

import (
	"strings"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestRecallKeywordFromMessages_IntentHints(t *testing.T) {
	msg := trpcmodel.NewSystemMessage(`Derived intent (align your plan and tools to this JSON):
{"refined_goal":"fix login bug","search_hints":["auth","session token"]}`)
	kw := RecallKeywordFromMessages([]trpcmodel.Message{
		msg,
		trpcmodel.NewUserMessage("please help"),
	})
	if !strings.Contains(kw, "auth") || !strings.Contains(kw, "session token") {
		t.Fatalf("keyword: %q", kw)
	}
}

func TestRecallKeywordFromMessages_UserFallback(t *testing.T) {
	long := strings.Repeat("x", 200)
	kw := RecallKeywordFromMessages([]trpcmodel.Message{trpcmodel.NewUserMessage(long)})
	if len([]rune(kw)) != 120 {
		t.Fatalf("expected 120 runes, got %d", len([]rune(kw)))
	}
}
