package modelsync

import (
	"aranea-agents/internal/modelregistry"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type Deps struct {
	Phases        *Phases
	StoreProvider modelregistry.StoreProvider
	Backend       modelregistry.ApplyBackend
}

type Phases struct {
	fetchPhase   modelregistry.Phase
	migratePhase modelregistry.Phase
	applyPhase   modelregistry.Phase
	logoPhase    modelregistry.Phase
}

func BuildPhases(backend modelregistry.ApplyBackend) *Phases {
	return &Phases{
		fetchPhase:   modelregistry.NewFetchPhase(),
		migratePhase: modelregistry.NewMigratePhase(backend),
		applyPhase:   modelregistry.NewApplyPhase(backend, backend),
		logoPhase:    modelregistry.NewLogoPhase(),
	}
}

func (p *Phases) List() []modelregistry.Phase {
	return []modelregistry.Phase{p.fetchPhase, p.migratePhase, p.applyPhase}
}

func (p *Phases) LogoPhase() modelregistry.Phase {
	return p.logoPhase
}

func RegisterAll(deps Deps) []trpctool.Tool {
	return []trpctool.Tool{
		newFetchDirectoryTool(deps),
		newMigrateProvidersTool(deps),
		newApplyDirectoryTool(deps),
		newSyncProviderLogosTool(deps),
	}
}
