package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *skillRepo) ListRegisteredSlugs(ctx context.Context) ([]string, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if slug := strings.TrimSpace(row.SkillKey); slug != "" {
			out = append(out, slug)
		}
	}
	return out, nil
}

func (r *skillRepo) ListEnabledPublishedSkillKeys(ctx context.Context) ([]string, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		Select(platformskill.FieldSkillKey).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.SkillKey)
	}
	return out, nil
}

// ListEnabledPublishedSkillRefs returns slug + updated_at for every enabled
// published skill. updated_at is bumped by every data-layer skill mutation,
// so consumers can build a content-based cache-key hash that changes exactly
// when skill content changes. Only two columns are selected: this runs on the
// per-request agent-build hot path.
func (r *skillRepo) ListEnabledPublishedSkillRefs(ctx context.Context) ([]biz.SkillEnabledRef, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		Select(platformskill.FieldSkillKey, platformskill.FieldUpdatedAt).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]biz.SkillEnabledRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.SkillEnabledRef{Slug: row.SkillKey, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

func (r *skillRepo) ListEnabledPublishedSkillCandidates(ctx context.Context) ([]biz.SkillRuntimeCandidate, error) {
	rows, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.EnabledEQ(true),
			platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")),
		).
		Order(platformskill.BySkillKey(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	out := make([]biz.SkillRuntimeCandidate, 0, len(rows))
	for _, row := range rows {
		tags := parseSkillTags(row.MetadataJSON)
		if len(tags) == 0 {
			tags = parseSkillTags(row.ConfigJSON)
		}
		// P0-7 批 D1：信任分级两列透出（含 NULL 防御——迁移 20261297 回填后
		// 生产不应再见 NULL；nil 按零值 "" 透出，biz 侧经
		// DefaultTrustForSource 归一为存量默认 T3，与门禁上线前行为一致）。
		trustLevel := ""
		if row.TrustLevel != nil {
			trustLevel = *row.TrustLevel
		}
		source := ""
		if row.Source != nil {
			source = *row.Source
		}
		out = append(out, biz.SkillRuntimeCandidate{

			SkillID:       row.ID,
			Slug:          row.SkillKey,
			Name:          row.Name,
			Description:   row.Description,
			Tags:          tags,
			TaxonomyPaths: mergeTaxonomyPaths(r.data.lg, row.MetadataJSON, row.ConfigJSON),
			Triggers:      parseSkillTriggers(row.MetadataJSON),
			TrustLevel:    trustLevel,
			Source:        source,
		})
	}
	return out, nil
}

func (r *skillRepo) GetLatestSkillMarkdown(ctx context.Context, skillID string) (string, error) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return "", apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	sv, err := r.data.RW().Read(ctx).SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return "", apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return "", entErrToBizErr(err, apierror.DomainSkill)
	}
	return sv.ContentMarkdown, nil
}

func (r *skillRepo) BatchGetSkillMarkdownBySlugs(ctx context.Context, slugs []string) (map[string]string, error) {
	if len(slugs) == 0 {
		return map[string]string{}, nil
	}
	skills, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.SkillKeyIn(slugs...), platformskill.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	skillIDs := make([]string, 0, len(skills))
	keyToID := make(map[string]string, len(skills))
	for _, s := range skills {
		skillIDs = append(skillIDs, s.ID)
		keyToID[s.SkillKey] = s.ID
	}
	if len(skillIDs) == 0 {
		return map[string]string{}, nil
	}
	rows, err := r.data.RW().Read(ctx).SkillVersion.Query().
		Where(skillversion.SkillIDIn(skillIDs...)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	idToMarkdown := make(map[string]string, len(skillIDs))
	seen := make(map[string]bool, len(skillIDs))
	for _, sv := range rows {
		if seen[sv.SkillID] {
			continue
		}
		seen[sv.SkillID] = true
		idToMarkdown[sv.SkillID] = sv.ContentMarkdown
	}
	out := make(map[string]string, len(slugs))
	for key, id := range keyToID {
		if md, ok := idToMarkdown[id]; ok {
			out[key] = md
		}
	}
	return out, nil
}
