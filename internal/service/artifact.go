package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/artifact"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/workspace"

	"aranea-agents/pkg/apierror"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// sessionWorkspaceLookup resolves a session ID to its Session (carrying
// WorkspaceID). Used by ArtifactService for IDOR protection (P1-1).
// Satisfied by *biz.SessionUsecase.
//
// 2026-07-15 P1-1 修复（审计报告 P1-1）：Artifact 模型只有 SessionID 没有
// WorkspaceID，跨租户访问只能通过 session→workspace 解析校验。
type sessionWorkspaceLookup interface {
	Get(ctx context.Context, id string) (biz.Session, error)
}

// ProvideSessionWorkspaceLookup 适配 *biz.SessionUsecase 到 sessionWorkspaceLookup。
// 供 wire 注入到 ArtifactService 做 IDOR 防护。
func ProvideSessionWorkspaceLookup(uc *biz.SessionUsecase) sessionWorkspaceLookup {
	return uc
}

// ArtifactService implements kratos artifact.v1.
type ArtifactService struct {
	v1.UnimplementedArtifactServiceServer
	uc            *biz.ArtifactUsecase
	signer        *artifact.Signer
	sessionLookup sessionWorkspaceLookup // P1-1: IDOR 防护
}

// NewArtifactService constructs an ArtifactService.
// sessionLookup 用于 IDOR 防护（P1-1）：解析 session→workspace 做跨租户校验。
// 传 nil 则跳过 workspace 校验（仅向后兼容旧测试；生产必须由 wire 注入）。
func NewArtifactService(uc *biz.ArtifactUsecase, signer *artifact.Signer, sl sessionWorkspaceLookup) *ArtifactService {
	s := &ArtifactService{uc: uc, signer: signer, sessionLookup: sl}
	s.refreshStorageGauge(context.Background())
	return s
}

// assertWorkspaceOwnsSession 校验 caller workspace 拥有指定 session。
// P1-1: IDOR 防护 — 防止跨租户访问 artifact。
//
// 校验逻辑：
//  1. sessionLookup 为 nil（旧测试）→ 跳过（向后兼容）
//  2. callerWS == SystemWorkspaceID → 绕过（cron/admin 后台任务）
//  3. 查 session → session.WorkspaceID；session 不存在返回 NotFound（不泄露存在性）
//  4. callerWS != session.WorkspaceID → Forbidden（调用 workspace.AssertWorkspace）
func (s *ArtifactService) assertWorkspaceOwnsSession(ctx context.Context, sessionID string) error {
	if s.sessionLookup == nil {
		return nil // 向后兼容：旧测试未注入 sessionLookup
	}
	if strings.TrimSpace(sessionID) == "" {
		return apierror.BadRequest(apierror.DomainArtifact, "session_id is required")
	}
	callerWS := workspace.IDFromContext(ctx)
	if callerWS == workspace.SystemWorkspaceID {
		return nil // 系统工作空间绕过（cron/admin）
	}
	sess, err := s.sessionLookup.Get(ctx, sessionID)
	if err != nil {
		// session 不存在 → NotFound，不泄露 session 是否存在
		return apierror.NotFound(apierror.DomainArtifact, "session not found")
	}
	// P1-2: 调用 workspace.AssertWorkspace（从 middleware 提升到 workspace 包）。
	return workspace.AssertWorkspace(callerWS, sess.WorkspaceID)
}

// assertWorkspaceOwnsArtifact 校验 caller workspace 拥有指定 artifact（通过其 session）。
// P1-1: IDOR 防护 — artifact 只挂 sessionID，需先 Load meta 再查 session workspace。
// 用 LoadMeta（轻量，不读 payload），返回 meta 供调用方复用。
func (s *ArtifactService) assertWorkspaceOwnsArtifact(ctx context.Context, artifactID string, version int) (artifactbiz.Artifact, error) {
	meta, err := s.uc.LoadMeta(ctx, artifactID, version)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return artifactbiz.Artifact{}, apierror.NotFound(apierror.DomainArtifact, "artifact not found")
		}
		return artifactbiz.Artifact{}, err
	}
	if err := s.assertWorkspaceOwnsSession(ctx, meta.SessionID); err != nil {
		return artifactbiz.Artifact{}, err
	}
	return meta, nil
}

