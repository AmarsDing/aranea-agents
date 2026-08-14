package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
)

// EffectiveAgentTool is one row in the agent effective tools matrix (legacy JSON shape).
type EffectiveAgentTool struct {
	ToolKey        string
	DisplayName    string
	Category       string
	Source         string
	Enabled        bool
	EffectiveState string
	Reason         string
}

// AgentEffectiveTools matches pkg/backend domain.AgentEffectiveTools JSON for API compatibility.
type AgentEffectiveTools struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
	Items        []EffectiveAgentTool
}

// AgentToolPolicyInput is the writable subset for PUT .../tools/policy.
type AgentToolPolicyInput struct {
	ToolsEnabled bool
	Profile      string
	Allow        []string
	Deny         []string
}

// GetEffectiveTools returns merged tool catalog + agent runtime policy (legacy semantics).
func (u *AgentUsecase) GetEffectiveTools(ctx context.Context, agentID string) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.reader.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	platform := loadWebResearchPlatformFromSys(ctx, u.sys)
	for i := range all.Items {
		EnrichToolRuntimeFieldsWithPlatform(&all.Items[i], platform, checkerToReadinessFunc(u.webResearchChecker))
	}
	eff := buildAgentEffectiveTools(settings, all.Items, u.lg)
	var overrides []ToolAgentOverride
	if o, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		overrides = o
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	applyWebResearchEffectiveGate(u.webResearchChecker, &eff, all.Items, platform, overrides)
	return eff, nil
}

func (u *AgentUsecase) runtimeSettingsForEffective(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			s := DefaultAgentRuntimeSettings()
			s.AgentID = agentID
			return withSettingDefaults(s), nil
		}
		return AgentRuntimeSettings{}, err
	}
	return withSettingDefaults(settings), nil
}

// UpdateAgentToolPolicy updates agent_runtime_settings tool columns and returns recomputed effective tools.
func (u *AgentUsecase) UpdateAgentToolPolicy(ctx context.Context, agentID string, in AgentToolPolicyInput) (AgentEffectiveTools, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.reader.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	settings.ToolsEnabled = in.ToolsEnabled
	if strings.TrimSpace(in.Profile) != "" {
		settings.ToolsProfile = strings.TrimSpace(in.Profile)
	}
	allowJSON, _ := json.Marshal(in.Allow)
	denyJSON, _ := json.Marshal(in.Deny)
	settings.ToolsAllowJSON = string(allowJSON)
	settings.ToolsDenyJSON = string(denyJSON)
	if _, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return AgentEffectiveTools{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, err
	}
	// No re-read after upsert: `settings` already holds the persisted values
	// (runtimeSettingsForEffective applied defaults; we mutated the tool columns
	// in place), so a second GetAgentRuntimeSettings would be a redundant query.

	platform := loadWebResearchPlatformFromSys(ctx, u.sys)
	for i := range all.Items {
		EnrichToolRuntimeFieldsWithPlatform(&all.Items[i], platform, checkerToReadinessFunc(u.webResearchChecker))
	}
	eff := buildAgentEffectiveTools(settings, all.Items, u.lg)
	var overrides []ToolAgentOverride
	if o, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, agentID); oerr == nil {
		overrides = o
		ApplyAgentToolOverrides(&eff, all.Items, overrides)
	}
	applyWebResearchEffectiveGate(u.webResearchChecker, &eff, all.Items, platform, overrides)
	return eff, nil
}

// ToolKeyInAllowJSON checks whether a specific tool key is present in a JSON allow list string.
// Used by CustomTool injection logic (e.g. build_orchestration_graph) to determine
// whether a non-Spirit agent should receive the tool.
func ToolKeyInAllowJSON(allowJSON, key string) bool {
	list, err := shared.JSONStringList(strings.TrimSpace(allowJSON))
	if err != nil {
		return false
	}
	for _, k := range list {
		if k == key {
			return true
		}
	}
	return false
}
