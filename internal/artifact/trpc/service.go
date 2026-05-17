// Package trpc adapts biz.ArtifactUsecase to implement the
// pkg/trpc-agent-go/artifact.Service interface used by the agent runtime.
package trpc

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
)

// ServiceAdapter implements trpcartifact.Service backed by biz.ArtifactUsecase.
type ServiceAdapter struct {
	uc *biz.ArtifactUsecase
}

// NewServiceAdapter creates a ServiceAdapter.
func NewServiceAdapter(uc *biz.ArtifactUsecase) *ServiceAdapter {
	return &ServiceAdapter{uc: uc}
}

var _ trpcartifact.Service = (*ServiceAdapter)(nil)

// SaveArtifact saves an artifact and returns the revision ID (version number).
func (s *ServiceAdapter) SaveArtifact(ctx context.Context, sessionInfo trpcartifact.SessionInfo, filename string, a *trpcartifact.Artifact) (int, error) {
	if a == nil {
		return 0, fmt.Errorf("artifact is nil")
	}
	mime := a.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	saved, err := s.uc.Save(ctx, sessionInfo.SessionID, filename, mime, a.Data)
	if err != nil {
		return 0, err
	}
	return saved.Version, nil
}

// LoadArtifact retrieves an artifact.  version == nil → latest.
func (s *ServiceAdapter) LoadArtifact(ctx context.Context, sessionInfo trpcartifact.SessionInfo, filename string, version *int) (*trpcartifact.Artifact, error) {
	// List all versions for this session+name to find by filename.
	versions, err := s.uc.ListVersions(ctx, sessionInfo.SessionID, filename)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}
	target := versions[len(versions)-1] // latest
	if version != nil {
		found := false
		for _, v := range versions {
			if v.Version == *version {
				target = v
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("artifact version %d not found for %s", *version, filename)
		}
	}

	_, data, err := s.uc.Load(ctx, target.ID, target.Version)
	if err != nil {
		return nil, err
	}
	return &trpcartifact.Artifact{
		Data:     data,
		MimeType: target.MimeType,
		Name:     target.Name,
	}, nil
}

// ListArtifactKeys lists all artifact filenames for a session.
func (s *ServiceAdapter) ListArtifactKeys(ctx context.Context, sessionInfo trpcartifact.SessionInfo) ([]string, error) {
	items, _, err := s.uc.List(ctx, sessionInfo.SessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names, nil
}

// DeleteArtifact removes an artifact by filename (deletes all matching versions).
func (s *ServiceAdapter) DeleteArtifact(ctx context.Context, sessionInfo trpcartifact.SessionInfo, filename string) error {
	versions, err := s.uc.ListVersions(ctx, sessionInfo.SessionID, filename)
	if err != nil {
		return err
	}
	for _, v := range versions {
		if err := s.uc.Delete(ctx, v.ID); err != nil {
			return err
		}
	}
	return nil
}

// ListVersions returns all available version numbers for an artifact.
func (s *ServiceAdapter) ListVersions(ctx context.Context, sessionInfo trpcartifact.SessionInfo, filename string) ([]int, error) {
	versions, err := s.uc.ListVersions(ctx, sessionInfo.SessionID, filename)
	if err != nil {
		return nil, err
	}
	nums := make([]int, 0, len(versions))
	for _, v := range versions {
		nums = append(nums, v.Version)
	}
	return nums, nil
}
