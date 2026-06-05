package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/loggateway"
)

const bodyPreviewLen = 500

type SkillDedupRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.SkillDedupReader = (*SkillDedupRepo)(nil)
	_ biz.SkillDedupWriter = (*SkillDedupRepo)(nil)
)

func NewSkillDedupRepo(data *Data, lg loggateway.Logger) *SkillDedupRepo {
	return &SkillDedupRepo{data: data, lg: lg}
}

// ListAllSkillSummaries returns all enabled, non-deleted skills with a body preview.
func (r *SkillDedupRepo) ListAllSkillSummaries(ctx context.Context) ([]biz.SkillSummary, error) {
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.EnabledEQ(true),
			platformskill.DeletedAtEQ(""),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// Batch-fetch latest published skill versions for body previews.
	skillIDs := make([]string, 0, len(skills))
	for _, s := range skills {
		skillIDs = append(skillIDs, s.ID)
	}

	bodyMap := make(map[string]string, len(skillIDs))
	if len(skillIDs) > 0 {
		versions, vErr := r.data.RW().Read(ctx).SkillVersion.Query().
			Where(
				skillversion.SkillIDIn(skillIDs...),
				skillversion.StatusEQ("published"),
			).
			All(ctx)
		if vErr != nil {
			r.lg.Warn("skill_dedup: failed to fetch skill versions, body previews will be empty",
				loggateway.StepID("skill_dedup.list_summaries"), loggateway.Err(vErr))
		} else {
			// Keep only the latest version per skill_id (last one by created_at order).
			for _, v := range versions {
				bodyMap[v.SkillID] = v.ContentMarkdown
			}
		}
	}

	result := make([]biz.SkillSummary, 0, len(skills))
	for _, s := range skills {
		preview := bodyMap[s.ID]
		if len(preview) > bodyPreviewLen {
			preview = preview[:bodyPreviewLen]
		}
		result = append(result, biz.SkillSummary{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.SkillKey,
			Description: s.Description,
			BodyPreview: preview,
		})
	}
	return result, nil
}

// DeprecateSkill marks a skill as disabled with the given reason.
func (r *SkillDedupRepo) DeprecateSkill(ctx context.Context, skillID string, reason string) error {
	_, err := r.data.RW().Write(ctx).PlatformSkill.UpdateOneID(skillID).
		SetEnabled(false).
		SetStatus("deprecated").
		Save(ctx)
	return err
}

// TransferInvocations updates skill_invocation records from one skill to another.
func (r *SkillDedupRepo) TransferInvocations(ctx context.Context, fromSkillID string, toSkillID string) error {
	_, err := r.data.RW().Write(ctx).SkillInvocation.Update().
		Where(skillinvocation.SkillIDEQ(fromSkillID)).
		SetSkillID(toSkillID).
		Save(ctx)
	return err
}
