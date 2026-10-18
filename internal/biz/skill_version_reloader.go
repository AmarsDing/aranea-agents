package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// SkillVersionReloader is the production SkillReloader: it registers the
// approved evolved draft as a new skill version. The previous version is
// preserved via ParentVersionID; rollback goes through the existing
// RollbackSkillVersion API.
type SkillVersionReloader struct {
	writer  skill.SkillVersionWriter
	queries skill.SkillQueryReader
	lg      loggateway.Logger
}

// NewSkillVersionReloader constructs a SkillVersionReloader.
func NewSkillVersionReloader(writer skill.SkillVersionWriter, queries skill.SkillQueryReader, lg loggateway.Logger) *SkillVersionReloader {
	return &SkillVersionReloader{
		writer:  writer,
		queries: queries,
		lg:      lg,
	}
}

// Reload registers draftBody as the new current version of skillID.
// parentVersionID may be empty — the reloader then anchors to the latest
// persisted version (fresh read, reflecting what is current NOW). Reload
// refuses to create an orphan version when no parent anchor can be resolved.
//
// 审计 W2 幂等：ApplyApprovedSuggestion 存在"版本注册成功、建议状态回写失败"
// 的部分成功窗口，重试不得重复注册版本。最新版本正文与草案等值（trim 比较）
// 即视为本次演化已应用，跳过注册——与 applyAgentCreateSkill 的 SkillExists
// 幂等策略对齐。
func (r *SkillVersionReloader) Reload(ctx context.Context, skillID string, draftBody string, parentVersionID string, evolutionReason string) error {
	skillID = strings.TrimSpace(skillID)
	draftBody = strings.TrimSpace(draftBody)
	if skillID == "" {
		return apierror.BadRequest("SKILL_RELOADER", "skill_id is required")
	}
	if draftBody == "" {
		return apierror.BadRequest("SKILL_RELOADER", "draft body is required")
	}

	latest, err := r.getLatestVersion(ctx, skillID)
	if latest == nil {
		if err != nil {
			return err
		}
		if strings.TrimSpace(parentVersionID) == "" {
			return apierror.NotFound("SKILL_RELOADER", "no existing version to anchor as parent: %s", skillID)
		}
	} else if strings.TrimSpace(latest.ContentMarkdown) == draftBody {
		r.lg.Info("SkillVersionReloader: latest version already matches draft, idempotent skip (duplicate apply)",
			loggateway.StepID("skill_reloader.reload"),
			loggateway.Str("skill_id", skillID),
			loggateway.Str("version_id", latest.ID))
		return nil
	}

	parent := strings.TrimSpace(parentVersionID)
	if parent == "" && latest != nil {
		parent = latest.ID
	}

	detail, err := r.writer.CreateSkillVersion(ctx, skill.CreateVersionInput{
		SkillID:         skillID,
		Body:            draftBody,
		ParentVersionID: parent,
		EvolutionReason: evolutionReason,
	})
	if err != nil {
		return err
	}

	r.lg.Info("SkillVersionReloader: new version registered",
		loggateway.StepID("skill_reloader.reload"),
		loggateway.Str("skill_id", skillID),
		loggateway.Str("version_id", detail.ID),
		loggateway.Str("parent_version_id", parent))
	return nil
}

// getLatestVersion returns the newest persisted version detail, or (nil, nil)
// when the skill has no versions yet.
func (r *SkillVersionReloader) getLatestVersion(ctx context.Context, skillID string) (*skill.SkillVersionDetail, error) {
	res, err := r.queries.ListSkillVersions(ctx, skill.VersionListQuery{SkillID: skillID, Limit: 1})
	if err != nil {
		r.lg.Warn("SkillVersionReloader: resolve latest version failed",
			loggateway.StepID("skill_reloader.reload"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, nil
	}
	latest := res.Items[0]
	return &latest, nil
}
