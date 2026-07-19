package data

import (
	"math"
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────
// factDecayWithKind tests (evergreen exemption)
// ──────────────────────────────────────────────────────────

// TestFactDecayWithKind_EvergreenExempt verifies that evergreen fact kinds
// (user_identity, agent_instruction, domain_knowledge, user_preference)
// never decay regardless of age.
func TestFactDecayWithKind_EvergreenExempt(t *testing.T) {
	now := time.Now().UTC()
	veryOld := now.Add(-365 * 24 * time.Hour).Format(time.RFC3339Nano)

	evergreenKinds := []string{
		"user_identity",
		"user_preference",
		"agent_instruction",
		"domain_knowledge",
	}
	for _, kind := range evergreenKinds {
		got := factDecayWithKind(kind, veryOld, now)
		if got != 1.0 {
			t.Errorf("factDecayWithKind(%q, 365d old) = %v, want 1.0 (evergreen exempt)", kind, got)
		}
	}
}

// TestFactDecayWithKind_NonEvergreenDecays verifies that non-evergreen kinds
// decay with the standard half-life.
func TestFactDecayWithKind_NonEvergreenDecays(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano) // 60 days = 2 half-lives

	nonEvergreenKinds := []string{
		"general",
		"fact",
		"episodic",
		"",
	}
	for _, kind := range nonEvergreenKinds {
		got := factDecayWithKind(kind, old, now)
		if got >= 1.0 {
			t.Errorf("factDecayWithKind(%q, 60d old) = %v, want < 1.0 (should decay)", kind, got)
		}
		// 60 days at 30-day half-life = 0.5^2 = 0.25
		want := 0.25
		if math.Abs(got-want) > 0.01 {
			t.Errorf("factDecayWithKind(%q, 60d old) = %v, want ≈ %v", kind, got, want)
		}
	}
}

// TestFactDecayWithKind_RecentNonEvergreen verifies that recent non-evergreen
// facts barely decay.
func TestFactDecayWithKind_RecentNonEvergreen(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-1 * 24 * time.Hour).Format(time.RFC3339Nano) // 1 day

	got := factDecayWithKind("general", recent, now)
	// 1 day at 30-day half-life = 0.5^(1/30) ≈ 0.977
	if got < 0.95 || got > 1.0 {
		t.Errorf("factDecayWithKind(\"general\", 1d old) = %v, want ≈ 0.977", got)
	}
}

