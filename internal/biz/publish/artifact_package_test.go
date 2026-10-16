package publish_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/pkg/loggateway"
)

func TestProcessNext_WritesArtifactViaSaver(t *testing.T) {
	root := t.TempDir()
	artRoot := filepath.Join(root, "arts")
	_ = os.MkdirAll(artRoot, 0o755)
	repo := publish.NewMemoryRepo()
	uc := publish.NewUsecase(repo, filepath.Join(root, "packages"))
	saver := artifactfs.NewFSArtifactRepoAt(artRoot, loggateway.NewNoop())
	uc.SetArtifactSaver(saver)

	task := review.Task{ID: "rt-1", Status: review.StatusApproved, DraftTitle: "Hello", DraftBody: "World", ContentItemID: "c1"}
	job, err := uc.CreateFromApprovedReview(task, "generic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.ProcessNext(1); err != nil {
		t.Fatal(err)
	}
	got, err := uc.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != publish.StatusPackaged {
		t.Fatalf("status=%s", got.Status)
	}
	if got.ArtifactID == "" || got.ArtifactID == got.ID {
		t.Fatalf("expected real artifact id, got %q", got.ArtifactID)
	}
	meta, err := saver.LoadMeta(context.Background(), got.ArtifactID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "manifest.json" {
		t.Fatalf("artifact name=%q", meta.Name)
	}
}
