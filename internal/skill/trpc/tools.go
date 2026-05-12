package trpc

import (
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpcskilltool "trpc.group/trpc-go/trpc-agent-go/tool/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

type SkillToolsetConfig struct {
	Repo               trpcskill.Repository
	Executor           codeexecutor.CodeExecutor
	ForceSaveArtifacts bool
}

func BuildSkillTools(cfg SkillToolsetConfig) []trpctool.Tool {
	var tools []trpctool.Tool
	tools = append(tools, trpcskilltool.NewLoadTool(cfg.Repo))
	if cfg.Executor != nil {
		runOpts := []func(*trpcskilltool.RunTool){
			trpcskilltool.WithForceSaveArtifacts(cfg.ForceSaveArtifacts),
		}
		tools = append(tools, trpcskilltool.NewRunTool(cfg.Repo, cfg.Executor, runOpts...))
	}
	tools = append(tools, trpcskilltool.NewListDocsTool(cfg.Repo))
	tools = append(tools, trpcskilltool.NewSelectDocsTool(cfg.Repo))
	return tools
}
