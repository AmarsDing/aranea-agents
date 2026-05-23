package team

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/google/wire"
)

// ProvideTeamGraphRunCoordinator wires a singleton coordinator for team graph HITL/resume.
func ProvideTeamGraphRunCoordinator(graphs *biz.GraphUsecase, teams biz.TeamRepository, bus event.Bus) *TeamGraphRunCoordinator {
	return NewTeamGraphRunCoordinator(graphs, teams, bus)
}

// ProviderSet wires team runtime.
var ProviderSet = wire.NewSet(NewRunner, ProvideTeamGraphRunCoordinator)
