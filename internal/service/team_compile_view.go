package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

func (s *TeamService) compileAgentDisplayNameResolver(ctx context.Context) func(agentID string) string {
	if s.agents == nil {
		return nil
	}
	return func(agentID string) string {
		ag, err := s.agents.Get(ctx, strings.TrimSpace(agentID))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(ag.DisplayName)
	}
}

func buildCompiledGraphNodeView(
	n biz.NodeDef,
	meta biz.NodeTaskMeta,
	metaOK bool,
	member team.MemberDef,
	memberOK bool,
	displayName func(agentID string) string,
) *v1.CompiledGraphNodeView {
	taskPrompt := strings.TrimSpace(n.Instruction)
	agentDisplayName := strings.TrimSpace(n.Description)
	if memberOK {
		if taskPrompt == "" {
			taskPrompt = strings.TrimSpace(member.TaskPrompt)
		}
		if dn := strings.TrimSpace(member.Name); dn != "" {
			agentDisplayName = dn
		} else if displayName != nil {
			if dn := displayName(member.AgentID); dn != "" {
				agentDisplayName = dn
			}
		}
	}
	if agentDisplayName == "" && displayName != nil && memberOK {
		if dn := displayName(member.AgentID); dn != "" {
			agentDisplayName = dn
		}
	}
	if agentDisplayName == "" {
		agentDisplayName = strings.TrimSpace(n.AgentName)
	}
	role := ""
	if metaOK {
		role = meta.RequiredRole
	}
	return &v1.CompiledGraphNodeView{
		Id:               n.ID,
		Type:             n.Type,
		AgentName:        n.AgentName,
		Role:             role,
		Description:      n.Description,
		AgentDisplayName: agentDisplayName,
		TaskPrompt:       taskPrompt,
	}
}

func (s *TeamService) buildCompiledGraphNodeViews(
	ctx context.Context,
	def team.Definition,
	nodes []biz.NodeDef,
	taskMeta map[string]biz.NodeTaskMeta,
) []*v1.CompiledGraphNodeView {
	memberByNode := team.MemberByCompileNodeID(def)
	displayName := s.compileAgentDisplayNameResolver(ctx)
	views := make([]*v1.CompiledGraphNodeView, 0, len(nodes))
	for _, n := range nodes {
		member, ok := memberByNode[n.ID]
		meta, metaOK := taskMeta[n.ID]
		views = append(views, buildCompiledGraphNodeView(n, meta, metaOK, member, ok, displayName))
	}
	return views
}
