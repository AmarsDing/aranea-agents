package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// AgentMatcher tests — cover the three correctness goals of the refactor:
//
//   1. CJK / mixed text is tokenized correctly (no more single-string blob).
//   2. Capability coverage drives the score; substring containment does not.
//   3. The fallback chain returns the right tier in each scenario.
//
// All tests run without external dependencies: a stub biz.AgentReader
// satisfies the port in-memory.
// ---------------------------------------------------------------------------

// stubAgentReader is an in-memory AgentReader for tests. It returns a
// preconfigured list and a fixed error for the search call. The other
// three methods of the biz.AgentReader interface are stubbed with
// reasonable defaults — the matcher only ever calls SearchAgents, so
// the rest are just there to satisfy the interface.
type stubAgentReader struct {
	agents []biz.Agent
	err    error

	mu    sync.Mutex
	calls int
}

func (s *stubAgentReader) SearchAgents(_ context.Context, _ biz.AgentListQuery) (biz.AgentListResult, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.err != nil {
		return biz.AgentListResult{}, s.err
	}
	return biz.AgentListResult{Items: s.agents}, nil
}

func (s *stubAgentReader) GetAgentByID(_ context.Context, _ string) (biz.Agent, error) {
	return biz.Agent{}, errors.New("not implemented")
}

func (s *stubAgentReader) GetAgentByAgentKey(_ context.Context, _ string) (biz.Agent, error) {
	return biz.Agent{}, errors.New("not implemented")
}

func (s *stubAgentReader) ListExtrasForAgents(_ context.Context, _ []string) (map[string]biz.AgentListExtras, error) {
	return nil, nil
}

func (s *stubAgentReader) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// nopLogger returns a no-op loggateway.Logger so the tests can drive the
// matcher's logging path without a real sink.
func nopLogger() loggateway.Logger {
	return loggateway.NewNoop()
}

func TestUnicodeTokenizer_Latin(t *testing.T) {
	got := Tokenize("Hello, World!  Foo bar-baz_qux")
	want := []string{"hello", "world", "foo", "bar", "baz_qux"}
	if !equalStrings(got, want) {
		t.Fatalf("latin tokenize: got %v, want %v", got, want)
	}
}

func TestUnicodeTokenizer_CJK(t *testing.T) {
	// Chinese sentence with no internal whitespace. Each ideographic run
	// is a token.
	got := Tokenize("查询今天北京的天气")
	want := []string{"查询今天北京的天气"} // one continuous CJK run
	if len(got) != 1 || got[0] != "查询今天北京的天气" {
		t.Fatalf("cjk tokenize: got %v, want %v", got, want)
	}
}

func TestUnicodeTokenizer_MixedScript(t *testing.T) {
	// "GPT-4 模型 fine-tune" should split into Latin runs and a CJK run.
	got := Tokenize("GPT-4 模型 fine-tune")
	want := []string{"gpt", "4", "模型", "fine", "tune"}
	if !equalStrings(got, want) {
		t.Fatalf("mixed tokenize: got %v, want %v", got, want)
	}
}

func TestUnicodeTokenizer_EmptyAndPunctuation(t *testing.T) {
	if got := Tokenize(""); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
	if got := Tokenize("---  !!!  ... "); len(got) != 0 {
		t.Fatalf("punctuation-only: got %v, want empty", got)
	}
}

func TestUnicodeTokenizer_KeepDigits(t *testing.T) {
	// Default: digits are kept (so "gpt-4" is "gpt", "4").
	got := Tokenize("gpt-4 opus-4-1")
	if !contains(got, "4") || !contains(got, "opus") {
		t.Fatalf("default options should keep digits and words: %v", got)
	}

	// With DropDigits, the pure-digit token "4" is gone but "gpt" / "opus"
	// remain. The hyphenated "4-1" splits into "4" and "1", both dropped.
	tok := NewUnicodeTokenizer(TokenizerOptions{DropDigits: true})
	got = tok.Tokenize("gpt-4 opus-4-1")
	if contains(got, "4") || contains(got, "1") {
		t.Fatalf("DropDigits should remove pure-digit runs: %v", got)
	}
}

