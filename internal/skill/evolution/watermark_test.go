package evolution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestFileWatermarkStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileWatermarkStore(dir, loggateway.NewNoop())
	ts := time.Date(2026, 10, 15, 12, 0, 0, 123456789, time.UTC)
	store.Set("sess-1", ts)

	store2 := NewFileWatermarkStore(dir, loggateway.NewNoop())
	got, ok := store2.Get("sess-1")
	if !ok {
		t.Fatal("expected watermark after restart")
	}
	if !got.Equal(ts) {
		t.Fatalf("got %v want %v", got, ts)
	}
}

func TestRestoreWatermark_InjectsSessionState(t *testing.T) {
	store := NewMemoryWatermarkStore()
	ts := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	store.Set("s1", ts)
	sess := &trpcsession.Session{ID: "s1"}
	restoreWatermark(store, sess)
	got, ok := readSessionWatermark(sess)
	if !ok || !got.Equal(ts) {
		t.Fatalf("session state watermark got %v ok=%v", got, ok)
	}
}

func TestPersistingReviewPolicy_DeclineWritesStore(t *testing.T) {
	store := NewMemoryWatermarkStore()
	ts := time.Now().UTC().Truncate(time.Millisecond)
	p := &persistingReviewPolicy{
		inner: trpcevolution.DefaultReviewPolicy{MinToolCalls: 100},
		store: store,
		lg:    loggateway.NewNoop(),
	}
	ok, err := p.ShouldReview(context.Background(), &trpcevolution.ReviewPolicyInput{
		SessionID: "s-decline",
		ReviewContext: &trpcevolution.ReviewContext{
			LatestTs:      ts,
			ToolCallCount: 1,
			Messages: []trpcmodel.Message{
				{Role: trpcmodel.RoleUser, Content: "hi"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected policy decline (below tool threshold)")
	}
	got, ok := store.Get("s-decline")
	if !ok || !got.Equal(ts) {
		t.Fatalf("store watermark got %v ok=%v want %v", got, ok, ts)
	}
}

func TestTwoTurnRestore_SecondStartsFromWatermark(t *testing.T) {
	store := NewMemoryWatermarkStore()
	t1 := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 15, 11, 0, 0, 0, time.UTC)
	store.Set("sess", t1)

	sess := &trpcsession.Session{ID: "sess"}
	restoreWatermark(store, sess)
	got, ok := readSessionWatermark(sess)
	if !ok || !got.Equal(t1) {
		t.Fatalf("turn2 restore got %v", got)
	}
	// Simulate worker write + our persist.
	sess.SetState(sessionStateKeyLastReviewAt, []byte(t2.Format(time.RFC3339Nano)))
	if raw, ok := sess.GetState(sessionStateKeyLastReviewAt); ok {
		if ts, err := time.Parse(time.RFC3339Nano, string(raw)); err == nil {
			store.Set("sess", ts)
		}
	}
	got2, ok := store.Get("sess")
	if !ok || !got2.Equal(t2) {
		t.Fatalf("after turn2 persist got %v", got2)
	}
}

func TestWatermarkPersistEnabled_DefaultOn(t *testing.T) {
	t.Setenv("EVOLUTION_WATERMARK_PERSIST", "")
	if !watermarkPersistEnabled() {
		t.Fatal("default should be on")
	}
	t.Setenv("EVOLUTION_WATERMARK_PERSIST", "0")
	if watermarkPersistEnabled() {
		t.Fatal("0 should disable")
	}
}

func TestFileWatermarkStore_SafeFilename(t *testing.T) {
	dir := t.TempDir()
	store := NewFileWatermarkStore(dir, loggateway.NewNoop())
	store.Set("sess/../evil", time.Now().UTC())
	entries, err := os.ReadDir(filepath.Join(dir, "watermarks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
}
