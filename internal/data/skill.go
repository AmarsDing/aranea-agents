package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizskill "aranea-agents/internal/biz/skill"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type skillRepo struct {
	data *Data
}

var _ bizskill.Repo = (*skillRepo)(nil)

// NewSkillRepo implements biz.SkillRepo via Ent (pkg-backend-to-kratos §2.2 — only entClient, no parallel sqlite pool).
func NewSkillRepo(d *Data) biz.SkillRepo {
	return &skillRepo{data: d}
}

func skillListPredicates(q biz.SkillListQuery) []predicate.PlatformSkill {
	ps := []predicate.PlatformSkill{platformskill.DeletedAtEQ("")}

	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		ps = append(ps, platformskill.Or(
			platformskill.WorkspaceIDEQ(""),
			platformskill.WorkspaceIDEQ(ws),
		))
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		ps = append(ps, platformskill.Or(
			platformskill.SkillKeyContainsFold(s),
			platformskill.NameContainsFold(s),
			platformskill.DescriptionContainsFold(s),
		))
	}
	if q.Tags != "" {
		for _, tag := range strings.Split(q.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			ps = append(ps, platformskill.MetadataJSONContainsFold(tag))
		}
	}
	if q.Enabled == "true" {
		ps = append(ps, platformskill.EnabledEQ(true))
	}
	if q.Enabled == "false" {
		ps = append(ps, platformskill.EnabledEQ(false))
	}
	if q.Status != "" {
		if q.Status == "published" {
			ps = append(ps, platformskill.Or(platformskill.StatusEQ("published"), platformskill.StatusEQ("active")))
		} else {
			ps = append(ps, platformskill.StatusEQ(q.Status))
		}
	}
	if q.FilesystemMissing == "true" {
		ps = append(ps, platformskill.FilesystemMissingEQ(true))
	}
	if q.FilesystemMissing == "false" {
		ps = append(ps, platformskill.FilesystemMissingEQ(false))
	}
	if origin := strings.TrimSpace(q.SyncOrigin); origin != "" {
		ps = append(ps, platformskill.MetadataJSONContains(`"sync_origin":"`+escapeJSONStringValue(origin)+`"`))
	}
	return ps
}

func normalizeSkillStatus(status string) string {
	switch status {
	case "", "active", "published":
		return "published"
	case "inactive":
		return "archived"
	default:
		return status
	}
}

func coalesceTime(startedAt, createdAt string) string {
	if strings.TrimSpace(startedAt) != "" {
		return startedAt
	}
	return createdAt
}

// skillListOrder 构造列表排序。生产唯一驱动为 Postgres（NewData 硬编码），
// tag 排序依赖 jsonb 提取 metadata_json 中首个标签名（envelope 与裸数组两种历史格式）。
func skillListOrder(q biz.SkillListQuery) []platformskill.OrderOption {
	dir := "ASC"
	if strings.EqualFold(strings.TrimSpace(q.SortOrder), "desc") {
		dir = "DESC"
	}
	switch strings.TrimSpace(q.SortBy) {
	case "name":
		return []platformskill.OrderOption{
			func(s *entsql.Selector) {
				s.OrderBy("LOWER(" + s.C(platformskill.FieldName) + ") " + dir)
				s.OrderBy(entsql.Asc(s.C(platformskill.FieldID)))
			},
		}
	case "tag":
		return []platformskill.OrderOption{
			func(s *entsql.Selector) {
				firstTag := "COALESCE(" +
					s.C(platformskill.FieldMetadataJSON) + "::jsonb -> 'tags' -> 0 ->> 'name', " +
					s.C(platformskill.FieldMetadataJSON) + "::jsonb -> 0 ->> 'name')"
				s.OrderBy("LOWER(" + firstTag + ") " + dir + " NULLS LAST")
				s.OrderBy("LOWER(" + s.C(platformskill.FieldName) + ") " + dir)
				s.OrderBy(entsql.Asc(s.C(platformskill.FieldID)))
			},
		}
	default:
		return []platformskill.OrderOption{
			platformskill.ByUpdatedAt(entsql.OrderDesc()),
			platformskill.ByCreatedAt(entsql.OrderDesc()),
		}
	}
}

