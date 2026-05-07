package skillruntime

import (
	"context"
	iofs "io/fs"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
)

// NewSkillToolsetFromFS builds an ADK skill toolset from a read-only filesystem root.
// Prefer this over calling skilltoolset.New directly so ADK wiring stays in one place.
func NewSkillToolsetFromFS(ctx context.Context, filesystem iofs.FS) (tool.Toolset, error) {
	if filesystem == nil {
		return nil, nil
	}
	return skilltoolset.New(ctx, skilltoolset.Config{Source: skill.NewFileSystemSource(filesystem)})
}

// AppendEnabledPublishedSkillToolsets adds one toolset rooted at the resolved skill storage path,
// limited to platform skills that are enabled and published (or active). When opts is nil, all such skills are mounted;
// otherwise layer A/B narrowing applies (see docs/需求/20 skill struct design.md 「十三′」).
func AppendEnabledPublishedSkillToolsets(ctx context.Context, out *[]tool.Toolset, skillUC *biz.SkillUsecase, sys biz.SystemSettingRepo, opts *SkillToolsetOptions) error {
	if skillUC == nil || out == nil {
		return nil
	}
	var slugs []string
	var err error
	if opts == nil {
		slugs, err = skillUC.ListEnabledPublishedSkillKeys(ctx)
	} else {
		slugs, err = resolveSkillSlugs(ctx, skillUC, opts)
	}
	if err != nil || len(slugs) == 0 {
		return err
	}
	rootDir := skillstorage.ResolveRoot()
	if sys != nil {
		if st, e := sys.Get(ctx); e == nil {
			rootDir = skillstorage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}
	ts, err := NewSkillToolsetFromFS(ctx, NewEnabledSkillsRootFS(rootDir, slugs))
	if err != nil || ts == nil {
		return err
	}
	*out = append(*out, ts)
	return nil
}
