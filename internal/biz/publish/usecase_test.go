package publish

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aranea-agents/internal/biz/review"
)

func TestPublishBlockedUntilApproved(t *testing.T) {
	dir := t.TempDir()
	uc := NewUsecase(NewMemoryRepo(), dir)
	pending := review.Task{ID: "r1", Status: review.StatusPending, DraftTitle: "t", DraftBody: "b"}
	if _, err := uc.CreateFromApprovedReview(pending, "xiaohongshu"); err == nil {
		t.Fatal("expected block")
	}
	now := time.Now()
	approved := review.Task{
		ID: "r2", Status: review.StatusApproved, ContentItemID: "c1",
		DraftTitle: "Title", DraftBody: "Body", DecidedAt: &now,
	}
	job, err := uc.CreateFromApprovedReview(approved, "xiaohongshu")
	if err != nil {
		t.Fatal(err)
	}
	n, err := uc.ProcessNext(5)
	if err != nil || n != 1 {
		t.Fatalf("process n=%d err=%v", n, err)
	}
	got, err := uc.Get(job.ID)
	if err != nil || got.Status != StatusPackaged {
		t.Fatalf("status=%s err=%v", got.Status, err)
	}
	if _, err := os.Stat(filepath.Join(got.PackageDir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}
