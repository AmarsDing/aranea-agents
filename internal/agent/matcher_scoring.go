package agent

import "math"

// ---------------------------------------------------------------------------
// Matcher scoring primitives.
//
// These functions are pure: no I/O, no side effects, no global state.
// They are the building blocks used by agent_matcher.go to compute a
// single number for "how well does this agent fit this task". Keeping them
// pure makes the matcher trivially testable and lets future pipelines
// (pgvector embeddings, LLM rerank) reuse the same scoring surface.
//
// Design
// ------
// The matcher blends two signals:
//
//   1. capability overlap — does the agent's role set cover the
//      required capabilities?  Computed as weighted Jaccard so that
//      asymmetric coverage (the agent has 5 roles, the user wants 2
//      specific ones) is rewarded more than naive string-set overlap.
//
//   2. semantic overlap — does the task description share vocabulary
//      with the agent's display text?  Computed as a term-frequency
//      cosine-like score in [0, 1]. Cosine on TF is the canonical
//      "vector space model" baseline from IR literature and works well
//      for short text with little training data.
//
// The two signals are blended with a tunable weight; the default biases
// toward capability overlap (capability is the business intent, semantic
// is the tie-breaker).
// ---------------------------------------------------------------------------

// MatchScoreWeights tunes how capability vs. semantic scores are blended.
type MatchScoreWeights struct {
	// Capability is the weight of the capability Jaccard score. Must be
	// in [0, 1]. Higher → trust role/capability overlap more.
	Capability float64
	// Semantic is the weight of the semantic TF score. Must be in [0, 1].
	// Higher → trust keyword similarity more.
	Semantic float64
}

// Normalize returns a copy of w with Capability + Semantic == 1, falling
// back to the default if w is invalid. The original is not mutated.
func (w MatchScoreWeights) Normalize() MatchScoreWeights {
	if w.Capability < 0 || w.Semantic < 0 {
		return DefaultMatchScoreWeights()
	}
	sum := w.Capability + w.Semantic
	if sum <= 0 {
		return DefaultMatchScoreWeights()
	}
	return MatchScoreWeights{
		Capability: w.Capability / sum,
		Semantic:   w.Semantic / sum,
	}
}

// DefaultMatchScoreWeights biases toward capability (0.7) and uses
// semantic (0.3) as a tie-breaker. Matches the legacy behaviour but
// through a single named constant so it is easy to tune.
func DefaultMatchScoreWeights() MatchScoreWeights {
	return MatchScoreWeights{Capability: 0.7, Semantic: 0.3}
}

// JaccardCapability returns the weighted Jaccard overlap between
// required and available capability sets, in [0, 1].
//
// "Weighted" means: the score reflects how much of `required` is covered
// by `available`, not just the symmetric intersection. This is the right
// question for capability matching — "did we cover what the user wanted?"
// — and avoids the false-positive where an agent with 100 generic roles
// would match every task at high Jaccard.
//
// Inputs are lowercased and trimmed before comparison. Duplicates and
// empty strings are ignored. Returns 0 when either side is empty.
func JaccardCapability(required, available []string) float64 {
	req := normalizeSet(required)
	avail := normalizeSet(available)
	if len(req) == 0 {
		return 0
	}
	hits := 0
	for k := range req {
		if _, ok := avail[k]; ok {
			hits++
		}
	}
	// coverage = |req ∩ avail| / |req|
	return float64(hits) / float64(len(req))
}

// TFSemantic returns a TF cosine score in [0, 1] between two token streams.
// The formula is:
//
//	score = Σ_t min(tf_a(t), tf_b(t)) * isf(t) / sqrt(Σ_t tf_a(t)^2 * isf(t)^2)
//
// where isf(t) is a small inverse-frequency weight (always 1 in the
// current corpus-free implementation; the architecture allows wiring a
// corpus later without changing the call sites). The √(Σ tf²) term is
// the L2 norm of the term vector, so the output is the cosine of the
// angle between the two TF vectors clipped to [0, 1].
//
// Returns 0 when either side is empty.
func TFSemantic(taskTokens, agentTokens []string) float64 {
	if len(taskTokens) == 0 || len(agentTokens) == 0 {
		return 0
	}
	tfA := termFrequency(taskTokens)
	tfB := termFrequency(agentTokens)

	// dot product over the intersection, plus per-side L2 norms.
	var dot, normA, normB float64
	for term, a := range tfA {
		normA += a * a
		if b, ok := tfB[term]; ok {
			dot += math.Min(a, b)
		}
	}
	for _, b := range tfB {
		normB += b * b
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	score := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// BlendMatchScore combines the two sub-scores with the given weights.
// Returns 0 when both inputs are 0 (degenerate case).
func BlendMatchScore(capability, semantic float64, w MatchScoreWeights) float64 {
	w = w.Normalize()
	if capability == 0 && semantic == 0 {
		return 0
	}
	return capability*w.Capability + semantic*w.Semantic
}

// ---------------------------------------------------------------------------
// Internals.
// ---------------------------------------------------------------------------

// normalizeSet trims, lowercases, and de-duplicates a string slice into a
// set. Empty strings are dropped.
func normalizeSet(in []string) map[string]struct{} {
	set := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = trimLower(s)
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	return set
}

// termFrequency returns a map from term to its (un-normalized) count in
// the input. The blend function uses raw counts in the dot product and
// the norms, so we deliberately do NOT divide by len here — that
// normalization would cancel out in the cosine anyway.
func termFrequency(tokens []string) map[string]float64 {
	tf := make(map[string]float64, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}
	return tf
}

// trimLower is a local helper to avoid pulling strings into the public
// import set just for this file. Behaviour is identical to
// strings.ToLower(strings.TrimSpace(s)).
func trimLower(s string) string {
	// Manual trim to avoid an allocation when there is no whitespace.
	start, end := 0, len(s)
	for start < end && isASCIISpace(s[start]) {
		start++
	}
	for end > start && isASCIISpace(s[end-1]) {
		end--
	}
	if start == 0 && end == len(s) {
		// Fast path: no trimming needed.
		return asciiToLower(s)
	}
	return asciiToLower(s[start:end])
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

// asciiToLower lowercases ASCII letters. Non-ASCII bytes are left alone;
// the matcher token streams are already lowercased by the tokenizer, so
// anything that reaches here is either already lowercase or is
// non-ASCII (e.g. CJK) where case-folding is irrelevant.
func asciiToLower(s string) string {
	needsLower := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			needsLower = true
			break
		}
	}
	if !needsLower {
		return s
	}
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
