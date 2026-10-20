package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *skillRepo) ListSkillVersions(ctx context.Context, q biz.SkillVersionListQuery) (biz.SkillVersionListResult, error) {
	c := r.data.RW().Read(ctx)
	exists, err := c.PlatformSkill.Query().
		Where(platformskill.IDEQ(q.SkillID), platformskill.DeletedAtEQ("")).
		Exist(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if !exists {
		return biz.SkillVersionListResult{}, apierror.NotFound(apierror.DomainSkill, "not found")
	}
	count, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		Count(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := c.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(q.SkillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		Offset(q.Offset).
		Limit(q.Limit).
		All(ctx)
	if err != nil {
		return biz.SkillVersionListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	items := make([]biz.SkillVersionDetail, 0, len(rows))
	for _, row := range rows {
		items = append(items, entSkillVersionToBiz(row))
	}
	return biz.SkillVersionListResult{
		Items:  items,
		Total:  count,
		Limit:  q.Limit,
		Offset: q.Offset,
	}, nil
}

func (r *skillRepo) RollbackSkillVersion(ctx context.Context, skillID string, versionID string) (biz.Skill, error) {
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	skillRow, err := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(skillID), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	if skillRow.Status != "published" && skillRow.Status != "active" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only published skills can be rolled back")
	}

	targetVer, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID), skillversion.IDEQ(versionID)).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	if _, err := tx.SkillVersion.Create().
		SetID(newVerID).
		SetSkillID(skillID).
		SetVersion(incrementVersion(targetVer.Version)).
		SetStatus("pass").
		SetContentMarkdown(targetVer.ContentMarkdown).
		SetMetadataJSON(targetVer.MetadataJSON).
		SetManifestJSON(targetVer.ManifestJSON).
		SetFileManifestJSON(targetVer.FileManifestJSON).
		SetValidationStatus("pass").
		SetPublishedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if _, err := tx.PlatformSkill.UpdateOneID(skillID).
		SetStatus("published").
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, skillID)
}

// CreateSkillVersion appends an evolved version to an existing skill.
// "Current" resolves by created_at DESC (see GetLatestSkillMarkdown), so
// inserting the row activates it — no platform_skill pointer to switch.
// Version metadata/manifests are inherited from the latest existing version;
// the parent anchor is explicit or falls back to that same latest version.
func (r *skillRepo) CreateSkillVersion(ctx context.Context, in biz.SkillCreateVersionInput) (biz.SkillVersionDetail, error) {
	in.SkillID = strings.TrimSpace(in.SkillID)
	in.Body = strings.TrimSpace(in.Body)
	if in.SkillID == "" || in.Body == "" {
		return biz.SkillVersionDetail{}, apierror.BadRequest(apierror.DomainSkill, "skill id and body are required")
	}
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	latest, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(in.SkillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.SkillVersionDetail{}, apierror.NotFound(apierror.DomainSkill, "no existing version for skill: %s", in.SkillID)
		}
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	parentID := strings.TrimSpace(in.ParentVersionID)
	if parentID == "" {
		parentID = latest.ID
	}
	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	row, err := tx.SkillVersion.Create().
		SetID(newVerID).
		SetSkillID(in.SkillID).
		SetVersion(incrementVersion(latest.Version)).
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(latest.MetadataJSON).
		SetManifestJSON(latest.ManifestJSON).
		SetFileManifestJSON(latest.FileManifestJSON).
		SetParentVersionID(parentID).
		SetEvolutionReason(in.EvolutionReason).
		SetValidationStatus("pass").
		SetPublishedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.SkillVersionDetail{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	r.syncSkillBodyToDisk(ctx, in.SkillID, in.Body)
	return entSkillVersionToBiz(row), nil
}

func entSkillVersionToBiz(row *dataent.SkillVersion) biz.SkillVersionDetail {
	return biz.SkillVersionDetail{
		ID:               row.ID,
		SkillID:          row.SkillID,
		Version:          row.Version,
		Status:           row.Status,
		ContentMarkdown:  row.ContentMarkdown,
		ValidationStatus: row.ValidationStatus,
		PublishedAt:      row.PublishedAt,
		CreatedAt:        row.CreatedAt,
		FileManifestJSON: row.FileManifestJSON,
	}
}

func incrementVersion(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return v + "+rollback"
	}
	patch := 0
	if n, err := fmt.Sscanf(parts[2], "%d", &patch); err == nil && n == 1 {
		return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
	}
	return v + "+rollback"
}
