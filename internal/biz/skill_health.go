package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz/types"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// SkillHealthReader aggregates skill invocation data to compute health metrics.
type SkillHealthReader interface {
	GetSkillHealth(ctx context.Context, skillID string, since7d, since30d time.Time) (*types.SkillHealthDetail, error)
}

// SkillHealthUsecase provides skill health metrics for the GetSkillHealth API.
type SkillHealthUsecase struct {
	repo SkillHealthReader
	lg   loggateway.Logger
}

// NewSkillHealthUsecase constructs a SkillHealthUsecase.
func NewSkillHealthUsecase(repo SkillHealthReader, lg loggateway.Logger) *SkillHealthUsecase {
	return &SkillHealthUsecase{repo: repo, lg: lg}
}

// GetSkillHealth returns health metrics for a single skill over 7d and 30d windows.
func (uc *SkillHealthUsecase) GetSkillHealth(ctx context.Context, skillID string) (*types.SkillHealthDetail, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return nil, kerrors.BadRequest("SKILL_INTELLIGENCE", "skill_id is required")
	}
	now := time.Now().UTC()
	since7d := now.Add(-7 * 24 * time.Hour)
	since30d := now.Add(-30 * 24 * time.Hour)
	return uc.repo.GetSkillHealth(ctx, skillID, since7d, since30d)
}