// UploadArtifact stores a base64-encoded artifact and returns its metadata.
func (s *ArtifactService) UploadArtifact(ctx context.Context, req *v1.UploadArtifactRequest) (*v1.ArtifactMeta, error) {
	if strings.TrimSpace(req.GetSessionId()) == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "session_id is required")
	}
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "name is required")
	}
	// P1-1: IDOR 防护 — 校验 caller workspace 拥有目标 session。
	if err := s.assertWorkspaceOwnsSession(ctx, req.GetSessionId()); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(req.GetDataBase64())
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "data_base64 is not valid base64")
	}
	if len(data) > artifactbiz.MaxUploadBytes {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "file exceeds 10 MB upload limit")
	}
	mime := strings.TrimSpace(req.GetMimeType())
	if mime == "" {
		mime = "application/octet-stream"
	}
	saved, err := s.uc.Save(ctx, req.GetSessionId(), req.GetName(), mime, data)
	if err != nil {
		return nil, err
	}
	metrics.ArtifactUploadBytesTotal.Add(float64(len(data)))
	s.refreshStorageGauge(ctx)
	return toProtoArtifactMeta(saved), nil
}

// GetArtifact returns an artifact with its binary payload (base64-encoded).
func (s *ArtifactService) GetArtifact(ctx context.Context, req *v1.GetArtifactRequest) (*v1.ArtifactData, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	version := int(req.GetVersion())
	// P1-1: IDOR 防护 — 先校验 workspace 所有权（LoadMeta + session→workspace）。
	if _, err := s.assertWorkspaceOwnsArtifact(ctx, id, version); err != nil {
		return nil, err
	}
	meta, data, err := s.uc.Load(ctx, id, version)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainArtifact, "artifact not found")
		}
		return nil, err
	}
	metrics.ArtifactDownloadBytesTotal.Add(float64(len(data)))
	return &v1.ArtifactData{
		Meta:       toProtoArtifactMeta(meta),
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}, nil
}

// ListArtifacts returns artifact metadata for a session (no payload).
func (s *ArtifactService) ListArtifacts(ctx context.Context, req *v1.ListArtifactsRequest) (*v1.ListArtifactsResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "session_id is required")
	}
	// P1-1: IDOR 防护 — 校验 caller workspace 拥有目标 session。
	if err := s.assertWorkspaceOwnsSession(ctx, sessionID); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	offset := int(req.GetOffset())
	if limit <= 0 {
		limit = 50
	}
	query := strings.TrimSpace(req.GetQuery())
	mimePrefix := strings.TrimSpace(req.GetMimeTypePrefix())
	items, total, err := s.uc.List(ctx, sessionID, limit, offset, query, mimePrefix)
	if err != nil {
		return nil, err
	}
	pbItems := make([]*v1.ArtifactMeta, 0, len(items))
	for _, it := range items {
		pbItems = append(pbItems, toProtoArtifactMeta(it))
	}
	return &v1.ListArtifactsResponse{
		Items: pbItems,
		Total: int32(total),
	}, nil
}

// ListArtifactVersions returns all version metadata for an artifact logical file.
func (s *ArtifactService) ListArtifactVersions(ctx context.Context, req *v1.ListArtifactVersionsRequest) (*v1.ListArtifactVersionsResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	// P1-1: IDOR 防护 — 校验 workspace 所有权（LoadMeta + session→workspace）。
	meta, err := s.assertWorkspaceOwnsArtifact(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	versions, err := s.uc.ListVersions(ctx, meta.SessionID, meta.Name)
	if err != nil {
		return nil, err
	}
	pbItems := make([]*v1.ArtifactMeta, 0, len(versions))
	for _, it := range versions {
		pbItems = append(pbItems, toProtoArtifactMeta(it))
	}
	return &v1.ListArtifactVersionsResponse{Items: pbItems}, nil
}

// DeleteArtifact removes an artifact and all its versions.
func (s *ArtifactService) DeleteArtifact(ctx context.Context, req *v1.DeleteArtifactRequest) (*emptypb.Empty, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	// P1-1: IDOR 防护 — 校验 workspace 所有权后再删除。
	if _, err := s.assertWorkspaceOwnsArtifact(ctx, id, 0); err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, id); err != nil {
		return nil, err
	}
	s.refreshStorageGauge(ctx)
	return &emptypb.Empty{}, nil
}

// DeleteArtifactVersion removes exactly one version of a logical artifact.
func (s *ArtifactService) DeleteArtifactVersion(ctx context.Context, req *v1.DeleteArtifactVersionRequest) (*emptypb.Empty, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	version := int(req.GetVersion())
	if version <= 0 {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "version must be > 0")
	}
	// P1-1: IDOR 防护 — 校验 workspace 所有权后再删除版本。
	if _, err := s.assertWorkspaceOwnsArtifact(ctx, id, version); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteVersion(ctx, id, version); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainArtifact, "artifact version not found")
		}
		return nil, err
	}
	s.refreshStorageGauge(ctx)
	return &emptypb.Empty{}, nil
}

