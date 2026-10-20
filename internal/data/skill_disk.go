package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *skillRepo) ListSkillSimilaritySources(ctx context.Context) ([]biz.SkillSimilaritySource, error) {
	c := r.data.RW().Read(ctx)
	rows, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ("")).
		Order(platformskill.ByUpdatedAt(entsql.OrderDesc()), platformskill.ByCreatedAt(entsql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}

	skillIDs := make([]string, 0, len(rows))
	for _, s := range rows {
		skillIDs = append(skillIDs, s.ID)
	}
	versionMap := make(map[string]*dataent.SkillVersion, len(skillIDs))
	if len(skillIDs) > 0 {
		versions, vErr := c.SkillVersion.Query().
			Where(skillversion.SkillIDIn(skillIDs...)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
			All(ctx)
		if vErr != nil {
			return nil, entErrToBizErr(vErr, apierror.DomainSkill)
		}
		for _, v := range versions {
			if _, exists := versionMap[v.SkillID]; !exists {
				versionMap[v.SkillID] = v
			}
		}
	}

	out := make([]biz.SkillSimilaritySource, 0, len(rows))
	for _, s := range rows {
		item := biz.SkillSimilaritySource{
			ID:          s.ID,
			Name:        s.Name,
			Slug:        s.SkillKey,
			Description: s.Description,
		}
		if sv, ok := versionMap[s.ID]; ok {
			item.Version = sv.Version
			item.Body = sv.ContentMarkdown
			item.BodyPreview = previewSkillBody(item.Body, 240)
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *skillRepo) UpsertSkillFromDisk(ctx context.Context, in biz.SkillDiskSyncInput) (biz.Skill, biz.SkillDiskSyncOutcome, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, apierror.BadRequest(apierror.DomainSkill, "skill name, slug and body are required")
	}

	tx, txErr := r.data.RW().Write(ctx).Tx(ctx)
	if txErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(txErr, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	skillRow, err := tx.PlatformSkill.Query().
		Where(platformskill.SkillKeyEQ(in.Slug), platformskill.DeletedAtEQ("")).
		Only(ctx)

	if dataent.IsNotFound(err) {
		skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
		versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
		metaJSON, encErr := encodeSkillMetadata(in.Tags, in.StorageDir, biz.SkillSyncOriginFilesystem, in.Triggers)
		if encErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, encErr
		}
		now := nowRFC3339()
		if _, createErr := tx.PlatformSkill.Create().
			SetID(skillID).
			SetSkillKey(in.Slug).
			SetName(in.Name).
			SetDescription(in.Description).
			SetStatus("draft").
			SetEnabled(false).
			SetSortOrder(0).
			SetConfigJSON("{}").
			SetMetadataJSON(string(metaJSON)).
			SetVisibility("").
			SetCreatedAt(now).
			SetUpdatedAt(now).
			SetDeletedAt("").
			Save(ctx); createErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
		}
		if _, createErr := tx.SkillVersion.Create().
			SetID(versionID).
			SetSkillID(skillID).
			SetVersion("1.0.0").
			SetStatus("pass").
			SetContentMarkdown(in.Body).
			SetMetadataJSON(string(metaJSON)).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); createErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(commitErr, apierror.DomainSkill)
		}
		sk, getErr := r.GetSkillByID(ctx, skillID)
		return sk, biz.SkillDiskSyncOutcome{}, getErr
	}
	if err != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	outcome := biz.SkillDiskSyncOutcome{}
	wasPublished := skillRow.Status == "published" || skillRow.Status == "active"
	now := nowRFC3339()
	metaJSON, encErr := encodeSkillMetadata(in.Tags, in.StorageDir, biz.SkillSyncOriginFilesystem, in.Triggers)
	if encErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, encErr
	}

	if derived := parseSkillMetadata(r.data.lg, skillRow.MetadataJSON).DerivedFrom; len(derived) > 0 {
		md := parseSkillMetadata(r.data.lg, metaJSON)
		md.DerivedFrom = derived
		if merged, mErr := json.Marshal(md); mErr == nil {
			metaJSON = string(merged)
		}
	}
	update := tx.PlatformSkill.UpdateOneID(skillRow.ID).
		SetName(in.Name).
		SetDescription(in.Description).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(now).
		SetFilesystemMissing(false)

	rowChanged := skillRow.Name != in.Name ||
		skillRow.Description != in.Description ||
		skillRow.MetadataJSON != string(metaJSON) ||
		skillRow.FilesystemMissing

	sv, svErr := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillRow.ID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc()), skillversion.ByID(entsql.OrderDesc())).
		First(ctx)
	if svErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(svErr, apierror.DomainSkill)
	}
	// refreshDiskBody 非空表示命中 F3 进化版保护：提交后以 DB 内容刷新磁盘。
	var refreshDiskBody string
	if strings.TrimSpace(sv.ContentMarkdown) != in.Body {
		if strings.TrimSpace(sv.EvolutionReason) != "" {

			refreshDiskBody = sv.ContentMarkdown
		} else {
			outcome.ContentChanged = true

			newVerID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
			newVerMetaJSON := string(metaJSON)
			if _, createErr := tx.SkillVersion.Create().
				SetID(newVerID).
				SetSkillID(skillRow.ID).
				SetVersion(incrementVersion(sv.Version)).
				SetStatus("pass").
				SetContentMarkdown(in.Body).
				SetMetadataJSON(newVerMetaJSON).
				SetManifestJSON(sv.ManifestJSON).
				SetFileManifestJSON(sv.FileManifestJSON).
				SetCreatedAt(now).
				SetUpdatedAt(now).
				Save(ctx); createErr != nil {
				return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(createErr, apierror.DomainSkill)
			}
		}
	}
	if outcome.ContentChanged && wasPublished {
		outcome.RevertedToDraft = true
		update = update.SetStatus("draft").SetEnabled(false)
	}
	if rowChanged || outcome.ContentChanged {
		if _, updateErr := update.Save(ctx); updateErr != nil {
			return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(updateErr, apierror.DomainSkill)
		}
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return biz.Skill{}, biz.SkillDiskSyncOutcome{}, entErrToBizErr(commitErr, apierror.DomainSkill)
	}
	if refreshDiskBody != "" {
		r.data.lg.Warn("skill disk sync: stale disk content blocked by evolution version, refreshing disk from DB",
			loggateway.StepID("data.skill"),
			loggateway.Str("skill_id", skillRow.ID),
			loggateway.Str("version_id", sv.ID),
			loggateway.Str("evolution_reason", sv.EvolutionReason))
		writeSkillBodyToDisk(r.data.lg, in.StorageDir, refreshDiskBody)
	}
	sk, getErr := r.GetSkillByID(ctx, skillRow.ID)
	return sk, outcome, getErr
}

