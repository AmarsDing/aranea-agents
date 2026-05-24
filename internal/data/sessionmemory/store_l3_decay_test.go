package sessionmemory

import (
	"context"
	"testing"
	"time"
)

func TestApplyAllFactImportanceDecay(t *testing.T) {
	store := openL3RecallTestStore(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	n, err := store.ApplyAllFactImportanceDecay(ctx, cutoff, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 old fact decayed, got %d", n)
	}
}