func (r *skillRepo) SearchSkills(ctx context.Context, q biz.SkillListQuery) (biz.SkillListResult, error) {
	c := r.data.RW().Read(ctx)
	preds := skillListPredicates(q)
	total, err := c.PlatformSkill.Query().Where(preds...).Count(ctx)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := c.PlatformSkill.Query().
		Where(preds...).
		Order(skillListOrder(q)...).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	items, err := r.batchEnrichSkills(ctx, rows)
	if err != nil {
		return biz.SkillListResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return biz.SkillListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *skillRepo) GetSkillByID(ctx context.Context, id string) (biz.Skill, error) {
	preds := []predicate.PlatformSkill{
		platformskill.IDEQ(id),
		platformskill.DeletedAtEQ(""),
	}
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		preds = append(preds, platformskill.WorkspaceIDIn(ids...))
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.enrichSkill(ctx, e)
}

func (r *skillRepo) UpdateSkillEnabled(ctx context.Context, id string, enabled bool) (biz.Skill, error) {
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	if enabled {
		existing, err := r.data.RW().Read(ctx).PlatformSkill.Query().
			Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
			Only(ctx)
		if err != nil {
			if dataent.IsNotFound(err) {
				return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
			}
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		status := strings.TrimSpace(strings.ToLower(existing.Status))
		if status != "published" && status != "active" {
			return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only published skills can be enabled")
		}
	}
	err := r.data.RW().Write(ctx).PlatformSkill.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) DuplicateSkill(ctx context.Context, id string) (biz.Skill, error) {
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	cur, err := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	latestVer, verErr := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if verErr != nil && !dataent.IsNotFound(verErr) {
		return biz.Skill{}, entErrToBizErr(verErr, apierror.DomainSkill)
	}
	newID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	newKey := fmt.Sprintf("%s-copy-%d", cur.SkillKey, time.Now().UTC().Unix())
	if strings.TrimSpace(cur.SkillKey) == "" {
		newKey = newID
	}
	now := nowRFC3339()
	if _, err = tx.PlatformSkill.Create().
		SetID(newID).
		SetSkillKey(newKey).
		SetName(cur.Name + " Copy").
		SetDescription(cur.Description).
		SetStatus("draft").
		SetEnabled(false).
		SetSortOrder(0).
		SetConfigJSON(cur.ConfigJSON).
		SetMetadataJSON(cur.MetadataJSON).
		SetVisibility(cur.Visibility).
		SetFallbackConfigJSON(cur.FallbackConfigJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(cur.WorkspaceID).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if latestVer != nil {
		verID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
		if _, err = tx.SkillVersion.Create().
			SetID(verID).
			SetSkillID(newID).
			SetVersion("1.0.0").
			SetStatus("pass").
			SetContentMarkdown(latestVer.ContentMarkdown).
			SetMetadataJSON(latestVer.MetadataJSON).
			SetManifestJSON(latestVer.ManifestJSON).
			SetFileManifestJSON(latestVer.FileManifestJSON).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, newID)
}

func (r *skillRepo) DeleteSkill(ctx context.Context, id string) error {
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()

	if _, vErr := tx.SkillVersion.Delete().
		Where(skillversion.SkillIDEQ(id)).
		Exec(ctx); vErr != nil {
		return entErrToBizErr(vErr, apierror.DomainSkill)
	}

	n, err := tx.PlatformSkill.Delete().
		Where(platformskill.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if n == 0 {
		return apierror.NotFound(apierror.DomainSkill, "skill not found")
	}

	return entErrToBizErr(tx.Commit(), apierror.DomainSkill)
}

func (r *skillRepo) GetSkillStorageDir(ctx context.Context, id string) (string, error) {
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return "", entErrToBizErr(err, apierror.DomainSkill)
	}
	var metadata struct {
		StorageDir string `json:"storage_dir"`
	}
	if err := json.Unmarshal([]byte(e.MetadataJSON), &metadata); err != nil {
		r.data.lg.Warn("unmarshal skill metadata failed", loggateway.StepID("data.skill"), loggateway.Err(err))
		return "", err
	}
	if strings.TrimSpace(metadata.StorageDir) == "" {
		return "", apierror.Internal(apierror.DomainSkill, "skill storage directory is not configured")
	}
	return metadata.StorageDir, nil
}

func (r *skillRepo) GetSkillBySkillKey(ctx context.Context, skillKey string) (biz.Skill, error) {
	skillKey = strings.TrimSpace(skillKey)
	if skillKey == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill key is required")
	}
	preds := []predicate.PlatformSkill{
		platformskill.SkillKeyEQ(skillKey),
		platformskill.DeletedAtEQ(""),
	}
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		preds = append(preds, platformskill.WorkspaceIDIn(ids...))
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(preds...).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.enrichSkill(ctx, e)
}
