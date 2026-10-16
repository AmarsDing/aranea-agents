package mvp_test

import (
	"testing"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/mvp"
)

func TestSmokeReviewToPackage(t *testing.T) {
	mvp.ResetForTest()
	rt := mvp.Ensure()
	item, _, err := rt.Content.CreateFromIngest(content.Item{
		WorkspaceID: "ws", Title: "绝对爆款标题", Body: "正文", SourceType: content.SourceManual,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := rt.Review.Enqueue(review.Task{
		ContentItemID: item.ID, DraftTitle: item.Title, DraftBody: item.Body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.MachineFlags) == 0 {
		t.Fatal("expected soft flags")
	}
	approved, err := rt.Review.Approve(task.ID, "ops", "lgtm")
	if err != nil {
		t.Fatal(err)
	}
	job, err := rt.Publish.CreateFromApprovedReview(approved, "generic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Publish.ProcessNext(1); err != nil {
		t.Fatal(err)
	}
	got, err := rt.Publish.Get(job.ID)
	if err != nil || got.Status != "packaged" {
		t.Fatalf("status=%s err=%v", got.Status, err)
	}
	if got.PackageDir == "" {
		t.Fatal("expected package_dir set")
	}
}

func TestSmokeReviewApproveCreatesPackageViaHTTPFlow(t *testing.T) {
	mvp.ResetForTest()
	rt := mvp.Ensure()
	item, _, err := rt.Content.CreateFromIngest(content.Item{
		WorkspaceID: "ws", Title: "pack me", Body: "body", SourceType: content.SourceManual,
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := rt.Review.Enqueue(review.Task{
		ContentItemID: item.ID, DraftTitle: item.Title, DraftBody: item.Body,
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := rt.Review.Approve(task.ID, "ops", "ok")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rt.Content.MarkApproved(item.ID)
	job, err := rt.Publish.CreateFromApprovedReview(approved, "generic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Publish.ProcessNext(1); err != nil {
		t.Fatal(err)
	}
	got, _ := rt.Publish.Get(job.ID)
	if got.Status != "packaged" {
		t.Fatalf("want packaged got %s", got.Status)
	}
}

