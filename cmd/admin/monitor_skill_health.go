package main

import (
	"context"

	"aranea-agents/internal/biz"
)

type monitorSkillHealthAdapter struct {
	skills *biz.SkillUsecase
}

func (a monitorSkillHealthAdapter) FilesystemHealthStats(ctx context.Context) (int, int, error) {
	if a.skills == nil {
		return 0, 0, nil
	}
	stats, err := a.skills.FilesystemHealthStats(ctx)
	if err != nil {
		return 0, 0, err
	}
	return stats.MissingCount, stats.PendingFilesystemCount, nil
}
