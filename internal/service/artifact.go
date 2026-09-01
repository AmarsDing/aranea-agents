package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/artifact"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/conf"
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

// sessionWorkspaceSearcher lists sessions within a caller workspace.
// Used by ListArtifacts for the "all artifacts" browse (empty session_id):
// the artifact store is keyed by session, so workspace-scoped listing must
// first resolve which sessions belong to the caller workspace.
// Satisfied by *biz.SessionUsecase.
type sessionWorkspaceSearcher interface {
	Search(ctx context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error)
}

// ProvideSessionWorkspaceLookup 适配 *biz.SessionUsecase 到 sessionWorkspaceLookup。
// 供 wire 注入到 ArtifactService 做 IDOR 防护。
func ProvideSessionWorkspaceLookup(uc *biz.SessionUsecase) sessionWorkspaceLookup {
	return uc
}

// ProvideSessionWorkspaceSearcher 适配 *biz.SessionUsecase 到 sessionWorkspaceSearcher。
// 供 wire 注入到 ArtifactService 做「全部产物」workspace 过滤。
func ProvideSessionWorkspaceSearcher(uc *biz.SessionUsecase) sessionWorkspaceSearcher {
	return uc
}

// ArtifactService implements kratos artifact.v1.
type ArtifactService struct {
	v1.UnimplementedArtifactServiceServer
	uc              *biz.ArtifactUsecase
	signer          *artifact.Signer
	sessionLookup   sessionWorkspaceLookup   // P1-1: IDOR 防护
	sessionSearcher sessionWorkspaceSearcher // 「全部产物」workspace 过滤
}

// NewArtifactService constructs an ArtifactService.
// sessionLookup 用于 IDOR 防护（P1-1）：解析 session→workspace 做跨租户校验。
// 传 nil 则跳过 workspace 校验（仅向后兼容旧测试；生产必须由 wire 注入）。
// sessionSearcher 用于空 session_id 的「全部产物」workspace 过滤；
// 传 nil 时空 session_id 退化为跨 session 全量（与 nil sessionLookup 的旧语义一致）。
func NewArtifactService(uc *biz.ArtifactUsecase, signer *artifact.Signer, sl sessionWorkspaceLookup, ss sessionWorkspaceSearcher) *ArtifactService {
	s := &ArtifactService{uc: uc, signer: signer, sessionLookup: sl, sessionSearcher: ss}
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
	return s.protoArtifactMeta(saved), nil
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
		Meta:       s.protoArtifactMeta(meta),
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}, nil
}

// ListArtifacts returns artifact metadata for a session (no payload).
// session_id 为空 = 「全部产物」浏览：列出 caller workspace 下所有 session 的产物
// （system workspace 或 nil sessionSearcher 时退化为跨 session 全量）。
func (s *ArtifactService) ListArtifacts(ctx context.Context, req *v1.ListArtifactsRequest) (*v1.ListArtifactsResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	limit := int(req.GetLimit())
	offset := int(req.GetOffset())
	if limit <= 0 {
		limit = 50
	}
	query := strings.TrimSpace(req.GetQuery())
	mimePrefix := strings.TrimSpace(req.GetMimeTypePrefix())

	var (
		items []artifactbiz.Artifact
		total int
		err   error
	)
	if sessionID == "" {
		items, total, err = s.listAllInWorkspace(ctx, limit, offset, query, mimePrefix)
	} else {
		// P1-1: IDOR 防护 — 校验 caller workspace 拥有目标 session。
		if err = s.assertWorkspaceOwnsSession(ctx, sessionID); err != nil {
			return nil, err
		}
		items, total, err = s.uc.List(ctx, sessionID, limit, offset, query, mimePrefix)
	}
	if err != nil {
		return nil, err
	}
	pbItems := make([]*v1.ArtifactMeta, 0, len(items))
	for _, it := range items {
		pbItems = append(pbItems, s.protoArtifactMeta(it))
	}
	return &v1.ListArtifactsResponse{
		Items: pbItems,
		Total: int32(total),
	}, nil
}

