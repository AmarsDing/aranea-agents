package trpc

import (
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcskilltool "trpc.group/trpc-go/trpc-agent-go/tool/skill"
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
