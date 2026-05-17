package service

import (
	"context"
	"encoding/base64"
	"strings"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ArtifactService implements kratos artifact.v1.
type ArtifactService struct {
	v1.UnimplementedArtifactServiceServer
	uc *biz.ArtifactUsecase
}

// NewArtifactService constructs an ArtifactService.
func NewArtifactService(uc *biz.ArtifactUsecase) *ArtifactService {
	return &ArtifactService{uc: uc}
}

// UploadArtifact stores a base64-encoded artifact and returns its metadata.
func (s *ArtifactService) UploadArtifact(ctx context.Context, req *v1.UploadArtifactRequest) (*v1.ArtifactMeta, error) {
	if strings.TrimSpace(req.GetSessionId()) == "" {
		return nil, kerrors.BadRequest("ARTIFACT", "session_id is required")
	}
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, kerrors.BadRequest("ARTIFACT", "name is required")
	}
	data, err := base64.StdEncoding.DecodeString(req.GetDataBase64())
	if err != nil {
		return nil, kerrors.BadRequest("ARTIFACT", "data_base64 is not valid base64: "+err.Error())
	}
	if len(data) > 50<<20 { // 50 MB cap
		return nil, kerrors.BadRequest("ARTIFACT", "artifact exceeds 50 MB size limit")
	}
	mime := strings.TrimSpace(req.GetMimeType())
	if mime == "" {
		mime = "application/octet-stream"
	}
	saved, err := s.uc.Save(ctx, req.GetSessionId(), req.GetName(), mime, data)
	if err != nil {
		return nil, err
	}
	return toProtoArtifactMeta(saved), nil
}

// GetArtifact returns an artifact with its binary payload (base64-encoded).
func (s *ArtifactService) GetArtifact(ctx context.Context, req *v1.GetArtifactRequest) (*v1.ArtifactData, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, kerrors.BadRequest("ARTIFACT", "id is required")
	}
	version := int(req.GetVersion())
	meta, data, err := s.uc.Load(ctx, id, version)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, kerrors.NotFound("ARTIFACT", err.Error())
		}
		return nil, err
	}
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
	items, total, err := s.uc.List(ctx, req.GetSessionId(), limit, offset)
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

// DeleteArtifact removes an artifact and all its versions.
func (s *ArtifactService) DeleteArtifact(ctx context.Context, req *v1.DeleteArtifactRequest) (*emptypb.Empty, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, kerrors.BadRequest("ARTIFACT", "id is required")
	}
	if err := s.uc.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
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
