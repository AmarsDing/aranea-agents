package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/internal/data/ent/skillversion"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type SkillMergeRepo struct {
	data *Data
	lg   loggateway.Logger
}

var (
	_ biz.SkillMergeReader = (*SkillMergeRepo)(nil)
	_ biz.SkillMergeWriter = (*SkillMergeRepo)(nil)
)

func NewSkillMergeRepo(data *Data, lg loggateway.Logger) *SkillMergeRepo {
	return &SkillMergeRepo{data: data, lg: lg}
}

// GetFullSkillForMerge 获取合并所需的完整 Skill 数据
func (r *SkillMergeRepo) GetFullSkillForMerge(ctx context.Context, skillID string) (*biz.SkillMergeSource, error) {
	skill, err := r.data.RW().Read(ctx).PlatformSkill.Query().
		Where(platformskill.IDEQ(skillID), platformskill.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return nil, apierror.NotFound(apierror.DomainSkill, "skill not found")
		}
		return nil, entErrToBizErr(err, "SKILL")
	}

	// 获取最新 published 版本的完整内容
	version, vErr := r.data.RW().Read(ctx).SkillVersion.Query().
		Where(
			skillversion.SkillIDEQ(skillID),
			skillversion.StatusEQ("published"),
		).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)

	body := ""
	if vErr == nil && version != nil {
		body = version.ContentMarkdown
	}
	if body == "" {
		r.lg.Warn("GetFullSkillForMerge: no published version body found, merge may lose content",
			loggateway.StepID("data.skill_merge"),
			loggateway.Str("skill_id", skillID))
	}

	// 从 metadata_json 解析 tags
	tags := extractTagNames(parseSkillTags(skill.MetadataJSON))

	return &biz.SkillMergeSource{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Body:        body,
		Tags:        tags,
		Status:      skill.Status,
	}, nil
}

// ApplyMerge 事务性应用合并结果
func (r *SkillMergeRepo) ApplyMerge(ctx context.Context, params biz.SkillMergeApplyParams) (*biz.SkillMergeResult, error) {
	tx, err := r.data.RW().Write(ctx).Tx(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SKILL")
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 创建新版本（fused 内容）— version 从目标最新版本递增
	now := nowRFC3339()
	newVersionID := fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
	latestVer, lvErr := tx.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(params.TargetID)).
		Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
		First(ctx)
	nextVer := "1.0.0"
	if lvErr == nil && latestVer != nil {
		nextVer = incrementVersion(latestVer.Version)
	}
	newVersion, vErr := tx.SkillVersion.Create().
		SetID(newVersionID).
		SetSkillID(params.TargetID).
		SetVersion(nextVer).
		SetContentMarkdown(params.FusedBody).
		SetStatus("published").
		SetEvolutionReason(params.MergeReason).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if vErr != nil {
		return nil, entErrToBizErr(vErr, "SKILL")
	}

	// 2. 更新目标 Skill 的 metadata（tags）
	// 先读取当前 metadata，更新 tags，再写回
	target, tErr := tx.PlatformSkill.Query().
		Where(platformskill.IDEQ(params.TargetID)).
		Only(ctx)
	if tErr != nil {
		return nil, entErrToBizErr(tErr, "SKILL")
	}
	md := parseSkillMetadata(r.lg, target.MetadataJSON)
	md.Tags = stringSliceToSkillTags(params.FusedTags)
	metaJSON, jErr := json.Marshal(md)
	if jErr != nil {
		return nil, entErrToBizErr(jErr, "SKILL")
	}

	_, uErr := tx.PlatformSkill.UpdateOneID(params.TargetID).
		SetMetadataJSON(string(metaJSON)).
		SetUpdatedAt(now).
		Save(ctx)
	if uErr != nil {
		return nil, entErrToBizErr(uErr, "SKILL")
	}

	// 3. 转移调用记录
	transferred, trErr := tx.SkillInvocation.Update().
		Where(skillinvocation.SkillIDEQ(params.SourceID)).
		SetSkillID(params.TargetID).
		Save(ctx)
	if trErr != nil {
		return nil, entErrToBizErr(trErr, "SKILL")
	}

	// 4. 废弃源 Skill
	_, dErr := tx.PlatformSkill.UpdateOneID(params.SourceID).
		SetEnabled(false).
		SetStatus("deprecated").
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if dErr != nil {
		return nil, entErrToBizErr(dErr, "SKILL")
	}

	if cErr := tx.Commit(); cErr != nil {
		return nil, entErrToBizErr(cErr, "SKILL")
	}

	return &biz.SkillMergeResult{
		TargetSkillID:    params.TargetID,
		NewVersionID:     newVersion.ID,
		FusedBody:        params.FusedBody,
		FusedTags:        params.FusedTags,
		TransferredCount: transferred,
	}, nil
}

// stringSliceToSkillTags converts []string to []biz.SkillTag for metadata encoding.
func stringSliceToSkillTags(names []string) []biz.SkillTag {
	tags := make([]biz.SkillTag, 0, len(names))
	for _, n := range names {
		tags = append(tags, biz.SkillTag{Name: n, Source: "merge"})
	}
	return tags
}
