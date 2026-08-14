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
	if len(team.DependsOn) > 0 {
		return TopologySequential
	}
	if team.ParallelConfigJSON != "" {
		cfg := ParseParallelConfig(team.ParallelConfigJSON, lg)
		if cfg.MaxConcurrentTeams > 1 {
			return TopologyParallel
		}
	}
	return TopologyCoordinator
}
