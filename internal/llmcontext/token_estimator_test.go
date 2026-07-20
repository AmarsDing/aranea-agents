package llmcontext

import "testing"

func TestTokenEstimator_DualAnchor(t *testing.T) {
	e := NewTokenEstimator()
	// Before any authoritative calibration: default blended ratio 2.5 chars/token.
	if got := e.EstimateChars(100); got != 40 {
		t.Errorf("default estimate mismatch: got %d, want 40", got)
	}
	// After authoritative calibration: 250 tokens for 1000 chars = 4.0 chars/token.
	e.RecordAuthoritative(250, 1000)
	if got := e.EstimateChars(100); got != 25 {
		t.Errorf("authoritative estimate mismatch: got %d, want 25", got)
	}
	// Incremental estimate since last authoritative anchor.
	e.RecordIncremental(10)
	got := e.EstimateTotal()
	// total = 250 + 10/4 = 252 (or 253 with rounding).
	if got < 252 || got > 253 {
		t.Errorf("incremental estimate mismatch: got %d", got)
	}
}

func TestTokenEstimator_ZeroInputs(t *testing.T) {
	e := NewTokenEstimator()
	if got := e.EstimateChars(0); got != 0 {
		t.Errorf("expected 0 for 0 chars, got %d", got)
	}
	if got := e.EstimateChars(-5); got != 0 {
		t.Errorf("expected 0 for negative chars, got %d", got)
	}
	// Tiny non-zero input must round up to 1 token.
	if got := e.EstimateChars(1); got != 1 {
		t.Errorf("expected 1 for 1 char, got %d", got)
	}
	if got := e.EstimateTotal(); got != 0 {
		t.Errorf("expected 0 total without any records, got %d", got)
	}
}

func TestTokenEstimator_AuthoritativeResetsIncremental(t *testing.T) {
	e := NewTokenEstimator()
	e.RecordAuthoritative(100, 400)
	e.RecordIncremental(80)
	if got := e.EstimateTotal(); got != 120 { // 100 + 80/4
		t.Fatalf("expected 120, got %d", got)
	}
	// New authoritative anchor must reset incremental accumulation.
	e.RecordAuthoritative(200, 800)
	if got := e.EstimateTotal(); got != 200 {
		t.Fatalf("expected 200 after re-anchor, got %d", got)
	}
}

func TestSharedEstimatorCalibratedByAuthoritativeUsage(t *testing.T) {
	resetSharedEstimatorForTest()
	defer resetSharedEstimatorForTest()
	// 默认 2.5 chars/token
	if got := EstimateTokensFromChars(1000); got != 400 {
		t.Fatalf("default estimate = %d, want 400", got)
	}
	// 权威锚点：2000 chars 实测 1000 tokens → 2.0 chars/token
	RecordAuthoritativeUsage(1000, 2000)
	if got := EstimateTokensFromChars(1000); got != 500 {
		t.Fatalf("calibrated estimate = %d, want 500", got)
	}
}
