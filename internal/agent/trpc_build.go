package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/pkg/skillstorage"
	"aranea-agents/internal/provider"
	skilltrpc "aranea-agents/internal/skill/trpc"
	tooltrpc "aranea-agents/internal/tools/trpc"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcllmagent "trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcbuiltin "trpc.group/trpc-go/trpc-agent-go/planner/builtin"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

type TRPCBuilderDeps struct {
	Catalog    *biz.LlmProviderModelUsecase
	AgentUC    *biz.AgentUsecase
	Agents     biz.AgentRepository
	RT         *provider.RoundTrip
	SkillUC    *biz.SkillUsecase
	Sys        biz.SystemSettingRepo
	Provider   string
	Model      string
	DialogMode string
}

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
	if strings.TrimSpace(ag.AgentKey) == "" {
		return nil, kerrors.BadRequest("AGENT", "agent_key required")
	}
	prov := strutil.FirstNonEmpty(deps.Provider, ag.Provider)
	mod := strutil.FirstNonEmpty(deps.Model, ag.Model)
	if prov == "" || mod == "" {
		return nil, kerrors.BadRequest("AGENT", "provider and model required")
	}

	m, err := provider.TRPCModelForProviderModel(ctx, deps.Catalog, deps.RT, prov, mod)
	if err != nil {
		return nil, err
	}

	files := ag.Files
	if len(files) == 0 && deps.Agents != nil {
		files, err = deps.Agents.ListAgentPromptFiles(ctx, ag.ID)
		if err != nil {
			return nil, err
		}
	}
	sys := BuildSystemPrompt(ag, files)
	promptDeps := Deps{
		Agents:  deps.Agents,
		AgentUC: deps.AgentUC,
	}
	if cue := RuntimeCapabilityCue(ctx, promptDeps, ag); cue != "" {
		sys = sys + "\n\n" + cue
	}

	opts := []trpcllmagent.Option{
		trpcllmagent.WithModel(m),
		trpcllmagent.WithInstruction(sys),
		trpcllmagent.WithDescription(strings.TrimSpace(ag.DisplayName)),
		trpcllmagent.WithChannelBufferSize(256),
	}

	if strings.EqualFold(deps.DialogMode, "plan") {
		opts = append(opts, trpcllmagent.WithPlanner(trpcbuiltin.New(trpcbuiltin.Options{})))
	}

	if deps.SkillUC != nil {
		repo, filter, exec, err := buildSkillDeps(ctx, deps)
		if err != nil {
			return nil, err
		}
		if repo != nil {
			opts = append(opts, trpcllmagent.WithSkills(repo))
		}
		if filter != nil {
			opts = append(opts, trpcllmagent.WithSkillFilter(filter))
		}
		if exec != nil {
			opts = append(opts, trpcllmagent.WithCodeExecutor(exec))
		}
		opts = append(opts,
			trpcllmagent.WithSkillToolProfile(trpcllmagent.SkillToolProfileFull),
			trpcllmagent.WithSkillsDirectoryHints(true),
		)
	}

	if ts, err := buildToolsetsForAgent(ag, deps); err == nil && ts != nil {
		if len(ts.ToolSets) > 0 {
			opts = append(opts, trpcllmagent.WithToolSets(ts.ToolSets))
		}
		if len(ts.Tools) > 0 {
			opts = append(opts, trpcllmagent.WithTools(ts.Tools))
		}
	}

	return trpcllmagent.New(strings.TrimSpace(ag.AgentKey), opts...), nil
}

func buildSkillDeps(ctx context.Context, deps TRPCBuilderDeps) (trpcskill.Repository, trpcskill.VisibilityFilter, codeexecutor.CodeExecutor, error) {
	slugs, err := deps.SkillUC.ListEnabledPublishedSkillKeys(ctx)
	if err != nil || len(slugs) == 0 {
		return nil, nil, nil, err
	}

	rootDir := skillstorage.ResolveRoot()
	if deps.Sys != nil {
		if st, e := deps.Sys.Get(ctx); e == nil {
			rootDir = skillstorage.ResolveRootWithPlatform(st.RootDirectory)
		}
	}

	repo, err := skilltrpc.NewFSRepositoryAdapter(rootDir)
	if err != nil {
		return nil, nil, nil, err
	}

	allowSet := sliceToSet(slugs)
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		name := strings.TrimSpace(strings.ToLower(summary.Name))
		return allowSet[name]
	}

	exec := skilltrpc.NewLocalExecutor(rootDir)
	return repo, filter, exec, nil
}

func sliceToSet(slugs []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range slugs {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			m[s] = true
		}
	}
	return m
}

func buildToolsetsForAgent(ag biz.Agent, deps TRPCBuilderDeps) (*tooltrpc.AssembledToolsets, error) {
	cfg := tooltrpc.ToolsetConfig{}
	if ag.Settings != nil && ag.Settings.ToolsEnabled {
		eff := loadEffectiveToolKeys(deps, ag.ID)
		cfg.Filesystem = eff["read_file"] || eff["list_files"] || eff["write_file"] || eff["edit_file"]
		cfg.ShellExec = eff["shell_exec"]
	}
	if !cfg.Filesystem && !cfg.ShellExec {
		return nil, nil
	}
	return tooltrpc.BuildToolsets(cfg)
}

func loadEffectiveToolKeys(deps TRPCBuilderDeps, agentID string) map[string]bool {
	m := map[string]bool{}
	if deps.AgentUC == nil || strings.TrimSpace(agentID) == "" {
		return m
	}
	eff, err := deps.AgentUC.GetEffectiveTools(context.Background(), agentID)
	if err != nil || !eff.ToolsEnabled {
		return m
	}
	for _, it := range eff.Items {
		if it.Enabled {
			m[it.ToolKey] = true
		}
	}
	return m
}
