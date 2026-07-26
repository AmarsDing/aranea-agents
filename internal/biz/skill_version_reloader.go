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
func (r *SkillVersionReloader) Reload(ctx context.Context, skillID string, draftBody string, parentVersionID string, evolutionReason string) error {
	skillID = strings.TrimSpace(skillID)
	draftBody = strings.TrimSpace(draftBody)
	if skillID == "" {
		return apierror.BadRequest("SKILL_RELOADER", "skill_id is required")
	}
	if draftBody == "" {
		return apierror.BadRequest("SKILL_RELOADER", "draft body is required")
	}

	parent := strings.TrimSpace(parentVersionID)
	if parent == "" {
		resolved, err := r.resolveLatestVersionID(ctx, skillID)
		if err != nil {
			return err
		}
		parent = resolved
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

func (r *SkillVersionReloader) resolveLatestVersionID(ctx context.Context, skillID string) (string, error) {
	res, err := r.queries.ListSkillVersions(ctx, skill.VersionListQuery{SkillID: skillID, Limit: 1})
	if err != nil {
		r.lg.Warn("SkillVersionReloader: resolve latest version failed",
			loggateway.StepID("skill_reloader.reload"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(err))
		return "", err
	}
	if len(res.Items) == 0 {
		return "", apierror.NotFound("SKILL_RELOADER", "no existing version to anchor as parent: %s", skillID)
	}
	return res.Items[0].ID, nil
}
