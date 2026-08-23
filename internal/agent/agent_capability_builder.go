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
	org         biz.OrganizationReader
	lg          loggateway.Logger
}

// NewAgentCapabilityBuilder creates a new AgentCapabilityBuilder.
func NewAgentCapabilityBuilder(agentReader biz.AgentReader, lg loggateway.Logger) *AgentCapabilityBuilder {
	return &AgentCapabilityBuilder{
		agentReader: agentReader,
		lg:          lg,
	}
}

// SetOrganizationReader attaches an org reader used to fill placement fields.
// Nil is allowed: BuildAll still succeeds and leaves DepartmentID empty.
func (b *AgentCapabilityBuilder) SetOrganizationReader(org biz.OrganizationReader) {
	if b == nil {
		return
	}
	b.org = org
}

// BuildAll builds AgentCapability for all active agents in the catalog.
func (b *AgentCapabilityBuilder) BuildAll(ctx context.Context) ([]biz.AgentCapability, error) {
	var capabilities []biz.AgentCapability
	for offset := 0; ; offset += 200 {
		result, err := b.agentReader.SearchAgents(ctx, biz.AgentListQuery{
			Status: "active",
			Limit:  200,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, ag := range result.Items {
			// 2026-07-04 问题 3 修复：系统 Agent（精灵助手/系统管家/记忆管家/技能管家）
			// 是基础设施级 Agent，不参与业务任务团队匹配，从源头过滤掉。
			if biz.IsSystemAgentKey(ag.AgentKey) {
				continue
			}
			mission := ag.MissionStatement
			if mission == "" {
				mission = ag.AgentDescription // 不变量 2：存量 Agent Mission 回退 Description
			}
			cap := biz.AgentCapability{
				AgentKey:     ag.AgentKey,
				DisplayName:  ag.DisplayName,
				Description:  ag.AgentDescription,
				Mission:      mission,
				DomainPath:   strings.TrimSpace(ag.DomainPath),
				Roles:        ag.Roles,
				Domains:      extractDomainsFromConfig(ag.ConfigJSON),
				Tools:        extractToolNamesFromConfig(ag.ConfigJSON),
				Skills:       extractSkillNamesFromConfig(ag.ConfigJSON),
				Capacity:     extractCapacityFromConfig(ag.ConfigJSON),
				PositionID:   ag.PositionID,
				PositionKey:  ag.PositionKey,
				AgentVariant: ag.AgentVariant,
			}
			capabilities = append(capabilities, cap)
		}
		if len(result.Items) < 200 {
			break
		}
	}
	b.fillOrgPlacement(ctx, capabilities)
	b.inferMissingRosterFields(capabilities)
	return capabilities, nil
}

// fillOrgPlacement batch-resolves company/department ancestors for capabilities
// that have a PositionID. Missing org nodes or a nil reader are non-fatal.
func (b *AgentCapabilityBuilder) fillOrgPlacement(ctx context.Context, caps []biz.AgentCapability) {
	if b == nil || b.org == nil || len(caps) == 0 {
		return
	}
	idSet := make(map[string]struct{})
	for _, cap := range caps {
		if id := strings.TrimSpace(cap.PositionID); id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return
	}
	nodes := loadOrgClosure(ctx, b.org, idSet)
	if len(nodes) == 0 {
		return
	}
	for i := range caps {
		place := placementFromNode(nodes, caps[i].PositionID)
		if place.DepartmentID == "" && place.PositionKey == "" {
			continue
		}
		if caps[i].PositionKey == "" {
			caps[i].PositionKey = place.PositionKey
		}
		caps[i].DepartmentID = place.DepartmentID
		caps[i].DepartmentKey = place.DepartmentKey
		caps[i].DepartmentName = place.DepartmentName
		caps[i].CompanyID = place.CompanyID
		caps[i].CompanyName = place.CompanyName
	}
}

func (b *AgentCapabilityBuilder) inferMissingRosterFields(caps []biz.AgentCapability) {
	for i := range caps {
		if strings.TrimSpace(caps[i].DomainPath) == "" {
			caps[i].DomainPath = biz.InferDomainPath(caps[i].PositionKey, caps[i].DepartmentKey, caps[i].DisplayName)
		}
		if strings.TrimSpace(caps[i].Mission) == "" {
			caps[i].Mission = biz.InferMissionStatement(caps[i].DisplayName, caps[i].Description)
		}
	}
}

type orgPlacement struct {
	PositionKey    string
	DepartmentID   string
	DepartmentKey  string
	DepartmentName string
	CompanyID      string
	CompanyName    string
}

func loadOrgClosure(ctx context.Context, org biz.OrganizationReader, seed map[string]struct{}) map[string]biz.OrganizationNode {
	out := make(map[string]biz.OrganizationNode)
	pending := make([]string, 0, len(seed))
	for id := range seed {
		pending = append(pending, id)
	}
	for len(pending) > 0 {
		missing := make([]string, 0, len(pending))
		for _, id := range pending {
			if _, ok := out[id]; !ok {
				missing = append(missing, id)
			}
		}
		pending = pending[:0]
		if len(missing) == 0 {
			break
		}
		rows, err := org.ListOrgNodesByIDs(ctx, missing)
		if err != nil {
			return out
		}
		for _, n := range rows {
			out[n.ID] = n
			if pid := strings.TrimSpace(n.ParentID); pid != "" {
				if _, seen := out[pid]; !seen {
					pending = append(pending, pid)
				}
			}
		}
	}
	return out
}

func placementFromNode(nodes map[string]biz.OrganizationNode, positionID string) orgPlacement {
	n, ok := nodes[strings.TrimSpace(positionID)]
	if !ok {
		return orgPlacement{}
	}
	var p orgPlacement
	switch n.Level {
	case "position":
		p.PositionKey = n.Key
		if dept, ok := nodes[n.ParentID]; ok && dept.Level == "department" {
			p.DepartmentID = dept.ID
			p.DepartmentKey = dept.Key
			p.DepartmentName = dept.Name
			if co, ok := nodes[dept.ParentID]; ok && co.Level == "company" {
				p.CompanyID = co.ID
				p.CompanyName = co.Name
			}
		}
	case "department":
		p.DepartmentID = n.ID
		p.DepartmentKey = n.Key
		p.DepartmentName = n.Name
		if co, ok := nodes[n.ParentID]; ok && co.Level == "company" {
			p.CompanyID = co.ID
			p.CompanyName = co.Name
		}
	case "company":
		p.CompanyID = n.ID
		p.CompanyName = n.Name
	}
	return p
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
