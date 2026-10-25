package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizskill "aranea-agents/internal/biz/skill"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"
)

func (r *skillRepo) CreateSkillWithVersion(ctx context.Context, in biz.SkillCreateInput) (biz.Skill, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || in.Slug == "" || in.Body == "" {
		return biz.Skill{}, apierror.BadRequest(apierror.DomainSkill, "skill name, slug and body are required")
	}
	metaJSON, err := encodeSkillMetadata(in.Tags, in.StorageDir, in.SyncOrigin, in.Triggers)
	if err != nil {
		return biz.Skill{}, err
	}
	// P0-7 批 D1：信任分级两列落值。manifest 声明只能更严、绝不覆盖
	// provenance（§8.4③⑤ fail-closed）。
	mf := manifest.Parse(in.Body)
	source, trust := biz.ResolveSkillTrustForPersist(in.SyncOrigin, mf.Source, mf.TrustLevel)
	var skillID string
	err = retrySkillIDConflict(func() error {
		var onceErr error
		skillID, onceErr = r.createSkillWithVersionOnce(ctx, in, metaJSON, string(source), string(trust))
		return onceErr
	})
	if err != nil {
		return biz.Skill{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	return r.GetSkillByID(ctx, skillID)
}

// createSkillWithVersionOnce 单次建仓尝试（返回原始错误供 23505 判定，翻译
// 统一在调用方）。skill 行 id 走 bizskill.NewSkillID（128bit ULID 语义随机段
// 替换旧 UnixNano——纳秒戳在同进程高并发下仍可见尾数碰撞面）；skillver id
// 维持存量 skillver_%d 不动。pk 撞号重试=整事务重开（25P02 铁律：出错后旧
// 事务已 abort，必须 Rollback 回收到位再 Begin 新事务），由 retrySkillIDConflict
// 驱动且仅认 skill 主键（skill_key 冲突 23505 属真实业务冲突，不重试、
// 上浮 CodeConflict）。
func (r *skillRepo) createSkillWithVersionOnce(ctx context.Context, in biz.SkillCreateInput, metaJSON, source, trust string) (string, error) {
	skillID := bizskill.NewSkillID()
	versionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	now := nowRFC3339()
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return "", err
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
		SetMetadataJSON(metaJSON).
		SetVisibility(in.Visibility).
		SetFallbackConfigJSON(in.DefaultConfigJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(in.WorkspaceID).
		SetTrustLevel(trust).
		SetSource(source).
		Save(ctx); err != nil {
		return "", err
	}
	if _, err = tx.SkillVersion.Create().
		SetID(versionID).
		SetSkillID(skillID).
		SetVersion("1.0.0").
		SetStatus("pass").
		SetContentMarkdown(in.Body).
		SetMetadataJSON(metaJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return skillID, nil
}

// skillIDConflictMaxAttempts：skill 主键撞号重试上限。NewSkillID 128bit 随机
// 段下 3 次连撞概率天文物理级、实为失败信号灯（ID 生成器故障）。
const skillIDConflictMaxAttempts = 3

// isSkillIDConflict 仅认 skill 表主键的 23505（constraint=skill_pkey 或
// ent 语义的 platform_skill_pkey）。skill_key 唯一冲突是真实业务冲突，
// 绝不重试（重试只会原地再撞）。
func isSkillIDConflict(err error) bool {
	var pgErr *pq.Error
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	return pgErr.Constraint == "skill_pkey" || pgErr.Constraint == "platform_skill_pkey"
}

// retrySkillIDConflict 驱动 skill 建仓/复制整事务重试：op 必须返回**原始**
// 错误（entErrToBizErr 会把 23505 翻成 CodeConflict 丢 constraint 名，判定
// 失效）；每次重入 op 内重新 Begin 事务（25P02：出错事务不可复用）。
func retrySkillIDConflict(op func() error) error {
	var err error
	for attempt := 0; attempt < skillIDConflictMaxAttempts; attempt++ {
		if err = op(); !isSkillIDConflict(err) {
			return err
		}
	}
	return err
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
