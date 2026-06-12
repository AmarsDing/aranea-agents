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

	"aranea-agents/pkg/apierror"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ArtifactService implements kratos artifact.v1.
type ArtifactService struct {
	v1.UnimplementedArtifactServiceServer
	uc     *biz.ArtifactUsecase
	signer *artifact.Signer
}

// NewArtifactService constructs an ArtifactService.
func NewArtifactService(uc *biz.ArtifactUsecase, signer *artifact.Signer) *ArtifactService {
	s := &ArtifactService{uc: uc, signer: signer}
	s.refreshStorageGauge(context.Background())
	return s
}

// UploadArtifact stores a base64-encoded artifact and returns its metadata.
func (s *ArtifactService) UploadArtifact(ctx context.Context, req *v1.UploadArtifactRequest) (*v1.ArtifactMeta, error) {
	if strings.TrimSpace(req.GetSessionId()) == "" {
		return nil, apierror.BadRequest("ARTIFACT", "session_id is required")
	}
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, apierror.BadRequest("ARTIFACT", "name is required")
	}
	data, err := base64.StdEncoding.DecodeString(req.GetDataBase64())
	if err != nil {
		return nil, apierror.BadRequest("ARTIFACT", "data_base64 is not valid base64: "+err.Error())
	}
	if len(data) > artifactbiz.MaxUploadBytes {
		return nil, apierror.BadRequest("ARTIFACT", "单个制品最大支持 10 MB，当前文件过大暂不支持上传")
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
	}
	version := int(req.GetVersion())
	meta, data, err := s.uc.Load(ctx, id, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apierror.NotFound("ARTIFACT", err.Error())
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
	limit := int(req.GetLimit())
	offset := int(req.GetOffset())
	if limit <= 0 {
		limit = 50
	}
	query := strings.TrimSpace(req.GetQuery())
	mimePrefix := strings.TrimSpace(req.GetMimeTypePrefix())
	items, total, err := s.uc.List(ctx, req.GetSessionId(), limit, offset, query, mimePrefix)
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
	}
	meta, _, err := s.uc.Load(ctx, id, 0)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apierror.NotFound("ARTIFACT", err.Error())
		}
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
	}
	version := int(req.GetVersion())
	if version <= 0 {
		return nil, apierror.BadRequest("ARTIFACT", "version must be > 0")
	}
	if err := s.uc.DeleteVersion(ctx, id, version); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apierror.NotFound("ARTIFACT", err.Error())
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
	}
	version := int(req.GetVersion())
	result, err := s.uc.Preview(ctx, id, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apierror.NotFound("ARTIFACT", err.Error())
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
		return nil, apierror.BadRequest("ARTIFACT", "id is required")
	}
	version := int(req.GetVersion())
	if _, _, err := s.uc.Load(ctx, id, version); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apierror.NotFound("ARTIFACT", err.Error())
		}
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
	token, err := s.signer.DownloadToken(id, version, expires)
	if err != nil {
		// OUT-05 / ART-02: prod environments without a configured key must
		// fail closed; never hand out a forgeable URL signed with the dev key.
		return nil, apierror.Unavailable("ARTIFACT", err.Error())
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
		http.Error(w, verr.Error(), http.StatusServiceUnavailable)
		return
	}
	if !ok {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}
	meta, data, err := s.uc.Load(r.Context(), id, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
