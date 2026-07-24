package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/artifact"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// fakeSessionLookup 是 sessionWorkspaceLookup 的测试 mock。
// P1-1: IDOR 测试用，按 sessionID 返回预设的 Session（含 WorkspaceID）。
type fakeSessionLookup struct {
	sessions map[string]biz.Session
}

func (f *fakeSessionLookup) Get(_ context.Context, id string) (biz.Session, error) {
	if s, ok := f.sessions[id]; ok {
		return s, nil
	}
	return biz.Session{}, fmt.Errorf("session not found: %s", id)
}

// newArtifactServiceWithLookup 构造带 sessionLookup 的 ArtifactService，用于 IDOR 测试。
func newArtifactServiceWithLookup(lookup sessionWorkspaceLookup) *ArtifactService {
	repo := newMemArtifactRepoB()
	uc := biz.NewArtifactUsecase(repo, loggateway.NewNoop())
	signer := artifact.NewSigner(loggateway.NewNoop())
	return NewArtifactService(uc, signer, lookup, nil)
}

// newMemArtifactRepoB 复用 biz_coverage_test 的内存 repo（package service 内可见）。
// 为避免命名冲突，这里定义独立构造器。
func newMemArtifactRepoB() artifactbiz.Repo {
	return &memArtifactRepoB{items: make(map[string]biz.Artifact), data: make(map[string][]byte)}
}

type memArtifactRepoB struct {
	items map[string]biz.Artifact
	data  map[string][]byte
}

func (m *memArtifactRepoB) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	id := biz.NewArtifactID()
	a := biz.Artifact{
		ID: id, SessionID: sessionID, Name: name, MimeType: mimeType,
		Size: int64(len(data)), Version: 1,
	}
	m.items[id] = a
	m.data[id] = data
	return a, nil
}

func (m *memArtifactRepoB) Load(_ context.Context, id string, _ int) (biz.Artifact, []byte, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, nil, fmt.Errorf("artifact not found: %s", id)
	}
	return a, m.data[id], nil
}

func (m *memArtifactRepoB) LoadMeta(_ context.Context, id string, _ int) (biz.Artifact, error) {
	a, ok := m.items[id]
	if !ok {
		return biz.Artifact{}, fmt.Errorf("artifact not found: %s", id)
	}
	return a, nil
}

func (m *memArtifactRepoB) LoadMetas(_ context.Context, ids []string, _ int) ([]biz.Artifact, error) {
	out := make([]biz.Artifact, 0, len(ids))
	for _, id := range ids {
		if a, ok := m.items[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memArtifactRepoB) List(_ context.Context, sessionID string, limit, _ int) ([]biz.Artifact, int, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		// 空 sessionID = 跨 session 全量（对齐 FSArtifactRepo.List 语义）。
		if sessionID == "" || a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, len(out), nil
}

func (m *memArtifactRepoB) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	delete(m.data, id)
	return nil
}

func (m *memArtifactRepoB) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	return nil
}

func (m *memArtifactRepoB) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	var out []biz.Artifact
	for _, a := range m.items {
		if a.SessionID == sessionID && a.Name == name {
			out = append(out, a)
		}
	}
	return out, nil
}

// uploadArtifactHelper 上传一个 artifact 并返回其 ID（测试辅助）。
func uploadArtifactHelper(t *testing.T, svc *ArtifactService, ctx context.Context, sessionID, name string) string {
	t.Helper()
	meta, err := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId:  sessionID,
		Name:       name,
		MimeType:   "text/plain",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("payload")),
	})
	if err != nil {
		t.Fatalf("upload helper failed: %v", err)
	}
	return meta.GetId()
}

// --- RED 测试：IDOR 防护（期望当前实现失败，作为修复回归基线） ---