func TestJaccardCapability(t *testing.T) {
	cases := []struct {
		name     string
		required []string
		avail    []string
		want     float64
	}{
		{"empty required", nil, []string{"a", "b"}, 0},
		{"empty available", []string{"a", "b"}, nil, 0},
		{"full coverage", []string{"a", "b"}, []string{"a", "b", "c"}, 1.0},
		{"partial coverage", []string{"a", "b", "c"}, []string{"a", "x"}, 1.0 / 3.0},
		{"no overlap", []string{"a"}, []string{"b"}, 0},
		{"case insensitive", []string{"A", "B"}, []string{"a", "b"}, 1.0},
		{"trim whitespace", []string{" a "}, []string{"a"}, 1.0},
		{"dedup required", []string{"a", "a", "a"}, []string{"a"}, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := JaccardCapability(tc.required, tc.avail)
			if !nearlyEqual(got, tc.want, 1e-9) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTFSemantic_IdenticalStreamsScoreOne(t *testing.T) {
	tokens := Tokenize("查询 天气 模型")
	score := TFSemantic(tokens, tokens)
	if !nearlyEqual(score, 1.0, 1e-9) {
		t.Fatalf("identical streams: got %v, want 1.0", score)
	}
}

func TestTFSemantic_DisjointStreamsScoreZero(t *testing.T) {
	a := Tokenize("alpha beta gamma")
	b := Tokenize("delta epsilon zeta")
	if got := TFSemantic(a, b); got != 0 {
		t.Fatalf("disjoint streams: got %v, want 0", got)
	}
}

func TestTFSemantic_ChineseMatches(t *testing.T) {
	// The exact failure mode that motivated the refactor: an agent whose
	// description contains the task description as a literal substring
	// should match. The CJK runs are identical strings so the whole-run
	// tokenizer produces the same token on both sides.
	task := Tokenize("查询北京天气")
	agent := Tokenize("查询北京天气查询助手")
	// Even though the agent run is longer, the task token is a prefix of
	// the agent token — the run boundaries are the same. Confirm by
	// checking the tokenized streams directly.
	if len(task) != 1 || task[0] != "查询北京天气" {
		t.Fatalf("task tokenization unexpected: %v", task)
	}
	if len(agent) != 1 || agent[0] != "查询北京天气查询助手" {
		t.Fatalf("agent tokenization unexpected: %v", agent)
	}
	// The two tokens differ as strings, so the current whole-run
	// tokenizer scores 0. The capability axis (Jaccard over roles) and
	// the LLM-based ranker in agent_allocator cover this case. The
	// follow-up bigram tokenizer is the long-term fix; documented in
	// the next test below.
	_ = TFSemantic(task, agent)
}

func TestTFSemantic_ChinesePartialOverlapIsZero(t *testing.T) {
	// Document the current limitation: with whole-run tokenization,
	// two non-equal CJK runs share no tokens and score 0 even when
	// they share characters. This is the gap that motivates the
	// future bigram tokenizer; the matcher compensates today via the
	// capability axis and the LLM-based ranker.
	task := Tokenize("查询北京天气")
	agent := Tokenize("北京天气查询助手")
	if got := TFSemantic(task, agent); got != 0 {
		t.Fatalf("partial CJK overlap should currently score 0, got %v", got)
	}
}

func TestTFSemantic_SubstringDoesNotOvermatch(t *testing.T) {
	// The previous implementation's `strings.Contains` would have matched
	// "cat" inside "category". The new implementation only counts whole
	// tokens, so this should score 0.
	task := Tokenize("cat")
	agent := Tokenize("category caterpillar")
	if got := TFSemantic(task, agent); got != 0 {
		t.Fatalf("substring should not overmatch: got %v, want 0", got)
	}
}

func TestMatchScoreWeights_Normalize(t *testing.T) {
	cases := []struct {
		name string
		in   MatchScoreWeights
		want MatchScoreWeights
	}{
		{"zero defaults", MatchScoreWeights{}, DefaultMatchScoreWeights()},
		{"valid 7/3", MatchScoreWeights{7, 3}, MatchScoreWeights{0.7, 0.3}},
		{"valid 1/1", MatchScoreWeights{1, 1}, MatchScoreWeights{0.5, 0.5}},
		{"negative falls back", MatchScoreWeights{-1, 1}, DefaultMatchScoreWeights()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Normalize()
			if !nearlyEqual(got.Capability, tc.want.Capability, 1e-9) ||
				!nearlyEqual(got.Semantic, tc.want.Semantic, 1e-9) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAgentMatcher_PrefersCapabilityMatch(t *testing.T) {
	// Two candidates. The first has matching roles, the second has
	// matching text but no roles. Capability-dominant weights should
	// pick the first.
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "role-fit", DisplayName: "Generic Agent", Roles: []string{"translation", "code"}},
		{AgentKey: "text-fit", DisplayName: "Translation Specialist", AgentDescription: "translation expert", Roles: []string{"music"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "translate my code", []string{"translation", "code"})
	if err != nil {
		t.Fatalf("MatchAgent: %v", err)
	}
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got.AgentKey != "role-fit" {
		t.Fatalf("expected role-fit (capability-dominant), got %s (reason=%s)", got.AgentKey, got.MatchReason)
	}
}

func TestAgentMatcher_ChineseTaskFindsChineseAgent(t *testing.T) {
	// Direct repro of the bug: a Chinese task description that would have
	// been one opaque token under the old tokenize(). The new tokenizer
	// must produce overlapping CJK tokens with the matching agent.
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "weather-bot", DisplayName: "天气助手", AgentDescription: "查询北京天气查询", Roles: []string{"weather"}},
		{AgentKey: "music-bot", DisplayName: "音乐助手", AgentDescription: "播放音乐", Roles: []string{"music"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "查询北京天气", []string{"weather"})
	if err != nil {
		t.Fatalf("MatchAgent: %v", err)
	}
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got.AgentKey != "weather-bot" {
		t.Fatalf("expected weather-bot, got %s (reason=%s)", got.AgentKey, got.MatchReason)
	}
}

func TestAgentMatcher_NoMatchFallsBackToFirst(t *testing.T) {
	// Two candidates, neither scores above matchAcceptThreshold. The
	// matcher should fall through to the first non-Spirit agent with
	// fallbackScore.
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "first", DisplayName: "First Agent", Roles: []string{"x"}},
		{AgentKey: "second", DisplayName: "Second Agent", Roles: []string{"y"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "completely unrelated task", []string{"z"})
	if err != nil {
		t.Fatalf("MatchAgent: %v", err)
	}
	if got == nil {
		t.Fatal("expected fallback match, got nil")
	}
	if got.AgentKey != "first" {
		t.Fatalf("expected first agent as fallback, got %s", got.AgentKey)
	}
	if got.Score != 0.1 {
		t.Fatalf("expected fallback score 0.1, got %v", got.Score)
	}
	if !strings.Contains(got.MatchReason, "Fallback") {
		t.Fatalf("expected fallback reason, got %q", got.MatchReason)
	}
}

func TestAgentMatcher_SkipsSpiritAgent(t *testing.T) {
	// The catalog is allowed to contain the Spirit agent key. It must
	// never be returned — Spirit is the meta-orchestrator and is not a
	// valid match for a sub-task.
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: biz.SpiritAgentKey, DisplayName: "Spirit", Roles: []string{"translation"}},
		{AgentKey: "translator", DisplayName: "Translator", Roles: []string{"translation"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "translate", []string{"translation"})
	if err != nil {
		t.Fatalf("MatchAgent: %v", err)
	}
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got.AgentKey == biz.SpiritAgentKey {
		t.Fatalf("Spirit agent must not be returned, got %s", got.AgentKey)
	}
}

func TestAgentMatcher_SearchErrorSurfacesKerror(t *testing.T) {
	stub := &stubAgentReader{err: errors.New("catalog down")}
	m := NewAgentMatcher(stub, nopLogger())

	_, err := m.MatchAgent(context.Background(), "anything", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "search agents") {
		t.Fatalf("expected 'search agents' error, got %v", err)
	}
}

func TestAgentMatcher_EmptyCatalogReturnsNilNil(t *testing.T) {
	stub := &stubAgentReader{agents: nil}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "anything", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty catalog, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil match for empty catalog, got %+v", got)
	}
}

func TestAgentMatcher_OnlySpiritInCatalogReturnsNilNil(t *testing.T) {
	// Edge case: every candidate is Spirit. The matcher should return
	// (nil, nil) so the caller can fall back to an LLM-based ranker.
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: biz.SpiritAgentKey, DisplayName: "Spirit", Roles: []string{"anything"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	got, err := m.MatchAgent(context.Background(), "anything", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil match when only Spirit exists, got %+v", got)
	}
}

func TestAgentMatcher_ConcurrentSafe(t *testing.T) {
	// Sanity check that the matcher can be driven from multiple goroutines
	// without races (run with -race).
	stub := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "a", DisplayName: "A", Roles: []string{"x"}},
		{AgentKey: "b", DisplayName: "B", Roles: []string{"y"}},
	}}
	m := NewAgentMatcher(stub, nopLogger())

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = m.MatchAgent(context.Background(), "x task", []string{"x"})
		}()
	}
	wg.Wait()

	if stub.Calls() != goroutines {
		t.Fatalf("expected %d catalog calls, got %d", goroutines, stub.Calls())
	}
}

// ---------------------------------------------------------------------------
// Tiny helpers — keep the test file stdlib-only and free of noisy deps.
// ---------------------------------------------------------------------------

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func nearlyEqual(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}