// listAllInWorkspace 实现空 session_id 的「全部产物」浏览。
// system workspace / nil sessionSearcher → 跨 session 全量（旧语义）；
// 否则先解析 caller workspace 的 session IDs（分页拉取），再按 session 聚合产物。
func (s *ArtifactService) listAllInWorkspace(ctx context.Context, limit, offset int, query, mimePrefix string) ([]artifactbiz.Artifact, int, error) {
	callerWS := workspace.IDFromContext(ctx)
	if s.sessionSearcher == nil || callerWS == workspace.SystemWorkspaceID {
		return s.uc.List(ctx, "", limit, offset, query, mimePrefix)
	}
	sessionIDs, err := s.listWorkspaceSessionIDs(ctx, callerWS)
	if err != nil {
		return nil, 0, err
	}
	return s.uc.ListBySessions(ctx, sessionIDs, limit, offset, query, mimePrefix)
}

// listWorkspaceSessionIDs 分页拉取 caller workspace 的全部 session IDs。
// SessionUsecase.Search 单次上限 100（normalizeSessionSearch），超过需翻页。
func (s *ArtifactService) listWorkspaceSessionIDs(ctx context.Context, workspaceID string) ([]string, error) {
	const pageSize = 100
	ids := make([]string, 0, pageSize)
	for page := 1; ; page++ {
		res, err := s.sessionSearcher.Search(ctx, biz.SessionSearchQuery{
			WorkspaceID: workspaceID,
			Page:        page,
			PageSize:    pageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, sess := range res.Items {
			ids = append(ids, sess.ID)
		}
		if len(ids) >= res.Total || len(res.Items) == 0 {
			return ids, nil
		}
	}
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
		pbItems = append(pbItems, s.protoArtifactMeta(it))
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
	// 版本号为 0 基（首个版本即 v0），删除路径 version 是显式必填路径参数，
	// 无"0=latest"歧义，仅拒绝负值。
	if version < 0 {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "version must be >= 0")
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
	resp := &v1.PreviewArtifactResponse{Meta: s.protoArtifactMeta(result.Meta)}
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
	case artifactbiz.PreviewKindAudio:
		// No base64 payload: browsers play via signed URL with inline=1.
		resp.PreviewKind = "audio"
	case artifactbiz.PreviewKindVideo:
		resp.PreviewKind = "video"
	default:
		resp.PreviewKind = "binary"
	}
	return resp, nil
}

// SignDownloadUrl returns a time-limited HMAC-signed download URL.
// C-02: token payload binds workspace_id so forged cross-tenant URLs fail verification.
func (s *ArtifactService) SignDownloadUrl(ctx context.Context, req *v1.SignDownloadUrlRequest) (*v1.SignDownloadUrlResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "id is required")
	}
	version := int(req.GetVersion())
	// P1-1: IDOR 防护 — 校验 workspace 所有权后再签名。
	meta, err := s.assertWorkspaceOwnsArtifact(ctx, id, version)
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	expires := time.Now().UTC().Add(ttl)
	wsID, err := s.artifactWorkspaceID(ctx, meta.SessionID)
	if err != nil {
		return nil, err
	}
	token, err := s.signer.DownloadToken(id, version, expires, wsID)
	if err != nil {
		// OUT-05 / ART-02: prod environments without a configured key must
		// fail closed; never hand out a forgeable URL signed with the dev key.
		return nil, apierror.Unavailable(apierror.DomainArtifact, "download signing unavailable")
	}
	q := fmt.Sprintf("/v1/artifacts/download?id=%s&version=%d&expires=%d&workspace_id=%s&token=%s",
		url.QueryEscape(id), version, expires.Unix(), url.QueryEscape(wsID), url.QueryEscape(token))
	return &v1.SignDownloadUrlResponse{
		Url:       q,
		ExpiresAt: expires.Format(time.RFC3339),
	}, nil
}

