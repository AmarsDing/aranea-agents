package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

// skill_import_repo.go implements biz.SkillImportWriter on skillRepo: the
// ZIP-import overwrite (overwrite_duplicate) and merge-retire paths. Kept in
// a separate file from skill.go per the import-engine change plan.

// AppendImportedVersion appends a new version to an existing skill (parent =
// current latest, patch version incremented) and refreshes the skill row
// name/description/tags — all in one transaction. Used by the import
// overwrite_duplicate decision.
func (r *skillRepo) AppendImportedVersion(ctx context.Context, in biz.SkillImportVersionInput) (biz.Skill, error) {
	in.SkillID = strings.TrimSpace(in.SkillID)
	in.Body = strings.TrimSpace(in.Body)
	if in.SkillID == "" || in.Body == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id and body are required")
	}
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	skillRow, err := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(in.SkillID), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	latest, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(in.SkillID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	// Refresh the skill row: name/description + tags in the metadata envelope
	// (storage_dir / sync_origin / derived_from preserved).
	md := parseSkillMetadata(r.data.lg, skillRow.MetadataJSON)
	md.Tags = in.Tags
	metaJSON, err := json.Marshal(md)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if _, err = tx.PlatformSkill.UpdateOneID(in.SkillID).
		SetName(strings.TrimSpace(in.Name)).
		SetDescription(strings.TrimSpace(in.Description)).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	newVerID := fmt.Sprintf("sv_%d", time.Now().UTC().UnixNano())
	if _, err = tx.SkillVersion.Create().
		SetID(newVerID).
		SetSkillID(in.SkillID).
		SetVersion(incrementVersion(latest.Version)).
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(string(metaJSON)).
		SetManifestJSON(latest.ManifestJSON).
		SetFileManifestJSON(latest.FileManifestJSON).
		SetParentVersionID(latest.ID).
		SetEvolutionReason("import overwrite").
		SetValidationStatus("pass").
		SetPublishedAt(now).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, in.SkillID)
}

// ArchiveSkill retires a skill: status=archived, enabled=false. Used by
// merge_group_with_ai with retire_sources=true to retire the source skills.
func (r *skillRepo) ArchiveSkill(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	if err := r.data.RW().Write(ctx).PlatformSkill.UpdateOneID(id).
		SetStatus("archived").
		SetEnabled(false).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx); err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	return nil
}

// SetSkillDerivedFrom records merge provenance (source skill IDs) in the
// skill's metadata JSON under derived_from.
func (r *skillRepo) SetSkillDerivedFrom(ctx context.Context, id string, sourceIDs []string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
	md.DerivedFrom = append([]string(nil), sourceIDs...)
	metaJSON, err := json.Marshal(md)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if err := r.data.RW().Write(ctx).PlatformSkill.UpdateOneID(id).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx); err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	return nil
}
