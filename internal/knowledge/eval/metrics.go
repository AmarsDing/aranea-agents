package eval

// RecallAtK returns the fraction of unique expected ids found within the
// first k ranked ids. An empty expected set means nothing to recall and
// returns 1.0. k <= 0 examines no ranks and returns 0.0 unless the expected
// set is empty.
func RecallAtK(ranked, expected []string, k int) float64 {
	want, denom := expectedSet(expected)
	if denom == 0 {
		return 1
	}
	if k <= 0 {
		return 0
	}
	found := 0
	for i := 0; i < len(ranked) && i < k; i++ {
		if _, ok := want[ranked[i]]; ok {
			found++
			delete(want, ranked[i])
		}
	}
	return float64(found) / float64(denom)
}

// MRR returns the reciprocal rank of the first ranked id contained in
// expected (1.0 for a first-position hit, 1/2 for second, and so on), or 0.0
// when nothing expected is ranked. An empty expected set returns 1.0.
func MRR(ranked, expected []string) float64 {
	want, denom := expectedSet(expected)
	if denom == 0 {
		return 1
	}
	for i, id := range ranked {
		if _, ok := want[id]; ok {
			return 1 / float64(i+1)
		}
	}
	return 0
}

// HitAtK returns 1.0 when at least one expected id appears within the first
// k ranked ids, otherwise 0.0. An empty expected set returns 1.0.
func HitAtK(ranked, expected []string, k int) float64 {
	want, denom := expectedSet(expected)
	if denom == 0 {
		return 1
	}
	if k <= 0 {
		return 0
	}
	for i := 0; i < len(ranked) && i < k; i++ {
		if _, ok := want[ranked[i]]; ok {
			return 1
		}
	}
	return 0
}

// FirstHitRank returns the 1-based rank of the first ranked id contained in
// expected, or 0 when nothing expected is ranked (including empty expected).
func FirstHitRank(ranked, expected []string) int {
	want, denom := expectedSet(expected)
	if denom == 0 {
		return 0
	}
	for i, id := range ranked {
		if _, ok := want[id]; ok {
			return i + 1
		}
	}
	return 0
}

// expectedSet dedupes expected ids, dropping empties, and returns the set
// together with the unique count that metrics use as the denominator.
func expectedSet(expected []string) (map[string]struct{}, int) {
	want := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		if id == "" {
			continue
		}
		want[id] = struct{}{}
	}
	return want, len(want)
}
