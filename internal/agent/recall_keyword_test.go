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
	runes := []rune(kw)
	if len(runes) != 121 {
		t.Fatalf("expected 121 runes (120 truncated + ellipsis), got %d", len(runes))
	}
	if !strings.HasSuffix(kw, "…") {
		t.Fatalf("expected truncation ellipsis, got %q", kw)
	}
}
