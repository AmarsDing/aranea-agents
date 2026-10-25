package data

import "testing"

// TestEvalRelationalHop2Budget pins the AMC P3-d deterministic bound
// (限深 2、链接展开总量上界 30): the second relational hop gets exactly what
// the first hop left of the shared id budget, never negative.
func TestEvalRelationalHop2Budget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		used int
		want int
	}{
		{0, 30},   // no hop-1 expansion: hop 2 may use the full budget
		{1, 29},   // typical: hop 1 found one linked fact
		{24, 6},   // hop 1 saturated its per-hop cap
		{30, 0},   // budget exhausted: hop 2 skipped
		{42, 0},   // over budget (defensive): clamp to zero, no negative cap
	}
	if evalRelationalHopIDBudget != 30 {
		t.Fatalf("relational link budget drifted: %d, want 30", evalRelationalHopIDBudget)
	}
	for _, tc := range cases {
		if got := evalRelationalHop2Budget(tc.used); got != tc.want {
			t.Errorf("evalRelationalHop2Budget(%d)=%d, want %d", tc.used, got, tc.want)
		}
	}
}
