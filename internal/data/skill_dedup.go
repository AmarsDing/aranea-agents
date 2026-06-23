package data

import (
	"context"

	entsql "entgo.io/ent/dialect/sql"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/platformskill"
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
)

func NewSkillDedupRepo(data *Data, lg loggateway.Logger) *SkillDedupRepo {
	return &SkillDedupRepo{data: data, lg: lg}
}

// ListAllSkillSummaries returns all enabled, non-deleted skills with a body preview and tags.
func (r *SkillDedupRepo) ListAllSkillSummaries(ctx context.Context) ([]biz.SkillSummary, error) {
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.EnabledEQ(true),
			platformskill.DeletedAtEQ(""),
		).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL_DEDUP")
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
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			All(ctx)
		if vErr != nil {
			r.lg.Warn("skill_dedup: failed to fetch skill versions, body previews will be empty",
				loggateway.StepID("skill_dedup.list_summaries"), loggateway.Err(vErr))
		} else {
			// Keep only the latest version per skill_id (first one after created_at DESC order).
			for _, v := range versions {
				if _, exists := bodyMap[v.SkillID]; !exists {
					bodyMap[v.SkillID] = v.ContentMarkdown
				}
			}
		}
	}

	result := make([]biz.SkillSummary, 0, len(skills))
	for _, s := range skills {
		preview := bodyMap[s.ID]
		if len(preview) > bodyPreviewLen {
			preview = preview[:bodyPreviewLen]
		}
		// Extract tags from metadata_json.
		tagNames := extractTagNames(parseSkillTags(s.MetadataJSON))
		result = append(result, biz.SkillSummary{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.SkillKey,
			Description: s.Description,
			BodyPreview: preview,
			Tags:        tagNames,
		})
	}
	return result, nil
}

// extractTagNames extracts tag name strings from SkillTag slice.
func extractTagNames(tags []biz.SkillTag) []string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names
}
