package agent

import "testing"

func TestDoomLoopDetector_DetectsRepetition(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.95) // 3 repeats, 95% similarity
	texts := []string{
		"I need to check the file.",
		"I need to check the file.",
		"I need to check the file.",
	}
	for _, text := range texts {
		if d.Observe(text) {
			return // detected on the 3rd identical text
		}
	}
	t.Fatal("expected doom loop detection after 3 identical texts")
}

func TestDoomLoopDetector_NoFalsePositiveOnDistinct(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.95)
	texts := []string{
		"First, I will read the configuration file.",
		"Now I am writing the new handler function.",
		"Next step: run the unit tests to verify.",
		"The tests all pass, so I will commit now.",
		"Finally, update the documentation accordingly.",
	}
	for _, text := range texts {
		if d.Observe(text) {
			t.Fatalf("unexpected doom loop detection for distinct text: %q", text)
		}
	}
}

func TestDoomLoopDetector_NearIdentical(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.9)
	// 22 words differing by exactly 1 word → Jaccard = 21/23 ≈ 0.913 ≥ 0.9.
	base := "the quick brown fox jumps over the lazy dog and then runs through the dense green forest near the river every single morning"
	variant1 := "the quick brown fox jumps over the lazy dog and then runs through the dense green forest near the river every single evening"
	variant2 := "the quick brown fox jumps over the lazy dog and then runs through the dense green forest near the river every single weekend"
	for _, text := range []string{base, variant1, variant2} {
		if d.Observe(text) {
			return
		}
	}
	t.Fatal("expected doom loop detection for near-identical texts")
}

func TestDoomLoopDetector_RecoveryAfterDistinct(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.95)
	d.Observe("repeat me please")
	d.Observe("repeat me please")
	// A distinct text resets the consecutive-similar counter.
	d.Observe("something entirely different here")
	if d.Observe("repeat me please") {
		t.Fatal("should not detect after counter reset (only 2 consecutive)")
	}
	if d.Observe("repeat me please") {
		t.Fatal("should not detect at 2 consecutive after reset")
	}
	if !d.Observe("repeat me please") {
		t.Fatal("expected detection at 3 consecutive after reset")
	}
}

func TestDoomLoopDetector_IgnoresEmpty(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.95)
	for i := 0; i < 10; i++ {
		if d.Observe("") {
			t.Fatal("empty text must never trigger detection")
		}
		if d.Observe("   \n  ") {
			t.Fatal("whitespace-only text must never trigger detection")
		}
	}
}
