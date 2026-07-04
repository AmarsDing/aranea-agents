package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// AgentCapabilityBuilder builds AgentCapability from the agent catalog.
type AgentCapabilityBuilder struct {
	agentReader biz.AgentReader
	lg          loggateway.Logger
}

// NewAgentCapabilityBuilder creates a new AgentCapabilityBuilder.
func NewAgentCapabilityBuilder(agentReader biz.AgentReader, lg loggateway.Logger) *AgentCapabilityBuilder {
	return &AgentCapabilityBuilder{
		agentReader: agentReader,
		lg:          lg,
	}
}

// BuildAll builds AgentCapability for all active agents in the catalog.
func (b *AgentCapabilityBuilder) BuildAll(ctx context.Context) ([]biz.AgentCapability, error) {
	result, err := b.agentReader.SearchAgents(ctx, biz.AgentListQuery{
		Status: "active",
		Limit:  200,
	})
	if err != nil {
		return nil, err
	}

	var capabilities []biz.AgentCapability
	for _, ag := range result.Items {
		// 2026-07-04 问题 3 修复：系统 Agent（精灵助手/系统管家/记忆管家/技能管家）
		// 是基础设施级 Agent，不参与业务任务团队匹配，从源头过滤掉。
		if biz.IsSystemAgentKey(ag.AgentKey) {
			continue
		}
		cap := biz.AgentCapability{
			AgentKey:    ag.AgentKey,
			DisplayName: ag.DisplayName,
			Description: ag.AgentDescription,
			Roles:       ag.Roles,
			Domains:     extractDomainsFromConfig(ag.ConfigJSON),
			Tools:       extractToolNamesFromConfig(ag.ConfigJSON),
			Skills:      extractSkillNamesFromConfig(ag.ConfigJSON),
			Capacity:    extractCapacityFromConfig(ag.ConfigJSON),
		}
		capabilities = append(capabilities, cap)
	}
	return capabilities, nil
}

// extractDomainsFromConfig extracts domain tags from agent config_json.
func extractDomainsFromConfig(configJSON string) []string {
	if strings.TrimSpace(configJSON) == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return nil
	}
	raw, ok := cfg["domains"]
	if !ok {
		return nil
	}
	var domains []string
	if json.Unmarshal(raw, &domains) != nil {
		return nil
	}
	return domains
}

// extractToolNamesFromConfig extracts tool names from agent config_json.
func extractToolNamesFromConfig(configJSON string) []string {
	if strings.TrimSpace(configJSON) == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return nil
	}
	// Try "tools" array first
	raw, ok := cfg["tools"]
	if !ok {
		return nil
	}
	// Tools can be an array of strings or array of objects with "name" field
	var names []string

	// Try as []string
	var strTools []string
	if json.Unmarshal(raw, &strTools) == nil {
		return strTools
	}

	// Try as []struct with name
	var objTools []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &objTools) == nil {
		for _, t := range objTools {
			if t.Name != "" {
				names = append(names, t.Name)
			}
		}
		return names
	}

	return nil
}

// extractSkillNamesFromConfig extracts skill names from agent config_json.
func extractSkillNamesFromConfig(configJSON string) []string {
	if strings.TrimSpace(configJSON) == "" {
		return nil
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return nil
	}
	raw, ok := cfg["skills"]
	if !ok {
		return nil
	}
	var skills []string
	if json.Unmarshal(raw, &skills) != nil {
		return nil
	}
	return skills
}

// extractCapacityFromConfig extracts capacity from agent config_json.
func extractCapacityFromConfig(configJSON string) int {
	if strings.TrimSpace(configJSON) == "" {
		return 1
	}
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return 1
	}
	raw, ok := cfg["capacity"]
	if !ok {
		return 1
	}
	var capacity int
	if json.Unmarshal(raw, &capacity) != nil || capacity <= 0 {
		return 1
	}
	return capacity
}
