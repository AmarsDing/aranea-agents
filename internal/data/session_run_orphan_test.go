package data

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestCountOrphanSessions(t *testing.T) {
	got := countOrphanSessions([]string{"s1", "s1", " s2 ", "", "s2"})
	if got["s1"] != 2 || got["s2"] != 2 {
		t.Fatalf("counts = %v, want s1=2 s2=2", got)
	}
	if _, ok := got[""]; ok {
		t.Fatal("empty session_id must not be counted")
	}
}

func TestOrphanHonestyContract_FIT_OBS_1(t *testing.T) {
	counts := countOrphanSessions([]string{"sess-a", "sess-a"})
	finishedAt := "2026-08-31T00:00:00Z"
	for _, n := range counts {
		if err := biz.CheckRunHonesty("orphaned", n, finishedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := biz.CheckRunHonesty("orphaned", 0, finishedAt); err == nil {
		t.Fatal("zero error_count delta must fail FIT-OBS-1")
	}
}
