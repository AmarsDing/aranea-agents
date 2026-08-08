package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
)

// P3-5 dataset content hash (review BP13: pure-logic unit coverage).

func TestHashEvalCasesOrderIndependent(t *testing.T) {
	a := biz.EvalCase{ID: "c-1", Input: "q1", ExpectedOutput: "a1", MetadataJSON: "{}"}
	b := biz.EvalCase{ID: "c-2", Input: "q2", ExpectedOutput: "a2", MetadataJSON: `{"k":"v"}`}
	h1 := hashEvalCases([]biz.EvalCase{a, b})
	h2 := hashEvalCases([]biz.EvalCase{b, a})
	if h1 != h2 {
		t.Fatalf("hash must not depend on listing order: %s vs %s", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("expected 16 hex chars, got %d (%s)", len(h1), h1)
	}
}

func TestHashEvalCasesContentSensitive(t *testing.T) {
	base := biz.EvalCase{ID: "c-1", Input: "q1", ExpectedOutput: "a1", MetadataJSON: "{}"}
	baseHash := hashEvalCases([]biz.EvalCase{base})

	variants := []struct {
		name string
		c    biz.EvalCase
	}{
		{"input changed", biz.EvalCase{ID: "c-1", Input: "q1!", ExpectedOutput: "a1", MetadataJSON: "{}"}},
		{"expected changed", biz.EvalCase{ID: "c-1", Input: "q1", ExpectedOutput: "a2", MetadataJSON: "{}"}},
		{"metadata changed", biz.EvalCase{ID: "c-1", Input: "q1", ExpectedOutput: "a1", MetadataJSON: `{"rubric":"x"}`}},
		{"case added", biz.EvalCase{ID: "c-2", Input: "q2"}},
	}
	for _, v := range variants {
		var got string
		if v.name == "case added" {
			got = hashEvalCases([]biz.EvalCase{base, v.c})
		} else {
			got = hashEvalCases([]biz.EvalCase{v.c})
		}
		if got == baseHash {
			t.Errorf("%s: hash must change, still %s", v.name, got)
		}
	}
}

func TestHashEvalCasesEmptyDeterministic(t *testing.T) {
	if hashEvalCases(nil) != hashEvalCases([]biz.EvalCase{}) {
		t.Fatal("empty dataset hash must be deterministic")
	}
}

// TestHashEvalCasesNoFieldBoundaryAmbiguity guards the NUL-separator contract:
// concatenated field pairs that collide without separators must hash differently.
func TestHashEvalCasesNoFieldBoundaryAmbiguity(t *testing.T) {
	h1 := hashEvalCases([]biz.EvalCase{{ID: "ab", Input: "c"}})
	h2 := hashEvalCases([]biz.EvalCase{{ID: "a", Input: "bc"}})
	if h1 == h2 {
		t.Fatal("field-boundary ambiguity: [ab,c] and [a,bc] must not collide")
	}
}
