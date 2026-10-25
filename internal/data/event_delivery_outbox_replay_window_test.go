package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// E2E-C2: with a stale reconnect cursor and a large gap, replay must return
// the NEWEST `limit` rows after the cursor (ascending on the wire), not the
// oldest 100 — ancient frames would otherwise crowd the truly-missed latest
// critical frames (e.g. team_summary, which REST never carries) out of the
// replay window permanently.

func TestEventDeliveryOutbox_ListAfterReplaysNewestWindow(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)

	now := time.Now().UTC()
	// 150 events; cursor at seq=0 (or a far-behind establishment-time cursor).
	for i := 1; i <= 150; i++ {
		row := biz.EventDeliveryOutboxRow{
			ID:        fmt.Sprintf("row-%d", i),
			SessionID: "sess-1",
			Seq:       int64(i),
			EventID:   fmt.Sprintf("v2:sess-1:%d:task.completed:t%d", i, i),
			Kind:      "task.completed",
			EntityID:  fmt.Sprintf("t%d", i),
			Payload:   []byte(`{"type":"v2_event"}`),
			CreatedAt: now,
		}
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("Insert(%d): %v", i, err)
		}
	}

	got, err := repo.ListAfter(ctx, "sess-1", "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 {
		t.Fatalf("replay window size = %d, want 100", len(got))
	}
	// Newest 100 after the cursor: seq 51..150, ascending on the wire.
	if got[0].Seq != 51 {
		t.Fatalf("window head seq = %d, want 51 (newest-after-cursor window, not oldest)", got[0].Seq)
	}
	if got[99].Seq != 150 {
		t.Fatalf("window tail seq = %d, want 150", got[99].Seq)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("wire order must stay ascending: seq[%d]=%d after %d", i, got[i].Seq, got[i-1].Seq)
		}
	}

	// Small gap (< limit) behaves identically to before: everything after the cursor.
	small, err := repo.ListAfter(ctx, "sess-1", "v2:sess-1:148:task.completed:t148", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(small) != 2 || small[0].Seq != 149 || small[1].Seq != 150 {
		t.Fatalf("small-gap replay = %+v, want seq 149,150", small)
	}
}
