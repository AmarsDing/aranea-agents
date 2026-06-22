package service_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/artifact"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// memArtifactRepo is an in-memory ArtifactRepo for service tests.
type memArtifactRepo struct {
	items map[string]biz.Artifact
	data  map[string][]byte
}

func newMemArtifactRepo() *memArtifactRepo {
	return &memArtifactRepo{items: make(map[string]biz.Artifact), data: make(map[string][]byte)}
}

func (m *memArtifactRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	id := biz.NewArtifactID()
	a := biz.Artifact{
		ID: id, SessionID: sessionID, Name: name, MimeType: mimeType,
		Size: int64(len(data)), Version: 1,
	}
	m.items[id] = a
	m.data[id] = data
	return a, nil
}

func (m *memArtifactRepo) Load(_ context.Context, id string, _ int) (biz.Artifact, []byte, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, nil, fmt.Errorf("artifact not found: %s", id)
	}
	return a, m.data[id], nil
}

func (m *memArtifactRepo) LoadMeta(_ context.Context, id string, _ int) (biz.Artifact, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, fmt.Errorf("artifact not found: %s", id)
	}
	return a, nil
}

func (m *memArtifactRepo) LoadMetas(_ context.Context, ids []string, _ int) ([]biz.Artifact, error) {
	out := make([]biz.Artifact, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.items[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memArtifactRepo) List(_ context.Context, sessionID string, limit, _ int) ([]biz.Artifact, int, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		if a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, len(out), nil
}

func (m *memArtifactRepo) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	delete(m.data, id)
	return nil
}

func (m *memArtifactRepo) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	return nil
}

func (m *memArtifactRepo) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		if a.SessionID == sessionID && a.Name == name {
			out = append(out, a)
		}
	}
	return out, nil
}

func newArtifactService() *service.ArtifactService {
	repo := newMemArtifactRepo()
	uc := biz.NewArtifactUsecase(repo, loggateway.NewNoop())
	signer := artifact.NewSigner(loggateway.NewNoop())
	return service.NewArtifactService(uc, signer)
}

func TestArtifactService_Upload_Get_Delete(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()
	payload := []byte("hello artifact")
	encoded := base64.StdEncoding.EncodeToString(payload)

	meta, err := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId:  "sess-1",
		Name:       "hello.txt",
		MimeType:   "text/plain",
		DataBase64: encoded,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if meta.GetName() != "hello.txt" {
		t.Errorf("name mismatch: %s", meta.GetName())
	}
	if meta.GetSize() != int64(len(payload)) {
		t.Errorf("size mismatch: %d", meta.GetSize())
	}

	got, err := svc.GetArtifact(ctx, &v1.GetArtifactRequest{Id: meta.GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	gotData, _ := base64.StdEncoding.DecodeString(got.GetDataBase64())
	if string(gotData) != string(payload) {
		t.Errorf("payload mismatch: %q", gotData)
	}

	_, err = svc.DeleteArtifact(ctx, &v1.DeleteArtifactRequest{Id: meta.GetId()})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.GetArtifact(ctx, &v1.GetArtifactRequest{Id: meta.GetId()})
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestArtifactService_Upload_Validation(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()

	// missing session_id
	_, err := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{Name: "x", DataBase64: "YQ=="})
	if err == nil {
		t.Error("expected error for missing session_id")
	}

	// missing name
	_, err = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{SessionId: "s", DataBase64: "YQ=="})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// bad base64
	_, err = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{SessionId: "s", Name: "f", DataBase64: "!!!invalid!!!"})
	if err == nil {
		t.Error("expected error for invalid base64")
	}

	// oversize payload
	oversized := make([]byte, artifactbiz.MaxUploadBytes+1)
	_, err = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId:  "s",
		Name:       "big.bin",
		DataBase64: base64.StdEncoding.EncodeToString(oversized),
	})
	if err == nil {
		t.Error("expected error for oversize upload")
	}
}

func TestArtifactService_List(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
			SessionId:  "sess-list",
			Name:       fmt.Sprintf("file-%d.txt", i),
			DataBase64: base64.StdEncoding.EncodeToString([]byte("data")),
		})
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	resp, err := svc.ListArtifacts(ctx, &v1.ListArtifactsRequest{SessionId: "sess-list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.GetItems()) != 3 {
		t.Errorf("expected 3 items, got %d", len(resp.GetItems()))
	}
}

func TestArtifactService_List_Filter(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()

	_, _ = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId: "sess-f", Name: "photo.png", MimeType: "image/png",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("img")),
	})
	_, _ = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId: "sess-f", Name: "readme.txt", MimeType: "text/plain",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("txt")),
	})

	resp, err := svc.ListArtifacts(ctx, &v1.ListArtifactsRequest{
		SessionId: "sess-f", MimeTypePrefix: "image/",
	})
	if err != nil {
		t.Fatalf("list filter: %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetName() != "photo.png" {
		t.Fatalf("expected image filter, got %+v", resp.GetItems())
	}
}

func TestArtifactService_ListArtifactVersions(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()

	meta1, _ := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId: "sess-v", Name: "file.bin", DataBase64: base64.StdEncoding.EncodeToString([]byte("v0")),
	})
	_, _ = svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId: "sess-v", Name: "file.bin", DataBase64: base64.StdEncoding.EncodeToString([]byte("v1")),
	})

	resp, err := svc.ListArtifactVersions(ctx, &v1.ListArtifactVersionsRequest{Id: meta1.GetId()})
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(resp.GetItems()) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(resp.GetItems()))
	}
}

func TestArtifactService_Delete_Validation(t *testing.T) {
	svc := newArtifactService()
	ctx := context.Background()

	_, err := svc.DeleteArtifact(ctx, &v1.DeleteArtifactRequest{Id: ""})
	if err == nil {
		t.Error("expected error for empty id")
	}
}
