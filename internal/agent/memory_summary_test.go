package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/llmcontext"
)

func TestMemorySummaryCue_WrapsProfileCard(t *testing.T) {
	reader := fakeProfileCardReader{card: &biz.ProfileCard{
		AgentID: "a1", UserID: "u1", Content: "- 名叫张三\n- 喜欢咖啡",
	}}
	cue, ids, usedPinned := MemorySummaryCue(context.Background(), reader, nil, "a1", "u1")
	if usedPinned {
		t.Fatal("card present must not use pinned fallback")
	}
	if len(ids) != 0 {
		t.Fatalf("card path must not return fallback IDs, got %v", ids)
	}
	if !strings.HasPrefix(cue, "<memory_summary>") || !strings.HasSuffix(cue, "</memory_summary>") {
		t.Fatalf("expected <memory_summary> envelope, got %q", cue)
	}
	if !strings.Contains(cue, "喜欢咖啡") {
		t.Fatalf("missing card content: %q", cue)
	}
}

func TestMemorySummaryCue_FallbackFromPinnedWhenNoCard(t *testing.T) {
	stub := &preferenceListerStub{rows: [][]byte{
		pinnedRow("f1", "preference", "回复用中文"),
		pinnedRow("f2", "constraint", "不要写密钥"),
	}}
	cue, ids, usedPinned := MemorySummaryCue(context.Background(), nil, stub, "a1", "u1")
	if !usedPinned {
		t.Fatal("empty card must use pinned fallback")
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 fallback fact IDs, got %v", ids)
	}
	if !strings.Contains(cue, "<memory_summary>") || !strings.Contains(cue, "回复用中文") {
		t.Fatalf("fallback must wrap pinned prefs: %q", cue)
	}
}

func TestMemorySummaryCue_Empty(t *testing.T) {
	cue, ids, usedPinned := MemorySummaryCue(context.Background(), nil, nil, "a1", "u1")
	if cue != "" || len(ids) != 0 || usedPinned {
		t.Fatalf("empty sources must yield empty cue, got cue=%q ids=%v used=%v", cue, ids, usedPinned)
	}
}

func TestWrapMemorySummary_CapsTokens(t *testing.T) {
	// ProfileCardCue already caps at 1200 runes (~480 tokens under the
	// shared estimator), so the envelope path is tested with a raw inner
	// block that exceeds 800 tokens.
	cue := wrapMemorySummary(strings.Repeat("word ", 3000))
	if est := llmcontext.EstimateTokensFromChars(len([]rune(cue))); est > memorySummaryMaxTokens {
		t.Fatalf("summary over budget: est=%d max=%d", est, memorySummaryMaxTokens)
	}
	if !strings.HasSuffix(cue, "…") {
		t.Fatalf("over-budget summary should end with ellipsis, got suffix %q", cue[len(cue)-8:])
	}
}
