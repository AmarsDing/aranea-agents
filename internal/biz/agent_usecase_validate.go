package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// validateAgentCreate performs common validation for agent creation.
// It mutates settings for A2A Proxy agents (force-disables certain features).
func validateAgentCreate(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings, skipProviderValidate bool) error {
	switch agent.AgentKind {
	case AgentKindA2AProxy:
		if agent.A2AProxy == nil || strings.TrimSpace(agent.A2AProxy.RemoteURL) == "" {
			return apierror.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
		if agent.Provider == "" {
			agent.Provider = "a2a"
		}
		if agent.Model == "" {
			agent.Model = "proxy"
		}
	default:
		// Allow empty provider/model — the agent will inherit the model
		// from the chat interface at runtime (resolved via WithModel RunOption).
		if (agent.Provider != "" && agent.Model == "") || (agent.Provider == "" && agent.Model != "") {
			return apierror.BadRequest("AGENT", "provider and model must both be set or both be empty")
		}
	}
	return validateAgentSettings(ctx, u, agent, settings, "agent.create.provider_validate", skipProviderValidate)
}

// validateAgentUpdate performs validation for agent updates.
// Unlike create, provider/model are already set on the merged agent;
// this validates the pair and all settings fields.
func validateAgentUpdate(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings) error {
	if IsA2AProxyAgent(*agent) {
		if agent.A2AProxy == nil || strings.TrimSpace(agent.A2AProxy.RemoteURL) == "" {
			return apierror.BadRequest("AGENT", "a2a_proxy remote_url is required")
		}
	}
	return validateAgentSettings(ctx, u, agent, settings, "agent.update.provider_validate", false)
}

// validateAgentSettings is the shared validation logic for both Create and Update paths.
// It validates provider/model pairs, settings fields, and force-disables A2A-incompatible features.
func validateAgentSettings(ctx context.Context, u *AgentUsecase, agent *Agent, settings *AgentRuntimeSettings, logStepID string, skipProviderValidate bool) error {
	// Validate that the provider+model pair exists in the catalog (non-A2A agents only).
	// Skip validation when provider/model is empty — the agent will inherit
	// the model from the chat interface at runtime (resolved via WithModel RunOption).
	// Duplicate may also skip validation so a clone of an existing agent can be created
	// even when its original provider/model is no longer enabled in the catalog.
	if !skipProviderValidate && u.providerValidator != nil && !IsA2AProxyAgent(*agent) {
		prov := strings.TrimSpace(agent.Provider)
		mod := strings.TrimSpace(agent.Model)
		if prov != "" || mod != "" {
			ok, msg, valErr := u.providerValidator.ValidatePair(ctx, prov, mod)
			if valErr != nil {
				u.lg.Warn("provider model validation failed, proceeding",
					loggateway.StepID(logStepID),
					loggateway.Str("provider", prov),
					loggateway.Str("model", mod),
					loggateway.Err(valErr))
			} else if !ok {
				return apierror.BadRequest("AGENT", "provider/model is not enabled: "+msg)
			}
		}
	}
	if err := ValidateCodeExecutorType(settings.CodeExecutorType); err != nil {
		return err
	}
	if err := ValidatePlannerKind(settings.PlannerKind); err != nil {
		return err
	}
	if err := ValidatePlannerConfigJSON(settings.PlannerKind, settings.PlannerConfigJSON); err != nil {
		return err
	}
	if err := ValidateRalphLoopSettings(settings); err != nil {
		return err
	}
	// 校验 ToolsAllowJSON / ToolsDenyJSON 是有效的 JSON 字符串数组格式。
	// 注意：allow/deny 列表允许包含 group:* 前缀和未注册的 tool key（设计如此，
	// 不存在的 key 会被运行时忽略），因此只校验 JSON 格式，不校验 key 存在性。
	if err := validateToolsPolicyJSON(settings.ToolsAllowJSON, "tools_allow"); err != nil {
		return err
	}
	if err := validateToolsPolicyJSON(settings.ToolsDenyJSON, "tools_deny"); err != nil {
		return err
	}
	if IsA2AProxyAgent(*agent) {
		settings.IntentPassEnabled = false
		settings.ToolsEnabled = false
		settings.MemoryEnabled = false
		settings.SelfEvolve = false
	}
	return nil
}

// validateToolsPolicyJSON 校验 ToolsAllowJSON / ToolsDenyJSON 是有效的 JSON 字符串数组格式。
// 空字符串和 "[]" 视为合法（表示无策略）。允许包含 "group:*" 前缀的组 key。
func validateToolsPolicyJSON(raw, field string) error {
	if _, err := shared.JSONStringList(raw); err != nil {
		return apierror.BadRequest("AGENT", field+" json parse: "+err.Error())
	}
	return nil
}
