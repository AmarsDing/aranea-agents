package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"

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

func (m *memArtifactRepo) LoadMeta(_ context.Context, id string, _ int) (artifactbiz.Artifact, error) {
	a, ok := m.items[id]
	if !ok {
		return artifactbiz.Artifact{}, fmt.Errorf("not found")
	}
	return a, nil
}

func (m *memArtifactRepo) LoadMetas(_ context.Context, ids []string, _ int) ([]artifactbiz.Artifact, error) {
	out := make([]artifactbiz.Artifact, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.items[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
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
	uc := artifactbiz.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	saved, err := uc.Save(ctx, "sess-1", "pic.png", "image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, nil, "sess-1", "see this", []string{saved.ID})
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

// ── 单轮超限治理 A：巨型文本附件落地 blob ─────────────────────

type stubUserInputGate struct {
	calls   []stubGateCall
	preview string
	persist bool
	err     error
}

type stubGateCall struct {
	sessionID string
	messageID string
	source    string
	size      int
}

func (s *stubUserInputGate) CheckUserInput(_ context.Context, sessionID, messageID, source, fullContent string) (biz.ToolResultGateResult, error) {
	if s.err != nil {
		return biz.ToolResultGateResult{}, s.err
	}
	s.calls = append(s.calls, stubGateCall{sessionID: sessionID, messageID: messageID, source: source, size: len(fullContent)})
	if !s.persist {
		return biz.ToolResultGateResult{}, nil
	}
	return biz.ToolResultGateResult{BlobID: "blob-x", PreviewText: s.preview, DidPersist: true}, nil
}

func TestBuildUserMessageFromAttachments_LargeTextGated(t *testing.T) {
	repo := &memArtifactRepo{items: map[string]artifactbiz.Artifact{}, data: map[string][]byte{}}
	uc := artifactbiz.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	large := strings.Repeat("y", biz.ToolResultSizeThreshold+10)
	saved, err := uc.Save(ctx, "sess-1", "big.txt", "text/plain", []byte(large))
	if err != nil {
		t.Fatal(err)
	}
	gate := &stubUserInputGate{persist: true, preview: "头部预览... [truncated blob_id=blob-x]"}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, gate, "sess-1", "see this", []string{saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.calls) != 1 {
		t.Fatalf("gate calls = %d, want 1", len(gate.calls))
	}
	call := gate.calls[0]
	if call.source != biz.ToolResultSourceAttachment {
		t.Fatalf("source = %q, want %q", call.source, biz.ToolResultSourceAttachment)
	}
	if call.messageID != saved.ID {
		t.Fatalf("messageID = %q, want artifact id %q", call.messageID, saved.ID)
	}
	if call.sessionID != "sess-1" {
		t.Fatalf("sessionID = %q, want sess-1", call.sessionID)
	}
	// 超限附件应替换为文本 preview part，而非 File part
	var found bool
	for _, p := range msg.ContentParts {
		if p.Type == trpcmodel.ContentTypeText && p.Text != nil && strings.Contains(*p.Text, "truncated") {
			found = true
		}
		if p.Type == trpcmodel.ContentTypeFile {
			t.Fatal("gated attachment should not remain a File part")
		}
	}
	if !found {
		t.Fatal("expected a text preview part containing truncation marker")
	}
}

func TestBuildUserMessageFromAttachments_SmallTextNotGated(t *testing.T) {
	repo := &memArtifactRepo{items: map[string]artifactbiz.Artifact{}, data: map[string][]byte{}}
	uc := artifactbiz.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	saved, err := uc.Save(ctx, "sess-1", "small.txt", "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	gate := &stubUserInputGate{persist: true, preview: "should-not-be-used"}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, gate, "sess-1", "", []string{saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.calls) != 0 {
		t.Fatalf("gate should not be called for small attachment, calls = %d", len(gate.calls))
	}
	if msg.ContentParts[0].Type != trpcmodel.ContentTypeFile {
		t.Fatalf("part0 type=%v, want File", msg.ContentParts[0].Type)
	}
}

func TestBuildUserMessageFromAttachments_NilGateKeepsFile(t *testing.T) {
	repo := &memArtifactRepo{items: map[string]artifactbiz.Artifact{}, data: map[string][]byte{}}
	uc := artifactbiz.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	large := strings.Repeat("y", biz.ToolResultSizeThreshold+10)
	saved, err := uc.Save(ctx, "sess-1", "big.txt", "text/plain", []byte(large))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, nil, "sess-1", "", []string{saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if msg.ContentParts[0].Type != trpcmodel.ContentTypeFile {
		t.Fatalf("part0 type=%v, want File (nil gate keeps current behavior)", msg.ContentParts[0].Type)
	}
}

func TestBuildUserMessageFromAttachments_ImageNotGated(t *testing.T) {
	repo := &memArtifactRepo{items: map[string]artifactbiz.Artifact{}, data: map[string][]byte{}}
	uc := artifactbiz.NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()
	saved, err := uc.Save(ctx, "sess-1", "pic.png", "image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	gate := &stubUserInputGate{persist: true, preview: "should-not-be-used"}
	msg, err := agent.BuildUserMessageFromArtifacts(ctx, uc, gate, "sess-1", "", []string{saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(gate.calls) != 0 {
		t.Fatalf("gate should not be called for image attachment, calls = %d", len(gate.calls))
	}
	if msg.ContentParts[0].Type != trpcmodel.ContentTypeImage {
		t.Fatalf("part0 type=%v, want Image", msg.ContentParts[0].Type)
	}
}