// TestIsEvergreenFactKind verifies the evergreen kind classifier.
func TestIsEvergreenFactKind(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"user_identity", true},
		{"user_preference", true},
		{"agent_instruction", true},
		{"domain_knowledge", true},
		{"general", false},
		{"fact", false},
		{"episodic", false},
		{"", false},
		{"USER_IDENTITY", true}, // case-insensitive
		{" user_identity ", true}, // whitespace-tolerant
	}
	for _, tt := range tests {
		got := isEvergreenFactKind(tt.kind)
		if got != tt.want {
			t.Errorf("isEvergreenFactKind(%q) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

// ──────────────────────────────────────────────────────────
// MMR diversity rerank tests
// ──────────────────────────────────────────────────────────

// TestMMRRerank_DiversityOrdering verifies that MMR pushes near-duplicates
// down and promotes diverse results.
func TestMMRRerank_DiversityOrdering(t *testing.T) {
	// 3 items: A and B are near-duplicates about cats; C is about quantum computing.
	// Raw scores: A=0.9, B=0.85, C=0.80
	// Without MMR: order is A, B, C (by score)
	// With MMR (λ=0.7): A first, then C (diverse), then B (redundant with A)
	texts := []string{
		"cats are cute fluffy animals that purr",
		"cats are cute fluffy animals that purr and meow",   // near-dup of A
		"quantum computing advances in error correction",     // completely different
	}
	scores := []float64{0.9, 0.85, 0.80}

	order := mmrRerankTexts(texts, scores, 3, 0.7)
	if len(order) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(order))
	}
	// First should be A (highest score).
	if order[0] != 0 {
		t.Errorf("first MMR pick: got index %d, want 0 (highest score)", order[0])
	}
	// Second should be C (index 2) — diverse, not B (near-dup of A).
	if order[1] != 2 {
		t.Errorf("second MMR pick: got index %d, want 2 (diverse over redundant)", order[1])
	}
	// Third is B (index 1).
	if order[2] != 1 {
		t.Errorf("third MMR pick: got index %d, want 1", order[2])
	}
}

// TestMMRRerank_Limit verifies that MMR respects the limit parameter.
func TestMMRRerank_Limit(t *testing.T) {
	texts := []string{
		"apple banana cherry",
		"apple banana cherry date",
		"completely different topic here",
		"another unique subject entirely",
	}
	scores := []float64{0.9, 0.85, 0.80, 0.75}

	order := mmrRerankTexts(texts, scores, 2, 0.7)
	if len(order) != 2 {
		t.Fatalf("expected 2 indices with limit=2, got %d", len(order))
	}
}

// TestMMRRerank_EmptyInput verifies edge cases.
func TestMMRRerank_EmptyInput(t *testing.T) {
	// Empty input.
	order := mmrRerankTexts(nil, nil, 5, 0.7)
	if len(order) != 0 {
		t.Errorf("expected 0 indices for empty input, got %d", len(order))
	}

	// Single item.
	order = mmrRerankTexts([]string{"hello"}, []float64{1.0}, 5, 0.7)
	if len(order) != 1 || order[0] != 0 {
		t.Errorf("expected [0] for single item, got %v", order)
	}

	// All identical texts — MMR should still return all (just in order).
	texts := []string{"same text", "same text", "same text"}
	scores := []float64{0.9, 0.8, 0.7}
	order = mmrRerankTexts(texts, scores, 3, 0.7)
	if len(order) != 3 {
		t.Errorf("expected 3 indices for identical texts, got %d", len(order))
	}
}

// TestMMRRerank_HighLambdaPrefersRelevance verifies that λ close to 1.0
// approximates pure score ordering (minimal diversity penalty).
func TestMMRRerank_HighLambdaPrefersRelevance(t *testing.T) {
	texts := []string{
		"cats are cute fluffy animals",
		"cats are cute fluffy animals that purr",
		"quantum computing error correction",
	}
	scores := []float64{0.9, 0.85, 0.80}

	// λ=0.99: almost pure relevance, B should still be second despite near-dup.
	order := mmrRerankTexts(texts, scores, 3, 0.99)
	if order[0] != 0 || order[1] != 1 {
		t.Errorf("high lambda should approximate score order, got %v", order)
	}
}

// TestMMRRerank_LowLambdaPrefersDiversity verifies that λ close to 0.0
// maximizes diversity at the cost of relevance.
func TestMMRRerank_LowLambdaPrefersDiversity(t *testing.T) {
	texts := []string{
		"cats are cute fluffy animals",
		"cats are cute fluffy animals that purr",
		"quantum computing error correction",
	}
	scores := []float64{0.9, 0.85, 0.80}

	// λ=0.3: heavy diversity penalty, C should be picked second.
	order := mmrRerankTexts(texts, scores, 3, 0.3)
	if order[0] != 0 {
		t.Errorf("first pick should still be highest score, got %d", order[0])
	}
	if order[1] != 2 {
		t.Errorf("low lambda should prefer diverse item, got index %d, want 2", order[1])
	}
}

// TestJaccardWordSet verifies the word-set Jaccard similarity.
func TestJaccardWordSet(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"", "", 0.0},
		{"hello", "", 0.0},
		{"", "hello", 0.0},
		{"hello world", "hello world", 1.0},
		{"hello world", "hello", 0.5},
		{"cats dogs", "birds fish", 0.0},
		{"the quick brown fox", "the quick brown fox jumps", 4.0 / 5.0},
	}
	for _, tt := range tests {
		got := jaccardWordSet(tt.a, tt.b)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("jaccardWordSet(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
