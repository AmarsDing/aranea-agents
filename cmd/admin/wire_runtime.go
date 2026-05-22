package main

import (
	"aranea-agents/internal/biz"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/team"
)

func initTeamRunnerGraphRuntime(runner *team.Runner, factory biz.GraphBuilderFactory, graphs *biz.GraphUsecase) {
	if runner == nil || factory == nil {
		return
	}
	if builder, ok := factory.(graphadapter.TeamGraphRootBuilder); ok {
		runner.SetGraphRootBuilder(builder)
	}
	if graphs != nil {
		runner.SetGraphBuildConfigLoader(graphadapter.NewLinkedGraphBuildConfigLoader(graphs))
	}
}
