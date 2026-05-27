package agent_test

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/agent"
	artifactbiz "aranea-agents/internal/biz/artifact"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type memArtifactRepo struct {
	items map[string]artifactbiz.Artifact
	data  map[string][]byte
}

func (m *memArtifactRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (artifactbiz.Artifact, error) {
	id := artifactbiz.NewArtifactID()
	a := artifactbiz.Artifact{
		ID: id, SessionID: sessionID, Name: name, MimeType: mimeType, Size: int64(len(data)),
	}
	m.items[id] = a
	m.data[id] = data
	return a, nil
}

func (m *memArtifactRepo) Load(_ context.Context, id string, _ int) (artifactbiz.Artifact, []byte, error) {
	a, ok := m.items[id]
	if !ok {
		return artifactbiz.Artifact{}, nil, fmt.Errorf("not found")
	}
	return a, m.data[id], nil
}

func (m *memArtifactRepo) List(context.Context, string, int, int) ([]artifactbiz.Artifact, int, error) {
	return nil, 0, nil
}

func (m *memArtifactRepo) Delete(context.Context, string) error { return nil }

func (m *memArtifactRepo) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	return nil
}

func (m *memArtifactRepo) ListBySessionAndName(context.Context, string, string) ([]artifactbiz.Artifact, error) {
	return nil, nil
}

func TestBuildUserMessageFromAttachments_image(t *testing.T) {
	repo := &memArtifactRepo{items: map[string]artifactbiz.Artifact{}, data: map[string][]byte{}}
	uc := artifactbiz.NewUsecase(repo)
	ctx := context.Background()
	saved, err := uc.Save(ctx, "sess-1", "pic.png", "image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, "sess-1", "see this", []string{saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != trpcmodel.RoleUser {
		t.Fatalf("role=%v", msg.Role)
	}
	if len(msg.ContentParts) != 2 {
		t.Fatalf("parts=%d", len(msg.ContentParts))
	}
	if msg.ContentParts[1].Type != trpcmodel.ContentTypeImage {
		t.Fatalf("part1 type=%v", msg.ContentParts[1].Type)
	}
}
