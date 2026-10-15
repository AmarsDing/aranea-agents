package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func (r *skillRepo) CreateSkillWithVersion(ctx context.Context, in biz.SkillCreateInput) (biz.Skill, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill name, slug and body are required")
	}
	skillID := fmt.Sprintf("skill_%d", time.Now().UTC().UnixNano())
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	metaJSON, err := encodeSkillMetadata(in.Tags, in.StorageDir, in.SyncOrigin, in.Triggers)
	if err != nil {
		return biz.Skill{}, err
	}
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.PlatformSkill.Create().
		SetID(skillID).
		SetSkillKey(in.Slug).
		SetName(in.Name).
		SetDescription(in.Description).
		SetStatus("draft").
		SetEnabled(false).
		SetSortOrder(0).
		SetConfigJSON("{}").
		SetMetadataJSON(string(metaJSON)).
		SetVisibility(in.Visibility).
		SetFallbackConfigJSON(in.DefaultConfigJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(in.WorkspaceID).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if _, err = tx.SkillVersion.Create().
		SetID(versionID).
		SetSkillID(skillID).
		SetVersion("1.0.0").
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(string(metaJSON)).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, skillID)
}

type skillMetadataEnvelope struct {
	Tags       []biz.SkillTag `json:"tags"`
	StorageDir string         `json:"storage_dir"`
	SyncOrigin string         `json:"sync_origin"`
	// Triggers 确定性触发词（P1-3），来自 SKILL.md frontmatter。
	Triggers []string `json:"triggers,omitempty"`
	// DerivedFrom records merge provenance (source skill IDs) for skills
	// created by merge_group_with_ai with retire_sources=true.
	DerivedFrom []string `json:"derived_from,omitempty"`
}