// TestArtifactService_IDOR_GetArtifact_RejectedAcrossWorkspaces 验证
// callerWS="ws-A" 访问属于 session（workspace="ws-B"）的 artifact 时被 Forbidden。
//
// P1-1 修复前：GetArtifact 只按 id 查询，无 workspace 校验 → IDOR 成立。
// P1-1 修复后：GetArtifact 调用 assertWorkspaceOwnsArtifact → Forbidden。
func TestArtifactService_IDOR_GetArtifact_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	// 先用 system workspace 上传 artifact（绕过校验），属于 sess-victim。
	uploadCtx := workspace.WithSystemWorkspace(context.Background())
	artID := uploadArtifactHelper(t, svc, uploadCtx, "sess-victim", "secret.txt")

	// attacker 用自己的 workspace 访问 victim 的 artifact。
	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.GetArtifact(attackerCtx, &v1.GetArtifactRequest{Id: artID})
	if err == nil {
		t.Fatal("IDOR: GetArtifact 允许跨 workspace 访问（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// TestArtifactService_IDOR_GetArtifact_SameWorkspace_Allowed 验证
// 同 workspace 访问不被误拒。
func TestArtifactService_IDOR_GetArtifact_SameWorkspace_Allowed(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	uploadCtx := workspace.WithContext(context.Background(), "ws-ok")
	artID := uploadArtifactHelper(t, svc, uploadCtx, "sess-ok", "ok.txt")

	// 同 workspace 访问应成功。
	_, err := svc.GetArtifact(uploadCtx, &v1.GetArtifactRequest{Id: artID})
	if err != nil {
		t.Fatalf("同 workspace 访问应成功，got: %v", err)
	}
}

// TestArtifactService_IDOR_DeleteArtifact_RejectedAcrossWorkspaces 验证
// 跨 workspace 删除 artifact 被 Forbidden。
func TestArtifactService_IDOR_DeleteArtifact_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	uploadCtx := workspace.WithSystemWorkspace(context.Background())
	artID := uploadArtifactHelper(t, svc, uploadCtx, "sess-victim", "secret.txt")

	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.DeleteArtifact(attackerCtx, &v1.DeleteArtifactRequest{Id: artID})
	if err == nil {
		t.Fatal("IDOR: DeleteArtifact 允许跨 workspace 删除（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// TestArtifactService_IDOR_ListArtifacts_RejectedAcrossWorkspaces 验证
// 跨 workspace 列举 session 的 artifacts 被 Forbidden。
func TestArtifactService_IDOR_ListArtifacts_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.ListArtifacts(attackerCtx, &v1.ListArtifactsRequest{SessionId: "sess-victim"})
	if err == nil {
		t.Fatal("IDOR: ListArtifacts 允许跨 workspace 列举（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// TestArtifactService_IDOR_UploadArtifact_RejectedAcrossWorkspaces 验证
// 跨 workspace 上传 artifact 到 victim session 被 Forbidden。
func TestArtifactService_IDOR_UploadArtifact_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.UploadArtifact(attackerCtx, &v1.UploadArtifactRequest{
		SessionId:  "sess-victim",
		Name:       "malware.txt",
		DataBase64: base64.StdEncoding.EncodeToString([]byte("evil")),
	})
	if err == nil {
		t.Fatal("IDOR: UploadArtifact 允许跨 workspace 上传（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// TestArtifactService_IDOR_PreviewArtifact_RejectedAcrossWorkspaces 验证
// 跨 workspace 预览 artifact 被 Forbidden。
func TestArtifactService_IDOR_PreviewArtifact_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	uploadCtx := workspace.WithSystemWorkspace(context.Background())
	artID := uploadArtifactHelper(t, svc, uploadCtx, "sess-victim", "secret.txt")

	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.PreviewArtifact(attackerCtx, &v1.PreviewArtifactRequest{Id: artID})
	if err == nil {
		t.Fatal("IDOR: PreviewArtifact 允许跨 workspace 预览（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// fakeSessionSearcher 是 sessionWorkspaceSearcher 的测试 mock。
// 按 WorkspaceID 返回预设的 session 列表（「全部产物」workspace 过滤用）。
type fakeSessionSearcher struct {
	sessions    map[string][]biz.Session // workspaceID → sessions
	gotQueryWS  string
	searchCalls int
}

func (f *fakeSessionSearcher) Search(_ context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
	f.searchCalls++
	f.gotQueryWS = q.WorkspaceID
	items := f.sessions[q.WorkspaceID]
	return biz.SessionListResult{Items: items, Total: len(items)}, nil
}

// newArtifactServiceFull 构造带 lookup + searcher 的 ArtifactService（全部产物测试）。
func newArtifactServiceFull(lookup sessionWorkspaceLookup, searcher sessionWorkspaceSearcher) *ArtifactService {
	repo := newMemArtifactRepoB()
	uc := biz.NewArtifactUsecase(repo, loggateway.NewNoop())
	signer := artifact.NewSigner(loggateway.NewNoop())
	return NewArtifactService(uc, signer, lookup, searcher)
}

// TestArtifactService_ListArtifacts_EmptySession_WorkspaceScoped 验证
// 「全部产物」Tab（空 session_id）只返回 caller workspace 的产物，不泄露其他租户数据。
func TestArtifactService_ListArtifacts_EmptySession_WorkspaceScoped(t *testing.T) {
	lookup := &fakeSessionLookup{sessions: map[string]biz.Session{
		"sess-a1": {ID: "sess-a1", WorkspaceID: "ws-A"},
		"sess-a2": {ID: "sess-a2", WorkspaceID: "ws-A"},
		"sess-b1": {ID: "sess-b1", WorkspaceID: "ws-B"},
	}}
	searcher := &fakeSessionSearcher{sessions: map[string][]biz.Session{
		"ws-A": {{ID: "sess-a1", WorkspaceID: "ws-A"}, {ID: "sess-a2", WorkspaceID: "ws-A"}},
	}}
	svc := newArtifactServiceFull(lookup, searcher)

	// system workspace 上传三个产物：两个属于 ws-A，一个属于 ws-B。
	sysCtx := workspace.WithSystemWorkspace(context.Background())
	uploadArtifactHelper(t, svc, sysCtx, "sess-a1", "a1.txt")
	uploadArtifactHelper(t, svc, sysCtx, "sess-a2", "a2.txt")
	uploadArtifactHelper(t, svc, sysCtx, "sess-b1", "b1.txt")

	callerCtx := workspace.WithContext(context.Background(), "ws-A")
	res, err := svc.ListArtifacts(callerCtx, &v1.ListArtifactsRequest{SessionId: ""})
	if err != nil {
		t.Fatalf("空 session_id 应成功（workspace 过滤），got: %v", err)
	}
	if searcher.searchCalls == 0 {
		t.Fatal("应调用 searcher.Search 做 workspace 过滤")
	}
	if searcher.gotQueryWS != "ws-A" {
		t.Fatalf("Search WorkspaceID=%q, want ws-A", searcher.gotQueryWS)
	}
	names := map[string]bool{}
	for _, it := range res.GetItems() {
		names[it.GetName()] = true
		if it.GetSessionId() == "sess-b1" {
			t.Fatalf("泄露了其他 workspace 的产物: %+v", it)
		}
	}
	if !names["a1.txt"] || !names["a2.txt"] || len(res.GetItems()) != 2 {
		t.Fatalf("应只返回 ws-A 的 2 个产物，got: %v", res.GetItems())
	}
}

// TestArtifactService_ListArtifacts_EmptySession_SystemWorkspace 验证
// system workspace（cron/admin）空 session_id 列出全部产物。
func TestArtifactService_ListArtifacts_EmptySession_SystemWorkspace(t *testing.T) {
	lookup := &fakeSessionLookup{sessions: map[string]biz.Session{
		"sess-a1": {ID: "sess-a1", WorkspaceID: "ws-A"},
		"sess-b1": {ID: "sess-b1", WorkspaceID: "ws-B"},
	}}
	searcher := &fakeSessionSearcher{sessions: map[string][]biz.Session{}}
	svc := newArtifactServiceFull(lookup, searcher)

	sysCtx := workspace.WithSystemWorkspace(context.Background())
	uploadArtifactHelper(t, svc, sysCtx, "sess-a1", "a1.txt")
	uploadArtifactHelper(t, svc, sysCtx, "sess-b1", "b1.txt")

	res, err := svc.ListArtifacts(sysCtx, &v1.ListArtifactsRequest{SessionId: ""})
	if err != nil {
		t.Fatalf("system workspace 空 session_id 应成功，got: %v", err)
	}
	if len(res.GetItems()) != 2 {
		t.Fatalf("system workspace 应看到全部 2 个产物，got: %d", len(res.GetItems()))
	}
	if searcher.searchCalls != 0 {
		t.Fatal("system workspace 不应调用 searcher（直接全量）")
	}
}

// TestArtifactService_ListArtifacts_EmptySession_NoSessionsInWorkspace 验证
// workspace 下无 session 时返回空列表而非报错。
func TestArtifactService_ListArtifacts_EmptySession_NoSessionsInWorkspace(t *testing.T) {
	lookup := &fakeSessionLookup{sessions: map[string]biz.Session{}}
	searcher := &fakeSessionSearcher{sessions: map[string][]biz.Session{}}
	svc := newArtifactServiceFull(lookup, searcher)

	callerCtx := workspace.WithContext(context.Background(), "ws-empty")
	res, err := svc.ListArtifacts(callerCtx, &v1.ListArtifactsRequest{SessionId: ""})
	if err != nil {
		t.Fatalf("无 session 的 workspace 应返回空列表，got: %v", err)
	}
	if len(res.GetItems()) != 0 || res.GetTotal() != 0 {
		t.Fatalf("应返回空，got items=%d total=%d", len(res.GetItems()), res.GetTotal())
	}
}

// TestArtifactService_IDOR_SignDownloadUrl_RejectedAcrossWorkspaces 验证
// 跨 workspace 签名下载 URL 被 Forbidden。
func TestArtifactService_IDOR_SignDownloadUrl_RejectedAcrossWorkspaces(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-victim": {ID: "sess-victim", WorkspaceID: "ws-victim"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)

	uploadCtx := workspace.WithSystemWorkspace(context.Background())
	artID := uploadArtifactHelper(t, svc, uploadCtx, "sess-victim", "secret.txt")

	attackerCtx := workspace.WithContext(context.Background(), "ws-attacker")
	_, err := svc.SignDownloadUrl(attackerCtx, &v1.SignDownloadUrlRequest{Id: artID})
	if err == nil {
		t.Fatal("IDOR: SignDownloadUrl 允许跨 workspace 签名（期望 Forbidden）")
	}
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("期望 Forbidden，got: %v", err)
	}
}

// TestArtifactService_SignedDownload_BindsWorkspace 验证 C-02：
// 签名 URL 绑定 workspace_id；篡改 workspace_id 或跨租户 token 被拒绝。
func TestArtifactService_SignedDownload_BindsWorkspace(t *testing.T) {
	t.Setenv("DEPLOY_ENV", "dev")
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")

	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)
	ctx := workspace.WithContext(context.Background(), "ws-ok")
	artID := uploadArtifactHelper(t, svc, ctx, "sess-ok", "ok.txt")

	signed, err := svc.SignDownloadUrl(ctx, &v1.SignDownloadUrlRequest{Id: artID, Version: 1})
	if err != nil {
		t.Fatalf("SignDownloadUrl: %v", err)
	}
	if !strings.Contains(signed.GetUrl(), "workspace_id=ws-ok") {
		t.Fatalf("signed URL missing workspace_id: %s", signed.GetUrl())
	}

	// Valid signed download succeeds.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, signed.GetUrl(), nil)
	svc.ServeSignedDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Tampered workspace_id must fail HMAC verification.
	u, err := url.Parse(signed.GetUrl())
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := u.Query()
	q.Set("workspace_id", "ws-attacker")
	u.RawQuery = q.Encode()
	rec2 := httptest.NewRecorder()
	svc.ServeSignedDownload(rec2, httptest.NewRequest(http.MethodGet, u.String(), nil))
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for tampered workspace_id, got %d", rec2.Code)
	}
}
