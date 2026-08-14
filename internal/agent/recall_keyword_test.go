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

// TestRecallKeywordFromMessages_StripsFillerPrefix verifies query cleaning
// (P1-1): politeness/filler prefixes must not pollute the recall query —
// they waste the 120-rune budget and drag the embedding away from the
// actual question.
func TestRecallKeywordFromMessages_StripsFillerPrefix(t *testing.T) {
	kw := RecallKeywordFromMessages([]trpcmodel.Message{
		trpcmodel.NewUserMessage("请直接回答：我叫什么名字？"),
	})
	if !strings.Contains(kw, "我叫什么名字") {
		t.Fatalf("expected core question kept, got %q", kw)
	}
	if strings.Contains(kw, "请直接回答") {
		t.Fatalf("filler prefix must be stripped, got %q", kw)
	}
}

// TestRecallKeywordFromMessages_MultiQuestionPreserved verifies multi-query
// expansion (P1-1): a multi-question user turn is split into focused
// sub-queries (joined in one query string) instead of one punctuation-laden
// blob, so each aspect participates in keyword/vector matching.
func TestRecallKeywordFromMessages_MultiQuestionPreserved(t *testing.T) {
	kw := RecallKeywordFromMessages([]trpcmodel.Message{
		trpcmodel.NewUserMessage("我叫什么名字？我喜欢喝什么？我的猫叫什么？"),
	})
	for _, sub := range []string{"我叫什么名字", "我喜欢喝什么", "我的猫叫什么"} {
		if !strings.Contains(kw, sub) {
			t.Fatalf("sub-query %q must survive cleaning, got %q", sub, kw)
		}
	}
	if strings.ContainsAny(kw, "？?。") {
		t.Fatalf("sentence punctuation must be normalized away, got %q", kw)
	}
}

// TestRecallKeywordFromMessages_LongPreambleTailBias verifies that when the
// cleaned query exceeds the rune budget, leading chit-chat is dropped first
// (question-last bias): the actual question at the tail is what memory
// recall must match.
func TestRecallKeywordFromMessages_LongPreambleTailBias(t *testing.T) {
	preamble := strings.Repeat("最近天气真不错我想随便聊聊，", 8) // 8×14=112 runes 闲聊铺垫
	question := "goroutine泄漏怎么排查"
	kw := RecallKeywordFromMessages([]trpcmodel.Message{
		trpcmodel.NewUserMessage(preamble + "。" + question + "？"),
	})
	if !strings.Contains(kw, question) {
		t.Fatalf("尾部问题必须在预算内保留（tail-biased packing）, got %q", kw)
	}
	if len([]rune(kw)) > 121 {
		t.Fatalf("query must fit the 120-rune budget, got %d runes", len([]rune(kw)))
	}
}

// TestRecallKeywordFromMessages_FillerOnlyFallsBack verifies that an input
// consisting entirely of filler ("你好") does not collapse to an empty
// query — the original text is kept as the recall keyword.
func TestRecallKeywordFromMessages_FillerOnlyFallsBack(t *testing.T) {
	kw := RecallKeywordFromMessages([]trpcmodel.Message{
		trpcmodel.NewUserMessage("你好"),
	})
	if strings.TrimSpace(kw) == "" {
		t.Fatalf("filler-only input must fall back to the original text, got empty")
	}
}
