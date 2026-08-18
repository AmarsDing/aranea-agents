package biz

import (
	"aranea-agents/pkg/loggateway"
)

type TopologyType string

const (
	TopologyDirect      TopologyType = "direct"
	TopologyParallel    TopologyType = "parallel"
	TopologySequential  TopologyType = "sequential"
	TopologyHybrid      TopologyType = "hybrid"
	TopologyCoordinator TopologyType = "coordinator"
)

func InferTopologyFromTeam(team Team, lg loggateway.Logger) TopologyType {
	if team.Topology != "" {
		return TopologyType(team.Topology)
	}
	hasDeps := len(team.DependsOn) > 0
	parallel := false
	if team.ParallelConfigJSON != "" {
		cfg := ParseParallelConfig(team.ParallelConfigJSON, lg)
		parallel = cfg.MaxConcurrentTeams > 1
	}
	switch {
	case hasDeps && parallel:
		return TopologyHybrid
	case hasDeps:
		return TopologySequential
	case parallel:
		return TopologyParallel
	default:
		return TopologyCoordinator
	}
}
