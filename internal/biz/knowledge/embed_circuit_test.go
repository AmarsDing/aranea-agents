package knowledge

import (
	"testing"
	"time"
)

// SP2 #9：embedding 熔断退避策略（SiYuan block_embeddings fail_count 同源）。
// 退避表：base 1min，按 failCount 指数左移，fc≥7 封顶 64min。
func TestEmbedCircuitAllow(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		failCount int
		lastTried time.Time
		want      bool
	}{
		{"never failed allows", 0, time.Time{}, true},
		{"fc1 within 1min denied", 1, now.Add(-30 * time.Second), false},
		{"fc1 elapsed allows", 1, now.Add(-61 * time.Second), true},
		{"fc3 within 4min denied", 3, now.Add(-3 * time.Minute), false},
		{"fc3 elapsed allows", 3, now.Add(-5 * time.Minute), true},
		{"fc7 within 64min cap denied", 7, now.Add(-60 * time.Minute), false},
		{"fc7 cap elapsed allows", 7, now.Add(-65 * time.Minute), true},
		{"fc20 capped at 64min within", 20, now.Add(-63 * time.Minute), false},
		{"fc20 capped at 64min elapsed", 20, now.Add(-65 * time.Minute), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EmbedCircuitAllow(c.failCount, c.lastTried, now); got != c.want {
				t.Errorf("EmbedCircuitAllow(%d, %v ago) = %v, want %v",
					c.failCount, now.Sub(c.lastTried), got, c.want)
			}
		})
	}
}
