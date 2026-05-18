package skillruntime

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/storage"
	skilltrpc "aranea-agents/internal/skill/trpc"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

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
	rootDir := storage.ResolveRoot()
	if sys != nil {
		if st, e := sys.Get(ctx); e == nil {
			rootDir = storage.ResolveRootWithPlatform(st.RootDirectory)
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
