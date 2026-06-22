package artifact_test

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"
)

type resolveMemRepo struct {
	items map[string]artifact.Artifact
	data  map[string][]byte
}

func (m *resolveMemRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (artifact.Artifact, error) {
	id := artifact.NewArtifactID()
	a := artifact.Artifact{
		ID: id, SessionID: sessionID, Name: name, MimeType: mimeType, Size: int64(len(data)),
	}
	m.items[id] = a
	m.data[id] = data
	return a, nil
}

func (m *resolveMemRepo) Load(_ context.Context, id string, _ int) (artifact.Artifact, []byte, error) {
	a, ok := m.items[id]
	if !ok {
		return artifact.Artifact{}, nil, fmt.Errorf("not found")
	}
	return a, m.data[id], nil
}

func (m *resolveMemRepo) LoadMeta(_ context.Context, id string, _ int) (artifact.Artifact, error) {
	a, ok := m.items[id]
	if !ok {
		return artifact.Artifact{}, fmt.Errorf("not found")
	}
	return a, nil
}

func (m *resolveMemRepo) LoadMetas(_ context.Context, ids []string, _ int) ([]artifact.Artifact, error) {
	out := make([]artifact.Artifact, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.items[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *resolveMemRepo) List(context.Context, string, int, int) ([]artifact.Artifact, int, error) {
	return nil, 0, nil
}

func (m *resolveMemRepo) Delete(context.Context, string) error { return nil }
func (m *resolveMemRepo) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	return nil
}

func (m *resolveMemRepo) ListBySessionAndName(context.Context, string, string) ([]artifact.Artifact, error) {
	return nil, nil
}

func TestResolveAttachmentRefs_ok(t *testing.T) {
	repo := &resolveMemRepo{items: map[string]artifact.Artifact{}, data: map[string][]byte{}}
	uc := artifact.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	saved, err := uc.Save(ctx, "sess-1", "a.png", "image/png", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := artifact.ResolveAttachmentRefs(ctx, uc, "sess-1", []string{saved.ID, "  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != saved.ID {
		t.Fatalf("refs=%+v", refs)
	}
}

func TestResolveAttachmentRefs_sessionMismatch(t *testing.T) {
	repo := &resolveMemRepo{items: map[string]artifact.Artifact{}, data: map[string][]byte{}}
	uc := artifact.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	saved, _ := uc.Save(ctx, "sess-a", "a.png", "image/png", []byte{1})
	_, err := artifact.ResolveAttachmentRefs(ctx, uc, "sess-b", []string{saved.ID})
	if err == nil {
		t.Fatal("expected session mismatch error")
	}
}

func TestResolveAttachmentRefs_missing(t *testing.T) {
	uc := artifact.NewUsecase(&resolveMemRepo{items: map[string]artifact.Artifact{}, data: map[string][]byte{}}, loggateway.NewNoop())
	_, err := artifact.ResolveAttachmentRefs(context.Background(), uc, "sess-1", []string{"missing-id"})
	if err == nil {
		t.Fatal("expected not found error")
	}
}