// artifactWorkspaceID resolves the owning workspace for a session (bound into signed URLs).
func (s *ArtifactService) artifactWorkspaceID(ctx context.Context, sessionID string) (string, error) {
	if s.sessionLookup == nil {
		return workspace.IDFromContext(ctx), nil
	}
	sess, err := s.sessionLookup.Get(ctx, sessionID)
	if err != nil {
		return "", apierror.NotFound(apierror.DomainArtifact, "session not found")
	}
	ws := strings.TrimSpace(sess.WorkspaceID)
	if ws == "" {
		ws = workspace.DefaultWorkspaceID
	}
	return ws, nil
}

// ServeSignedDownload streams artifact bytes when the HMAC token is valid.
// C-02: verifies workspace_id from the signed payload matches the artifact owner.
func (s *ArtifactService) ServeSignedDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
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
	ok, verr := s.signer.VerifyDownloadToken(id, version, expiresUnix, wsID, token)
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
	// C-02: token workspace must match the artifact's owning workspace.
	ownerWS, wsErr := s.artifactWorkspaceID(r.Context(), meta.SessionID)
	if wsErr != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := workspace.AssertWorkspace(wsID, ownerWS); err != nil {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}
	mime := strings.TrimSpace(meta.MimeType)
	if mime == "" {
		mime = "application/octet-stream"
	}
	// inline=1 lets browsers play media via <audio>/<video> src. Restricted to
	// non-executable media types (design §13.7: no HTML/JS inline injection).
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && artifactInlineAllowed(mime) {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, meta.Name))
	metrics.ArtifactDownloadBytesTotal.Add(float64(len(data)))
	// ServeContent handles Range requests (video seeking) and sets
	// Content-Length / Content-Range; modtime is best-effort from CreatedAt.
	http.ServeContent(w, r, meta.Name, artifactModTime(meta.CreatedAt), bytes.NewReader(data))
}

// artifactInlineAllowed reports whether a MIME type may be served with
// Content-Disposition: inline. Only media browsers render without script
// execution risk qualify (audio/video/image); everything else stays
// attachment even when inline=1 is requested.
func artifactInlineAllowed(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	return strings.HasPrefix(m, "audio/") || strings.HasPrefix(m, "video/") || strings.HasPrefix(m, "image/")
}

// artifactModTime parses CreatedAt (RFC3339) for http.ServeContent. A zero
// time is returned when unset/unparseable, disabling Last-Modified.
func artifactModTime(createdAt string) time.Time {
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(createdAt)); err == nil {
		return ts
	}
	return time.Time{}
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

// protoArtifactMeta wraps toProtoArtifactMeta and, for local FS backends,
// resolves the on-disk absolute path into StorageUri. The absolute host path
// is only disclosed when conf.LocalRevealEnabled() (M27 Phase 5: local
// single-user deployments); otherwise the stored relative URI is returned so
// API responses never leak the host filesystem layout (OUT-05 / ART-03).
//
// S06 产物可见性（2026-09-01）：远程服务器部署下配置
// ARANEA_ARTIFACT_PUBLIC_BASE（如 108 的 UNC 共享根）时，StorageUri 映射为
// 用户可访问的公开路径，前端据此展示/复制「定位到文件夹」路径。
func (s *ArtifactService) protoArtifactMeta(a biz.Artifact) *v1.ArtifactMeta {
	m := toProtoArtifactMeta(a)
	if s.uc != nil && conf.LocalRevealEnabled() {
		if abs := s.uc.ResolveAbsPath(a); abs != "" {
			m.StorageUri = abs
		}
		return m
	}
	if base := conf.ArtifactPublicBase(); base != "" && a.StorageURI != "" {
		// 存储相对路径（正斜杠）映射为公开根下的反斜杠路径（UNC/Windows 形态）。
		m.StorageUri = base + `\` + strings.ReplaceAll(a.StorageURI, "/", `\`)
	}
	return m
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
