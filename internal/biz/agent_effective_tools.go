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
	// ExecutionTimeoutSec 为 nil 表示不修改该字段（P1-2 resolver 化策略字段，
	// 需要区分「未提交」与「提交 0 = 恢复默认」，故用指针）。
	ExecutionTimeoutSec *int32
}

// AgentToolPolicyChange 分类一次 policy 更新实际改变的字段维度（P1-2）：
// service 层据此决定失效策略——StructureChanged 才需要重建 agent；
// 仅 PolicyChanged（resolver 化字段）时 Set resolver 即可，零重建。
type AgentToolPolicyChange struct {
	// StructureChanged：装配字段（ToolsEnabled/Profile/Allow/Deny）值变化，
	// 影响工具集装配，必须 invalidate 重建。
	StructureChanged bool
	// PolicyChanged：resolver 化策略字段（当前仅 ExecutionTimeoutSec）值变化。
	PolicyChanged bool
	// NewExecutionTimeoutSec 是变更后的 timeout 原始值（仅 PolicyChanged 时有意义），
	// 供 service 层同步 policyResolver。
	NewExecutionTimeoutSec int
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
// P1-2：返回值附带变更分类——与持久化前值比较得出（而非请求字段存在性，
// 因前端全量提交表单，字段存在≠值变化）。调用方据 AgentToolPolicyChange
// 决定失效策略：StructureChanged → invalidate 重建；仅 PolicyChanged →
// 同步 policyResolver，零重建。
func (u *AgentUsecase) UpdateAgentToolPolicy(ctx context.Context, agentID string, in AgentToolPolicyInput) (AgentEffectiveTools, AgentToolPolicyChange, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentEffectiveTools{}, AgentToolPolicyChange{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	if _, err := u.reader.GetAgentByID(ctx, agentID); err != nil {
		return AgentEffectiveTools{}, AgentToolPolicyChange{}, err
	}
	settings, err := u.runtimeSettingsForEffective(ctx, agentID)
	if err != nil {
		return AgentEffectiveTools{}, AgentToolPolicyChange{}, err
	}
	var change AgentToolPolicyChange
	if settings.ToolsEnabled != in.ToolsEnabled {
		change.StructureChanged = true
	}
	settings.ToolsEnabled = in.ToolsEnabled
	if p := strings.TrimSpace(in.Profile); p != "" {
		if settings.ToolsProfile != p {
			change.StructureChanged = true
		}
		settings.ToolsProfile = p
	}
	allowJSON, _ := json.Marshal(in.Allow)
	denyJSON, _ := json.Marshal(in.Deny)
	if settings.ToolsAllowJSON != string(allowJSON) || settings.ToolsDenyJSON != string(denyJSON) {
		change.StructureChanged = true
	}
	settings.ToolsAllowJSON = string(allowJSON)
	settings.ToolsDenyJSON = string(denyJSON)
	if in.ExecutionTimeoutSec != nil && int(*in.ExecutionTimeoutSec) != settings.ToolsExecutionTimeoutSec {
		change.PolicyChanged = true
		change.NewExecutionTimeoutSec = int(*in.ExecutionTimeoutSec)
		settings.ToolsExecutionTimeoutSec = int(*in.ExecutionTimeoutSec)
	}
	if _, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
		return AgentEffectiveTools{}, AgentToolPolicyChange{}, err
	}
	all, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return AgentEffectiveTools{}, AgentToolPolicyChange{}, err
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
	return eff, change, nil
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
