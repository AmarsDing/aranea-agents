package trpc_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
)

// memArtifactRepo is an in-memory biz.ArtifactRepo for tests.
type memArtifactRepo struct {
	mu    sync.Mutex
	store map[string]*memEntry
}

type memEntry struct {
	meta biz.Artifact
	data []byte
}

func newMemArtifactRepo() *memArtifactRepo {
	return &memArtifactRepo{store: map[string]*memEntry{}}
}

func (r *memArtifactRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := fmt.Sprintf("%s-%s-%d", sessionID, name, len(r.store))
	art := biz.Artifact{
		ID: id, SessionID: sessionID, Name: name, MimeType: mimeType,
		Size: int64(len(data)), Version: r.countVersions(sessionID, name),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	r.store[id] = &memEntry{meta: art, data: cp}
	return art, nil
}

func (r *memArtifactRepo) countVersions(sessionID, name string) int {
	n := 0
	for _, e := range r.store {
		if e.meta.SessionID == sessionID && e.meta.Name == name {
			n++
		}
	}
	return n
}

func (r *memArtifactRepo) Load(_ context.Context, id string, _ int) (biz.Artifact, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.store[id]
	if !ok {
		return biz.Artifact{}, nil, fmt.Errorf("artifact not found: %s", id)
	}
	return e.meta, e.data, nil
}

func (r *memArtifactRepo) List(_ context.Context, sessionID string, _, _ int) ([]biz.Artifact, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []biz.Artifact
	seen := map[string]bool{}
	for _, e := range r.store {
		if e.meta.SessionID == sessionID && !seen[e.meta.Name] {
			seen[e.meta.Name] = true
			out = append(out, e.meta)
		}
	}
	return out, len(out), nil
}

func (r *memArtifactRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
	return nil
}

func (r *memArtifactRepo) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []biz.Artifact
	for _, e := range r.store {
		if e.meta.SessionID == sessionID && e.meta.Name == name {
			out = append(out, e.meta)
		}
	}
	return out, nil
}

func makeAdapter(t *testing.T) *artifacttrpc.ServiceAdapter {
	t.Helper()
	repo := newMemArtifactRepo()
	uc := biz.NewArtifactUsecase(repo)
	return artifacttrpc.NewServiceAdapter(uc)
}

func TestServiceAdapter_SaveAndLoad(t *testing.T) {
	svc := makeAdapter(t)
	ctx := context.Background()
	info := trpcartifact.SessionInfo{AppName: "test", UserID: "u1", SessionID: "sess1"}

	rev, err := svc.SaveArtifact(ctx, info, "hello.txt", &trpcartifact.Artifact{
		Data:     []byte("hello world"),
		MimeType: "text/plain",
	})
	if err != nil {
		t.Fatalf("SaveArtifact: %v", err)
	}
	if rev != 0 {
		t.Fatalf("expected revision 0, got %d", rev)
	}

	art, err := svc.LoadArtifact(ctx, info, "hello.txt", nil)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if art == nil {
		t.Fatal("expected artifact, got nil")
	}
	if string(art.Data) != "hello world" {
		t.Fatalf("data mismatch: %q", art.Data)
	}
}

func TestServiceAdapter_ListArtifactKeys(t *testing.T) {
	svc := makeAdapter(t)
	ctx := context.Background()
	info := trpcartifact.SessionInfo{SessionID: "sess2"}

	_, _ = svc.SaveArtifact(ctx, info, "a.bin", &trpcartifact.Artifact{Data: []byte("a")})
	_, _ = svc.SaveArtifact(ctx, info, "b.bin", &trpcartifact.Artifact{Data: []byte("b")})

	keys, err := svc.ListArtifactKeys(ctx, info)
	if err != nil {
		t.Fatalf("ListArtifactKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestServiceAdapter_DeleteArtifact(t *testing.T) {
	svc := makeAdapter(t)
	ctx := context.Background()
	info := trpcartifact.SessionInfo{SessionID: "sess3"}

	_, _ = svc.SaveArtifact(ctx, info, "del.txt", &trpcartifact.Artifact{Data: []byte("x")})
	if err := svc.DeleteArtifact(ctx, info, "del.txt"); err != nil {
		t.Fatalf("DeleteArtifact: %v", err)
	}
	keys, _ := svc.ListArtifactKeys(ctx, info)
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestServiceAdapter_ListVersions(t *testing.T) {
	svc := makeAdapter(t)
	ctx := context.Background()
	info := trpcartifact.SessionInfo{SessionID: "sess4"}

	_, _ = svc.SaveArtifact(ctx, info, "file.bin", &trpcartifact.Artifact{Data: []byte("v0")})
	_, _ = svc.SaveArtifact(ctx, info, "file.bin", &trpcartifact.Artifact{Data: []byte("v1")})

	versions, err := svc.ListVersions(ctx, info, "file.bin")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
}