func encodeSkillMetadata(tags []biz.SkillTag, storageDir, syncOrigin string, triggers []string) (string, error) {
	md := skillMetadataEnvelope{
		Tags:       tags,
		StorageDir: strings.TrimSpace(storageDir),
		SyncOrigin: strings.TrimSpace(syncOrigin),
		Triggers:   normalizeSkillTriggers(triggers),
	}
	b, err := json.Marshal(md)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseSkillTriggers 从 metadata envelope 提取确定性触发词（P1-3）。
// 旧格式（无 triggers 字段）返回空切片。
func parseSkillTriggers(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var envelope struct {
		Triggers []string `json:"triggers"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}
	return normalizeSkillTriggers(envelope.Triggers)
}

// normalizeSkillTriggers 去空白、去空串、大小写不敏感去重。
func normalizeSkillTriggers(triggers []string) []string {
	if len(triggers) == 0 {
		return nil
	}
	out := make([]string, 0, len(triggers))
	seen := map[string]bool{}
	for _, t := range triggers {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func parseSkillMetadata(lg loggateway.Logger, raw string) skillMetadataEnvelope {
	var md skillMetadataEnvelope
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		lg.Warn("unmarshal skill metadata envelope failed", loggateway.StepID("data.skill"), loggateway.Err(err))
	}
	return md
}

func (r *skillRepo) PatchSkill(ctx context.Context, id string, patch biz.SkillUpdateDraft) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	e, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	now := nowRFC3339()

	if patch.HasBody {
		body := strings.TrimSpace(patch.Body)

		md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
		storageDir := strings.TrimSpace(md.StorageDir)
		if storageDir == "" {
			return biz.Skill{}, apierror.Internal(apierror.DomainSkill, "skill storage directory is not configured")
		}

		skillPath := filepath.Join(storageDir, "SKILL.md")
		if !isPathWithinRoot(storageDir, skillPath) {
			return biz.Skill{}, apierror.Internal(apierror.DomainSkill, "skill file path escapes storage directory")
		}

		tx, txErr := r.data.RW().Write(ctx).Tx(ctx)
		if txErr != nil {
			return biz.Skill{}, entErrToBizErr(txErr, apierror.DomainSkill)
		}
		defer func() { _ = tx.Rollback() }()

		upd := tx.PlatformSkill.UpdateOneID(id).SetUpdatedAt(now)
		if patch.HasName {
			upd.SetName(strings.TrimSpace(patch.Name))
		}
		if patch.HasDescription {
			upd.SetDescription(strings.TrimSpace(patch.Description))
		}
		if patch.HasTags || patch.HasTriggers {
			md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
			if patch.HasTags {
				md.Tags = normalizeSkillTags(patch.Tags)
			}
			if patch.HasTriggers {
				md.Triggers = normalizeSkillTriggers(patch.Triggers)
			}
			metaJSON, jerr := json.Marshal(md)
			if jerr != nil {
				return biz.Skill{}, jerr
			}
			upd.SetMetadataJSON(string(metaJSON))
		}
		if err := upd.Exec(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}

		sv, err := tx.SkillVersion.Query().
			Where(skillversion.SkillIDEQ(id)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			First(ctx)
		if err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		if _, err := tx.SkillVersion.UpdateOneID(sv.ID).
			SetContentMarkdown(body).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		if err = tx.Commit(); err != nil {
			return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
		}

		if writeErr := os.WriteFile(skillPath, []byte(body), 0o644); writeErr != nil {
			r.data.lg.Warn("PatchSkill: filesystem write failed after DB commit",
				loggateway.StepID("data.skill"),
				loggateway.Str("skill_id", id),
				loggateway.Str("path", skillPath),
				loggateway.Err(writeErr))
		}
	} else {

		txErr := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
			client := r.data.RW().Write(txCtx)
			upd := client.PlatformSkill.UpdateOneID(id).SetUpdatedAt(now)
			if patch.HasName {
				upd.SetName(strings.TrimSpace(patch.Name))
			}
			if patch.HasDescription {
				upd.SetDescription(strings.TrimSpace(patch.Description))
			}
			if patch.HasTags || patch.HasTriggers {
				md := parseSkillMetadata(r.data.lg, e.MetadataJSON)
				if patch.HasTags {
					md.Tags = normalizeSkillTags(patch.Tags)
				}
				if patch.HasTriggers {
					md.Triggers = normalizeSkillTriggers(patch.Triggers)
				}
				metaJSON, jerr := json.Marshal(md)
				if jerr != nil {
					return jerr
				}
				upd.SetMetadataJSON(string(metaJSON))
			}
			if err := upd.Exec(txCtx); err != nil {
				return entErrToBizErr(err, apierror.DomainSkill)
			}

			if patch.HasTags {
				sv, svErr := client.SkillVersion.Query().
					Where(skillversion.SkillIDEQ(id)).
					Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
					First(txCtx)
				if svErr != nil {
					if dataent.IsNotFound(svErr) {

						return nil
					}
					return entErrToBizErr(svErr, apierror.DomainSkill)
				}
				svMd := parseSkillMetadata(r.data.lg, sv.MetadataJSON)
				svMd.Tags = normalizeSkillTags(patch.Tags)
				svMetaJSON, jerr := json.Marshal(svMd)
				if jerr != nil {
					return jerr
				}
				if err := client.SkillVersion.UpdateOneID(sv.ID).
					SetMetadataJSON(string(svMetaJSON)).
					SetUpdatedAt(now).
					Exec(txCtx); err != nil {
					return entErrToBizErr(err, apierror.DomainSkill)
				}
			}
			return nil
		})
		if txErr != nil {
			return biz.Skill{}, txErr
		}
	}
	return r.GetSkillByID(ctx, id)
}

func (r *skillRepo) PublishSkill(ctx context.Context, id string, validationStatus string) (biz.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill id is required")
	}
	validationStatus = strings.TrimSpace(strings.ToLower(validationStatus))
	switch validationStatus {
	case "pass", "warn":
	default:
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "invalid validation_status for publish")
	}

	existing, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(id), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if existing.Status != "draft" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "only draft skills can be published")
	}

	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.PlatformSkill.UpdateOneID(id).
		SetStatus("published").
		SetUpdatedAt(now).
		Exec(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.NotFound(apierror.DomainSkill, "not found")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	sv, err := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(id)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "SKILL.md body is required")
		}
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	if sv == nil || strings.TrimSpace(sv.ContentMarkdown) == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "SKILL.md body is required")
	}
	if _, serr := tx.SkillVersion.UpdateOneID(sv.ID).
		SetPublishedAt(now).
		SetValidationStatus(validationStatus).
		SetUpdatedAt(now).
		Save(ctx); serr != nil {
		return biz.Skill{}, entErrToBizErr(serr, apierror.DomainSkill)
	}
	if err = tx.Commit(); err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, id)
}
