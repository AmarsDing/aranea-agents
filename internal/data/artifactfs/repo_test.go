package artifactfs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/data/artifactfs"
	"aranea-agents/pkg/loggateway"
)

func TestFSArtifactRepo_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
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
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
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
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
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

func TestFSArtifactRepo_ListAllSessions(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	ctx := context.Background()

	_, _ = repo.Save(ctx, "sess-a", "a.txt", "text/plain", []byte("a"))
	_, _ = repo.Save(ctx, "sess-b", "b.txt", "text/plain", []byte("b"))

	items, total, err := repo.List(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 items across sessions, got total=%d len=%d", total, len(items))
	}
}

func TestFSArtifactRepo_StorageBytes(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	ctx := context.Background()

	_, _ = repo.Save(ctx, "sess1", "a.bin", "application/octet-stream", []byte("12345"))
	n, err := repo.StorageBytes(ctx)
	if err != nil {
		t.Fatalf("StorageBytes: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes, got %d", n)
	}
}

func TestFSArtifactRepo_Delete(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
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

// OUT-05 / ART-01: Save must reject session IDs that could traverse out of the
// configured artifact root. Without this guard, an attacker controlling the
// session ID can write/read files anywhere the process has permission to.
func TestFSArtifactRepo_RejectsTraversalSessionID(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	ctx := context.Background()

	bad := []string{
		"../escape",
		"..",
		"a/b",
		"a\\b",
		"foo/../bar",
		"",
		"   ",
		string([]byte{0}),
	}
	for _, sid := range bad {
		if _, err := repo.Save(ctx, sid, "x.txt", "text/plain", []byte("x")); err == nil {
			t.Errorf("expected Save to reject session_id %q, got nil error", sid)
		}
	}
}

// OUT-05 / ART-03: persisted StorageURI must be relative to the artifact root so
// API responses (and meta sidecar dumps) never disclose absolute filesystem
// layout. The repo must still be able to Load the artifact by joining root +
// relative URI internally.
func TestFSArtifactRepo_StorageURIIsRelative(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	ctx := context.Background()

	saved, err := repo.Save(ctx, "sess-rel", "foo.txt", "text/plain", []byte("hi"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	uri := saved.StorageURI
	if uri == "" {
		t.Fatal("expected non-empty StorageURI")
	}
	if filepath.IsAbs(uri) {
		t.Fatalf("expected relative StorageURI, got absolute %q", uri)
	}
	if !strings.HasPrefix(uri, "sess-rel/") {
		t.Fatalf("expected URI prefixed with session id, got %q", uri)
	}

	// Round-trip: Load must still find the artifact when StorageURI is relative.
	_, data, err := repo.Load(ctx, saved.ID, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(data) != "hi" {
		t.Fatalf("payload mismatch: %q", data)
	}
}

func TestFSArtifactRepo_LegacyStorageURIWithRoot(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	ctx := context.Background()

	saved, err := repo.Save(ctx, "sess-legacy", "bar.txt", "text/plain", []byte("legacy"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	sessionDir := filepath.Join(dir, "sess-legacy")
	metaPath := filepath.Join(sessionDir, fmt.Sprintf("%s-v%d.json", saved.ID, saved.Version))
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}

	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	meta["storage_uri"] = filepath.Join(dir, "sess-legacy", fmt.Sprintf("%s-v%d.bin", saved.ID, saved.Version))
	updated, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, updated, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	_, data, err := repo.Load(ctx, saved.ID, 0)
	if err != nil {
		t.Fatalf("Load with absolute StorageURI: %v", err)
	}
	if string(data) != "legacy" {
		t.Fatalf("payload mismatch: %q", data)
	}

	rootPrefixedURI := filepath.Join(dir, "sess-legacy", fmt.Sprintf("%s-v%d.bin", saved.ID, saved.Version))
	meta["storage_uri"] = filepath.ToSlash(rootPrefixedURI)
	updated2, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, updated2, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	_, data2, err := repo.Load(ctx, saved.ID, 0)
	if err != nil {
		t.Fatalf("Load with root-prefixed StorageURI %q: %v", rootPrefixedURI, err)
	}
	if string(data2) != "legacy" {
		t.Fatalf("payload mismatch: %q", data2)
	}
}

func TestFSArtifactRepo_ResolveBinPath_RejectsAbsOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	repo := artifactfs.NewFSArtifactRepoAt(dir, loggateway.NewNoop())
	outside := filepath.Join(t.TempDir(), "evil.bin")
	if err := os.WriteFile(outside, []byte("pwned"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	got := artifactfs.ResolveBinPath(repo, artifactfs.ArtifactMeta{
		ID: "art-1", SessionID: "sess1", Version: 0, StorageURI: outside,
	})
	gotAbs, _ := filepath.Abs(got)
	outsideAbs, _ := filepath.Abs(outside)
	if gotAbs == outsideAbs {
		t.Fatalf("must not return absolute path outside root: %q", got)
	}
	rootAbs, _ := filepath.Abs(dir)
	if !strings.HasPrefix(strings.ToLower(gotAbs), strings.ToLower(rootAbs)) {
		t.Fatalf("fallback path %q must stay under root %q", gotAbs, rootAbs)
	}
}
