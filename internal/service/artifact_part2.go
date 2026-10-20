package service

import (
	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/metrics"
	"context"
	"strings"
)

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
