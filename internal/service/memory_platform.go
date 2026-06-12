package service

import (
	"context"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func (s *MemoryService) GetMemoryPlatformSettings(ctx context.Context, _ *v1.GetMemoryPlatformSettingsRequest) (*v1.MemoryPlatformSettings, error) {
	return s.memoryPlatformSettingsProto(ctx)
}

func (s *MemoryService) UpdateMemoryPlatformSettings(ctx context.Context, req *v1.UpdateMemoryPlatformSettingsRequest) (*v1.MemoryPlatformSettings, error) {
	if s.sysUC == nil {
		return nil, apierror.Unavailable("MEMORY", "system settings service not available")
	}
	_, err := s.sysUC.UpdateMemoryPlatform(ctx, biz.MemoryPlatformSetting{
		PolicyStrict:            req.GetPolicyStrict(),
		EpisodeBackfillDisabled: req.GetEpisodeBackfillDisabled(),
	})
	if err != nil {
		return nil, err
	}
	return s.memoryPlatformSettingsProto(ctx)
}

func (s *MemoryService) memoryPlatformSettingsProto(ctx context.Context) (*v1.MemoryPlatformSettings, error) {
	out := &v1.MemoryPlatformSettings{}
	envPS, envBF := biz.MemoryPlatformEnvOverrides()
	out.EnvPolicyStrictOverride = envPS
	out.EnvEpisodeBackfillDisabledOverride = envBF
	if s.sysUC != nil {
		row, err := s.sysUC.Get(ctx)
		if err == nil {
			out.PolicyStrict = row.MemoryPlatform.PolicyStrict
			out.EpisodeBackfillDisabled = row.MemoryPlatform.EpisodeBackfillDisabled
		}
	}
	return out, nil
}
