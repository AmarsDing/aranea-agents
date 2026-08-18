package biz

import (
	"context"
	stderrors "errors"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// ReorderAgents persists a manual ordering of agents.
// Manual order is not stored (no sort_order column). Returning success would
// make the list UI look like the drag stuck; fail closed instead.
func (u *AgentUsecase) ReorderAgents(_ context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return apierror.FailedPrecondition("AGENT", "manual agent order is not persisted; list order is is_default → kind → updated_at")
}

// UpsertByKey 按 agent_key 幂等创建或更新 Agent。
// 如果 agent_key 已存在则更新，否则创建。返回最终的 Agent（含水合）。
func (u *AgentUsecase) UpsertByKey(ctx context.Context, agent Agent) (Agent, error) {
	agent.AgentKey = strings.TrimSpace(agent.AgentKey)
	if agent.AgentKey == "" {
		return Agent{}, apierror.BadRequest("AGENT", "agent_key is required for upsert")
	}
	existing, err := u.reader.GetAgentByAgentKey(ctx, agent.AgentKey)
	if err == nil {
		// 已存在 → 更新
		agent.ID = existing.ID
		return u.Update(ctx, existing.ID, agent)
	}
	if !stderrors.Is(err, shared.ErrNotFound) {
		return Agent{}, err
	}
	// 不存在 → 创建
	return u.Create(ctx, agent)
}

// CreateWithFilesAndSettings 在事务中创建 Agent 并同时写入 Files 和 RuntimeSettings。
// 适用于 Pack 导入等需要精确控制写入内容的场景。
func (u *AgentUsecase) CreateWithFilesAndSettings(ctx context.Context, agent Agent, files []AgentPromptFile, settings *AgentRuntimeSettings) (Agent, error) {
	agent.AgentKey = strings.TrimSpace(agent.AgentKey)
	agent.DisplayName = strings.TrimSpace(agent.DisplayName)
	agent.Provider = strings.TrimSpace(agent.Provider)
	agent.Model = strings.TrimSpace(agent.Model)
	agent.AgentKind = NormalizeAgentKind(agent.AgentKind)
	HydrateAgentKind(&agent)

	if agent.AgentKey == "" || agent.DisplayName == "" {
		return Agent{}, apierror.BadRequest("AGENT", "agent_key and display_name are required")
	}
	if agent.ID == "" {
		agent.ID = newAgentCatalogID()
	}
	if agent.Status == "" {
		agent.Status = string(AgentStatusActive)
	} else if err := ValidateAgentStatus(agent.Status); err != nil {
		return Agent{}, err
	}

	// Settings
	var s AgentRuntimeSettings
	if settings != nil {
		s = *settings
	} else {
		s = withSettingDefaults(settingsFromAgentInput(agent))
	}
	s.AgentID = agent.ID

	if err := validateAgentCreate(ctx, u, &agent, &s, false); err != nil {
		return Agent{}, err
	}
	if agent.AgentKind != AgentKindA2AProxy {
		agent.AgentKind = AgentKindLLM
	}

	// Files
	for i := range files {
		files[i].AgentID = agent.ID
	}
	files = withFileDefaults(files)

	// DEV-10 FIXED: ConfigJSON is no longer written; cleared before persisting.
	agent.ConfigJSON = ""

	if err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := u.writer.CreateAgent(txCtx, agent); err != nil {
			if isAgentKeyDuplicate(err) {
				return apierror.BadRequest("AGENT_KEY_CONFLICT", "agent_key already in use")
			}
			return err
		}
		if _, err := u.settings.UpsertAgentRuntimeSettings(txCtx, s); err != nil {
			return err
		}
		if len(files) > 0 {
			if _, err := u.files.ReplaceAgentPromptFiles(txCtx, agent.ID, files); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Agent{}, err
	}
	readCtx := ctx
	if ctx.Err() != nil {
		readCtx = context.Background()
	}
	return u.Get(readCtx, agent.ID)
}

// AgentBatchUpdateInput is LIST-04 bulk enable/disable/delete.
type AgentBatchUpdateInput struct {
	IDs    []string
	Status string // optional: active | inactive
	Delete bool
}

// BatchUpdateAgents applies status changes or deletes for many agents inside a transaction.
func (u *AgentUsecase) BatchUpdateAgents(ctx context.Context, in AgentBatchUpdateInput) (int, error) {
	if u == nil || u.tx == nil {
		return 0, apierror.Internal("AGENT", "agent repository not configured")
	}
	if st := strings.TrimSpace(in.Status); st != "" {
		if err := ValidateAgentStatus(st); err != nil {
			return 0, err
		}
	}
	var n int
	err := u.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		for _, id := range in.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if in.Delete {
				// Permission check: unified with single Delete via canDeleteAgent
				a, err := u.reader.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				if err := canDeleteAgent(a); err != nil {
					return err
				}
				if err := u.writer.DeleteAgent(txCtx, id); err != nil {
					return err
				}
				n++
				continue
			}
			if st := strings.TrimSpace(in.Status); st != "" {
				a, err := u.reader.GetAgentByID(txCtx, id)
				if err != nil {
					return err
				}
				// AS-FSM-01: Validate state transition.
				if a.Status != st {
					if _, err := u.agentSM.Transition(ParseAgentState(a.Status), agentEventForTarget(ParseAgentState(st))); err != nil {
						return apierror.BadRequest("AGENT", "invalid status transition from "+a.Status+" to "+st+" for agent "+id)
					}
				}
				a.Status = st
				if _, err := u.writer.UpdateAgent(txCtx, a); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

func mergeAgentCatalog(current, patch Agent) Agent {
	out := current
	// Immutable fields: AgentKey and AgentKind are never merged from patch.
	// They are validated as immutable in Update() before this function is called.
	// Skipping them here prevents accidental overwrite if patch carries a stale value.
	// out.AgentKey remains current.AgentKey
	// out.AgentKind remains current.AgentKind
	out.DisplayName = firstNonEmpty(patch.DisplayName, current.DisplayName)
	out.Provider = firstNonEmpty(patch.Provider, current.Provider)
	out.Model = firstNonEmpty(patch.Model, current.Model)
	out.Status = firstNonEmpty(patch.Status, current.Status)
	// Boolean fields: *bool semantics — nil means "not set" (skip), non-nil
	// means "explicitly set" (overwrite). This solves the Proto3 zero-value
	// ambiguity where false and "not set" are indistinguishable.
	if patch.IsDefault != nil {
		out.IsDefault = patch.IsDefault
	}
	if patch.IsFavorite != nil {
		out.IsFavorite = patch.IsFavorite
	}
	out.Icon = firstNonEmpty(patch.Icon, current.Icon)
	out.AgentDescription = firstNonEmpty(patch.AgentDescription, current.AgentDescription)
	out.PositionID = firstNonEmpty(patch.PositionID, current.PositionID)
	out.PositionKey = firstNonEmpty(patch.PositionKey, current.PositionKey)
	out.AgentVariant = firstNonEmpty(patch.AgentVariant, current.AgentVariant)
	out.VariantDescription = firstNonEmpty(patch.VariantDescription, current.VariantDescription)
	out.MissionStatement = firstNonEmpty(patch.MissionStatement, current.MissionStatement)
	out.DomainPath = firstNonEmpty(patch.DomainPath, current.DomainPath)
	out.MetadataJSON = firstNonEmpty(patch.MetadataJSON, current.MetadataJSON)
	if len(patch.Roles) > 0 {
		out.Roles = patch.Roles
	}
	out.SystemPromptMode = firstNonEmpty(patch.SystemPromptMode, current.SystemPromptMode)
	if patch.ContextWindow != 0 {
		out.ContextWindow = patch.ContextWindow
	}
	if patch.BudgetMonthlyCents != 0 {
		out.BudgetMonthlyCents = patch.BudgetMonthlyCents
	}
	// DEV-10 FIXED: ConfigJSON is no longer merged/written; it is computed on read.
	// out.ConfigJSON is intentionally left from current (will be cleared before persist).
	if patch.Settings != nil {
		out.Settings = patch.Settings
	}
	if len(patch.Files) > 0 {
		out.Files = patch.Files
	}
	if patch.A2AProxy != nil && IsA2AProxyAgent(out) {
		out.A2AProxy = patch.A2AProxy
	}
	return out
}

// firstNonEmpty returns the first argument after TrimSpace if non-empty, otherwise the second.
func firstNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return b
}
