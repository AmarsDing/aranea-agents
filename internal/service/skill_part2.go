package service

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"os"
	"path/filepath"
	"strings"
)

func (s *SkillService) skillDir(ctx context.Context, id string) (string, error) {
	dir, err := s.uc.GetStorageDir(ctx, id)
	if err != nil || strings.TrimSpace(dir) == "" {
		current, getErr := s.uc.Get(ctx, id)
		if getErr != nil {
			if err != nil {
				return "", err
			}
			return "", getErr
		}
		dir = filepath.Join(s.fs.ResolveRoot(ctx), current.Slug)
		s.lg.Warn("skillDir fallback: GetStorageDir failed or returned empty, using slug-based path",
			loggateway.StepID("service.skill"),
			loggateway.Str("skill_id", id),
			loggateway.Err(err),
			loggateway.Str("fallback_dir", dir))
	}
	if !s.fs.DirExists(dir) {
		return "", os.ErrNotExist
	}
	return dir, nil
}
