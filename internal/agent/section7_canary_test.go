package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/llmcontext"
	"aranea-agents/internal/memory"
)

// §7 canaries from docs/reports/2026-08-22-analysis-codex-vs-aranea.md:
// 4. Spirit 隔日：无需重申 3 条以上稳定偏好（C1 / memory_summary）
// 5. use_count 与「prompt 里真正出现的条目」相关系数转正（C2）
// plus the token cap already required by C1.

func TestSection7Canary_NextDayPinnedPrefs(t *testing.T) {
	t.Parallel()
	stub := &preferenceListerStub{rows: [][]byte{
		pinnedRow("f1", "preference", "回复用中文"),
		pinnedRow("f2", "preference", "喜欢短句"),
		pinnedRow("f3", "constraint", "不要写密钥"),
	}}
	cue, ids, usedPinned := MemorySummaryCue(context.Background(), nil, stub, "spirit", "u-canary")
	if !usedPinned {
		t.Fatal("cold start without profile card must fall back to pinned prefs")
	}
	if len(ids) < 3 {
		t.Fatalf("§7.4 wants ≥3 stable prefs, got ids=%v cue=%q", ids, cue)
	}
	for _, want := range []string{"回复用中文", "喜欢短句", "不要写密钥"} {
		if !strings.Contains(cue, want) {
			t.Fatalf("memory_summary missing %q: %q", want, cue)
		}
	}
	if !strings.Contains(cue, "<memory_summary>") {
		t.Fatalf("must wrap the Codex-style envelope: %q", cue)
	}
	// C2: the IDs returned here are the inject-bump set, not a recall-side
	// use_count increment. The honest "appeared in the prompt" metric is
	// injected_count (FR-12.6 / bumpFactInjectedCounts).
	wantIDs := map[string]bool{"f1": true, "f2": true, "f3": true}
	for _, id := range ids {
		if !wantIDs[id] {
			t.Fatalf("unexpected inject id %q in %v", id, ids)
		}
		delete(wantIDs, id)
	}
	if len(wantIDs) != 0 {
		t.Fatalf("inject ids missing %v (got %v)", wantIDs, ids)
	}
}

func TestSection7Canary_MemorySummaryTokenCap(t *testing.T) {
	t.Parallel()
	cue := wrapMemorySummary(strings.Repeat("偏好条目 ", 4000))
	if est := llmcontext.EstimateTokensFromChars(len([]rune(cue))); est > memorySummaryMaxTokens {
		t.Fatalf("§7 token canary: est=%d exceeds %d", est, memorySummaryMaxTokens)
	}
}

func TestSection7Canary_RecallDoesNotIncrementUseCount(t *testing.T) {
	t.Parallel()
	store := &section7UseCountStore{}
	svc := memory.NewReconsolidationService(store, nil, nil)
	if err := svc.OnRecall(context.Background(), "entity-1", []string{"entity-2"}); err != nil {
		t.Fatalf("OnRecall: %v", err)
	}
	if store.incremented {
		t.Fatal("§7.5: use_count must not increment on recall")
	}
	if !store.boosted {
		t.Fatal("OnRecall must still boost activation")
	}
}

type section7UseCountStore struct {
	incremented bool
	boosted     bool
}

func (s *section7UseCountStore) BoostActivation(context.Context, string, float64, string) (bool, error) {
	s.boosted = true
	return true, nil
}

func (s *section7UseCountStore) IncrementUseCount(context.Context, string) (bool, error) {
	s.incremented = true
	return true, nil
}
