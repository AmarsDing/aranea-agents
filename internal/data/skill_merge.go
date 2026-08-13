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
		return nil, entErrToBizErr(err, apierror.DomainSkill)
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
	var newVersionID string
	var transferred int

	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		client := r.data.RW().Write(txCtx)

		// 1. 创建新版本（fused 内容）— version 从目标最新版本递增
		now := nowRFC3339()
		newVersionID = fmt.Sprintf("skillver_%d", time.Now().UTC().UnixNano())
		latestVer, lvErr := client.SkillVersion.Query().
			Where(skillversion.SkillIDEQ(params.TargetID)).
			Order(skillversion.ByCreatedAt(entsql.OrderDesc())).
			First(txCtx)
		nextVer := "1.0.0"
		if lvErr == nil && latestVer != nil {
			nextVer = incrementVersion(latestVer.Version)
		}
		newVersion, vErr := client.SkillVersion.Create().
			SetID(newVersionID).
			SetSkillID(params.TargetID).
			SetVersion(nextVer).
			SetContentMarkdown(params.FusedBody).
			SetStatus("published").
			SetEvolutionReason(params.MergeReason).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(txCtx)
		if vErr != nil {
			return entErrToBizErr(vErr, apierror.DomainSkill)
		}
		newVersionID = newVersion.ID

		// 2. 更新目标 Skill 的 metadata（tags）
		target, tErr := client.PlatformSkill.Query().
			Where(platformskill.IDEQ(params.TargetID)).
			Only(txCtx)
		if tErr != nil {
			return entErrToBizErr(tErr, apierror.DomainSkill)
		}
		md := parseSkillMetadata(r.lg, target.MetadataJSON)
		md.Tags = stringSliceToSkillTags(params.FusedTags)
		metaJSON, jErr := json.Marshal(md)
		if jErr != nil {
			return entErrToBizErr(jErr, apierror.DomainSkill)
		}

		_, uErr := client.PlatformSkill.UpdateOneID(params.TargetID).
			SetMetadataJSON(string(metaJSON)).
			SetUpdatedAt(now).
			Save(txCtx)
		if uErr != nil {
			return entErrToBizErr(uErr, apierror.DomainSkill)
		}

		// 3. 转移调用记录
		var trErr error
		transferred, trErr = client.SkillInvocation.Update().
			Where(skillinvocation.SkillIDEQ(params.SourceID)).
			SetSkillID(params.TargetID).
			Save(txCtx)
		if trErr != nil {
			return entErrToBizErr(trErr, apierror.DomainSkill)
		}

		// 4. 废弃源 Skill（软删墓碑保留审计）。skill_key 为全表唯一索引（无状态
		// 过滤），墓碑必须释放 slug——追加 deprecated 后缀，否则同名重建/导入
		// 永久唯一键冲突（批次 C2）。
		src, sErr := client.PlatformSkill.Get(txCtx, params.SourceID)
		if sErr != nil {
			return entErrToBizErr(sErr, apierror.DomainSkill)
		}
		releasedKey := fmt.Sprintf("%s--deprecated-%d", src.SkillKey, time.Now().UTC().UnixNano())
		_, dErr := client.PlatformSkill.UpdateOneID(params.SourceID).
			SetEnabled(false).
			SetStatus("deprecated").
			SetDeletedAt(now).
			SetSkillKey(releasedKey).
			SetUpdatedAt(now).
			Save(txCtx)
		if dErr != nil {
			return entErrToBizErr(dErr, apierror.DomainSkill)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &biz.SkillMergeResult{
		TargetSkillID:    params.TargetID,
		NewVersionID:     newVersionID,
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
