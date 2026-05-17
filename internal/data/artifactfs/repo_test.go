package artifactfs_test

import (
	"context"
	"testing"

	"aranea-agents/internal/data/artifactfs"
)

func TestFSArtifactRepo_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir)
	ctx := context.Background()

	payload := []byte("hello artifact")
	saved, err := repo.Save(ctx, "sess1", "test.txt", "text/plain", payload)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if saved.Version != 0 {
		t.Fatalf("expected version 0, got %d", saved.Version)
	}
	if saved.Size != int64(len(payload)) {
		t.Fatalf("size mismatch: want %d got %d", len(payload), saved.Size)
	}

	meta, data, err := repo.Load(ctx, saved.ID, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("payload mismatch: want %q got %q", payload, data)
	}
	if meta.SHA256 != saved.SHA256 {
		t.Fatalf("sha256 mismatch")
	}
}

func TestFSArtifactRepo_MultiVersion(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir)
	ctx := context.Background()

	_, _ = repo.Save(ctx, "sess1", "file.bin", "application/octet-stream", []byte("v0"))
	_, _ = repo.Save(ctx, "sess1", "file.bin", "application/octet-stream", []byte("v1"))

	versions, err := repo.ListBySessionAndName(ctx, "sess1", "file.bin")
	if err != nil {
		t.Fatalf("ListBySessionAndName: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != 0 || versions[1].Version != 1 {
		t.Fatalf("versions not in order")
	}
}

func TestFSArtifactRepo_List(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir)
	ctx := context.Background()

	_, _ = repo.Save(ctx, "sess2", "a.txt", "text/plain", []byte("a"))
	_, _ = repo.Save(ctx, "sess2", "b.txt", "text/plain", []byte("b"))

	items, total, err := repo.List(ctx, "sess2", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 items, got %d total %d items", total, len(items))
	}
}

func TestFSArtifactRepo_Delete(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir)
	ctx := context.Background()

	saved, _ := repo.Save(ctx, "sess3", "del.txt", "text/plain", []byte("bye"))
	if err := repo.Delete(ctx, saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, total, _ := repo.List(ctx, "sess3", 10, 0)
	if total != 0 {
		t.Fatalf("expected empty list after delete, got %d", total)
	}
}
