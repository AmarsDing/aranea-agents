package llmcontext

import "testing"

func TestContextRatio(t *testing.T) {
	if got := ContextRatio(64_000, 128_000); got != 0.5 {
		t.Fatalf("got %v want 0.5", got)
	}
	if got := ContextRatio(200_000, 128_000); got != 1 {
		t.Fatalf("cap: got %v want 1", got)
	}
	if got := ContextRatio(0, 128_000); got != 0 {
		t.Fatalf("zero prompt: got %v", got)
	}
}

func TestContextStatusForRatio(t *testing.T) {
	cases := []struct {
		ratio float64
		want  string
	}{
		{0.5, "normal"},
		{0.65, "warning"},
		{0.85, "critical"},
		{0.96, "exceeded"},
	}
	for _, tc := range cases {
		if got := ContextStatusForRatio(tc.ratio); got != tc.want {
			t.Fatalf("ratio %v: got %q want %q", tc.ratio, got, tc.want)
		}
	}
}