func (r *skillRepo) MarkSkillFilesystemMissing(ctx context.Context, slug string, missing bool) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return apierror.BadRequest(apierror.DomainSkill, "skill slug is required")
	}

	n, err := r.data.RW().Write(ctx).PlatformSkill.Update().
		Where(
			platformskill.SkillKeyEQ(slug),
			platformskill.DeletedAtEQ(""),
			platformskill.FilesystemMissingEQ(!missing),
		).
		SetFilesystemMissing(missing).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if n == 0 {

		exist, qErr := r.data.RW().Read(ctx).PlatformSkill.Query().
			Where(platformskill.SkillKeyEQ(slug), platformskill.DeletedAtEQ("")).
			Exist(ctx)
		if qErr != nil {
			return entErrToBizErr(qErr, apierror.DomainSkill)
		}
		if !exist {
			return apierror.NotFound(apierror.DomainSkill, "not found")
		}
	}
	return nil
}

func (r *skillRepo) FilesystemHealthStats(ctx context.Context) (biz.SkillFilesystemHealthStats, error) {
	c := r.data.RW().Read(ctx)
	missing, err := c.PlatformSkill.Query().
		Where(platformskill.DeletedAtEQ(""), platformskill.FilesystemMissingEQ(true)).
		Count(ctx)
	if err != nil {
		return biz.SkillFilesystemHealthStats{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	pending, err := c.PlatformSkill.Query().
		Where(
			platformskill.DeletedAtEQ(""),
			platformskill.StatusEQ("draft"),
			platformskill.FilesystemMissingEQ(false),
			platformskill.MetadataJSONContains(`"sync_origin":"filesystem"`),
		).
		Count(ctx)
	if err != nil {
		return biz.SkillFilesystemHealthStats{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return biz.SkillFilesystemHealthStats{
		MissingCount:           missing,
		PendingFilesystemCount: pending,
	}, nil
}
