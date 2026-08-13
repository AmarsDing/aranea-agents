package biz

import (
	"context"

	"aranea-agents/internal/biz/plugin"
)

type (
	Plugin                   = plugin.Plugin
	PluginPermissions        = plugin.Permissions
	PluginListQuery          = plugin.ListQuery
	PluginListResult         = plugin.ListResult
	PluginStatUpdate         = plugin.StatUpdate
	PluginRepo               = plugin.Repo
	PluginRun                = plugin.Run
	PluginRunQuery           = plugin.RunQuery
	PluginRunListResult      = plugin.RunListResult
	PluginRunRepo            = plugin.RunRepo
	PluginUsecase            = plugin.Usecase
	PluginCostGuardUsageRepo = plugin.CostGuardUsageRepo
	ScopeAgentLookup         = plugin.ScopeAgentLookup
)

var (
	NewPluginUsecase         = plugin.NewUsecase
	ValidatePluginJSONSchema = plugin.ValidateJSONSchema
	AdminPluginPerms         = plugin.AdminPerms
	BuiltinMetaDrifted       = plugin.BuiltinMetaDrifted
)

// agentScopeLookup adapts AgentRepository to plugin.ScopeAgentLookup.
type agentScopeLookup struct {
	AgentRepository
}

func (a agentScopeLookup) AgentExists(ctx context.Context, id string) error {
	_, err := a.AgentRepository.GetAgentByID(ctx, id)
	return err
}

// NewScopeAgentLookup wraps an AgentRepository as a ScopeAgentLookup.
func NewScopeAgentLookup(repo AgentRepository) ScopeAgentLookup {
	return agentScopeLookup{repo}
}