// PreviewArtifact returns inline preview content for browser rendering.
func (s *ArtifactService) PreviewArtifact(ctx context.Context, req *v1.PreviewArtifactRequest) (*v1.PreviewArtifactResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	version := int(req.GetVersion())
	// P1-1: IDOR 防护 — 校验 workspace 所有权后再预览。
	if _, err := s.assertWorkspaceOwnsArtifact(ctx, id, version); err != nil {
		return nil, err
	}
	result, err := s.uc.Preview(ctx, id, version)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainArtifact, "artifact not found")
		}
		return nil, err
	}
	resp := &v1.PreviewArtifactResponse{Meta: toProtoArtifactMeta(result.Meta)}
	switch result.Kind {
	case artifactbiz.PreviewKindText:
		resp.PreviewKind = "text"
		resp.TextContent = result.TextContent
	case artifactbiz.PreviewKindImage:
		resp.PreviewKind = "image"
		resp.DataBase64 = base64.StdEncoding.EncodeToString(result.Data)
	case artifactbiz.PreviewKindPDF:
		resp.PreviewKind = "pdf"
		resp.DataBase64 = base64.StdEncoding.EncodeToString(result.Data)
	default:
		resp.PreviewKind = "binary"
	}
	return resp, nil
}

// SignDownloadUrl returns a time-limited HMAC-signed download URL.
func (s *ArtifactService) SignDownloadUrl(ctx context.Context, req *v1.SignDownloadUrlRequest) (*v1.SignDownloadUrlResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	version := int(req.GetVersion())
	// P1-1: IDOR 防护 — 校验 workspace 所有权后再签名。
	if _, err := s.assertWorkspaceOwnsArtifact(ctx, id, version); err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24*time.Hour
	}
	expires := time.Now().UTC().Add(ttl)
	token, err := s.signer.DownloadToken(id, version, expires)
	if err != nil {
		// OUT-05 / ART-02: prod environments without a configured key must
		// fail closed; never hand out a forgeable URL signed with the dev key.
		return nil, apierror.Unavailable(apierror.DomainArtifact, "download signing unavailable")
	}
	q := fmt.Sprintf("/v1/artifacts/download?id=%s&version=%d&expires=%d&token=%s",
		id, version, expires.Unix(), token)
	return &v1.SignDownloadUrlResponse{
		Url:       q,
		ExpiresAt: expires.Format(time.RFC3339),
	}, nil
}

// ServeSignedDownload streams artifact bytes when the HMAC token is valid.
func (s *ArtifactService) ServeSignedDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	expiresRaw := strings.TrimSpace(r.URL.Query().Get("expires"))
	version := 0
	if v := strings.TrimSpace(r.URL.Query().Get("version")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			version = n
		}
	}
	expiresUnix, err := artifact.ParseExpires(expiresRaw)
	if err != nil {
		http.Error(w, "invalid expires", http.StatusBadRequest)
		return
	}
	ok, verr := s.signer.VerifyDownloadToken(id, version, expiresUnix, token)
	if verr != nil {
		// OUT-05 / ART-02: surface a clear 503 instead of a 403 storm when prod
		// is missing its key — operators see misconfig, not a generic auth error.
		http.Error(w, "download signing unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ok {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}
	meta, data, err := s.uc.Load(r.Context(), id, version)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	mime := strings.TrimSpace(meta.MimeType)
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.Name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	metrics.ArtifactDownloadBytesTotal.Add(float64(len(data)))
	if _, err := w.Write(data); err != nil {
		// Client may have disconnected mid-stream; nothing we can do but log.
		// Do not treat as a server error since the response headers are already sent.
		return
	}
}

func toProtoArtifactMeta(a biz.Artifact) *v1.ArtifactMeta {
	return &v1.ArtifactMeta{
		Id:          a.ID,
		SessionId:   a.SessionID,
		Name:        a.Name,
		MimeType:    a.MimeType,
		Size:        a.Size,
		Sha256:      a.SHA256,
		StorageKind: a.StorageKind,
		StorageUri:  a.StorageURI,
		Version:     int32(a.Version),
		CreatedAt:   a.CreatedAt,
	}
}

func (s *ArtifactService) refreshStorageGauge(ctx context.Context) {
	if s == nil || s.uc == nil {
		return
	}
	n, err := s.uc.StorageBytes(ctx)
	if err != nil {
		return
	}
	metrics.ArtifactStorageBytes.Set(float64(n))
}
