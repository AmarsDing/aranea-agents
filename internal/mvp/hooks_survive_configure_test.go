package mvp_test

import (
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/mvp"
)

func TestConfigurePreservesGraphHooks(t *testing.T) {
	mvp.ResetForTest()
	var resumed string
	mvp.ConfigureGraphHooks(func(executionID string, resumeValue map[string]any) error {
		resumed = executionID
		return nil
	}, nil)
	mvp.Configure(mvp.Deps{})
	rt := mvp.Ensure()
	if rt.GraphResume == nil {
		t.Fatal("GraphResume wiped by Configure")
	}
	if err := rt.GraphResume("exec-keep", nil); err != nil {
		t.Fatal(err)
	}
	if resumed != "exec-keep" {
		t.Fatalf("resume=%q", resumed)
	}
}

func TestReviewNotifySurvivesConfigure(t *testing.T) {
	mvp.ResetForTest()
	var got string
	mvp.ConfigureReviewNotify(func(task review.Task) {
		got = task.ID
	})
	mvp.Configure(mvp.Deps{})
	task, err := mvp.Ensure().Review.Enqueue(review.Task{
		ContentItemID: "c1",
		DraftTitle:    "t",
		DraftBody:     "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != task.ID {
		t.Fatalf("notify not wired after Configure: got=%q want=%q", got, task.ID)
	}
}

func TestPackageFilePathAllowlist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	job := publish.Job{PackageDir: dir, Status: publish.StatusPackaged}
	path, err := mvp.PackageFilePath(job, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "manifest.json" {
		t.Fatalf("path=%s", path)
	}
	if _, err := mvp.PackageFilePath(job, "../etc/passwd"); err == nil {
		t.Fatal("expected reject traversal")
	}
	if _, err := mvp.PackageFilePath(job, "secret.bin"); err == nil {
		t.Fatal("expected reject non-allowlisted")
	}
}
