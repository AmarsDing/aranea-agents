package sessionmemory

import "testing"

func TestRelationValidAt_OpenEnded(t *testing.T) {
	now := "2026-05-24T12:00:00Z"
	if !relationValidAt("2026-05-01T00:00:00Z", "", now, now) {
		t.Fatal("expected valid open-ended relation")
	}
}

func TestRelationValidAt_Expired(t *testing.T) {
	q := "2026-05-24T12:00:00Z"
	if relationValidAt("2026-05-01T00:00:00Z", "2026-05-20T00:00:00Z", q, q) {
		t.Fatal("expected expired relation")
	}
}

func TestRelationValidAt_FutureStart(t *testing.T) {
	q := "2026-05-24T12:00:00Z"
	if relationValidAt("2026-06-01T00:00:00Z", "", q, q) {
		t.Fatal("expected not-yet-valid relation")
	}
}
