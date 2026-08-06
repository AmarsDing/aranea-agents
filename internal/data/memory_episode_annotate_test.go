package data

import (
	"encoding/json"
	"testing"
)

// P2-R1: L2 episodes must expose the calibrated score breakdown under the
// "scores" key (same contract as annotateFactScores for L3) so the composite
// layered path can rank L2 and L3 by Scores.Total instead of raw importance.
func TestAnnotateEpisodeScores(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"id":"ep1","title":"T","importance":0.4}`)
	bd := recallScoreBreakdown{Keyword: 0.5, Vector: 0.2, Importance: 0.3, Total: 0.42}
	out := annotateEpisodeScores(raw, bd)
	var row map[string]any
	if err := json.Unmarshal(out, &row); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sc, ok := row["scores"].(map[string]any)
	if !ok {
		t.Fatalf("scores key missing: %s", out)
	}
	if got := sc["total"]; got != 0.42 {
		t.Fatalf("scores.total=%v, want 0.42", got)
	}
	// original fields preserved
	if row["title"] != "T" || row["importance"] != 0.4 {
		t.Fatalf("original fields lost: %s", out)
	}
	// invalid JSON passes through unchanged
	bad := []byte(`{oops`)
	if string(annotateEpisodeScores(bad, bd)) != string(bad) {
		t.Fatalf("invalid JSON should pass through unchanged")
	}
}
