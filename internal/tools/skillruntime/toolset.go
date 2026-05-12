package skillruntime

import (
	"context"
	iofs "io/fs"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"
	skilltrpc "aranea-agents/internal/skill/trpc"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
)

func NewSkillToolsetFromFS(ctx context.Context, filesystem iofs.FS) (tool.Toolset, error) {
	if filesystem == nil {
		return nil, nil
	}
	return skilltoolset.New(ctx, skilltoolset.Config{Source: skill.NewFileSystemSource(filesystem)})
}

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

func BuildTRPCSkillTools(ctx context.Context, skillUC *biz.SkillUsecase, sys biz.SystemSettingRepo, opts *SkillToolsetOptions, exec codeexecutor.CodeExecutor) ([]trpctool.Tool, error) {
	if skillUC == nil {
		return nil, nil
	}
	var slugs []string
	var err error
	if opts == nil {
		slugs, err = skillUC.ListEnabledPublishedSkillKeys(ctx)
	} else {
		slugs, err = resolveSkillSlugs(ctx, skillUC, opts)
	}
	if err != nil || len(slugs) == 0 {
		return nil, err
	}
	rootDir := skillstorage.ResolveRoot()
	if sys != nil {
		if st, e := sys.Get(ctx); e == nil {
			rootDir = skillstorage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}
	repo, err := skilltrpc.NewFSRepositoryAdapter(rootDir)
	if err != nil {
		return nil, err
	}
	filtered := skilltrpc.NewFilteredRepository(repo, slugs)
	return skilltrpc.BuildSkillTools(skilltrpc.SkillToolsetConfig{
		Repo:     filtered,
		Executor: exec,
	}), nil
}
