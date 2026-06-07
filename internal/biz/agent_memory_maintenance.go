package biz

import (
	"context"
	stderrors "errors"

	"aranea-agents/internal/biz/shared"
)

// AgentMemoryMaintenanceTarget is one agent's background memory maintenance knobs.
type AgentMemoryMaintenanceTarget struct {
	AgentID              string
	WriteL2Episode       bool
	WriteL3Facts         bool
	WriteL4Graph         bool
	L2RetentionDays      int
	L3DecayIntervalHours int
	L4DecayIntervalHours int
	L4DecayOverridesJSON string
}

// ListMemoryMaintenanceTargets returns agents with memory master gate enabled and decay/retention settings resolved.
func (u *AgentUsecase) ListMemoryMaintenanceTargets(ctx context.Context) ([]AgentMemoryMaintenanceTarget, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	const pageSize = 100
	var out []AgentMemoryMaintenanceTarget
	for offset := 0; ; offset += pageSize {
		page, err := u.repo.SearchAgents(ctx, AgentListQuery{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, ag := range page.Items {
			settings, err := u.repo.GetAgentRuntimeSettings(ctx, ag.ID)
			if err != nil {
				if !stderrors.Is(err, shared.ErrNotFound) {
					return nil, err
				}
				settings = DefaultAgentRuntimeSettings()
				settings.AgentID = ag.ID
			}
			policy := ResolveMemoryRuntimePolicy(&settings)
			if !policy.MasterEnabled {
				continue
			}
			out = append(out, AgentMemoryMaintenanceTarget{
				AgentID:              ag.ID,
				WriteL2Episode:       policy.WriteL2Episode,
				WriteL3Facts:         policy.WriteL3Facts,
				WriteL4Graph:         policy.WriteL4Graph,
				L2RetentionDays:      policy.L2RetentionDays,
				L3DecayIntervalHours: policy.L3DecayIntervalHours,
				L4DecayIntervalHours: policy.L4DecayIntervalHours,
				L4DecayOverridesJSON: policy.L4DecayOverridesJSON,
			})
		}
		if len(page.Items) < pageSize || offset+len(page.Items) >= page.Total {
			break
		}
	}
	return out, nil
}
