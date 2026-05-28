package session

import (
	"context"
)

func (uc *SessionUsecase) hydrateTimelineRefs(ctx context.Context, sessionID string, refs []TimelineEventRef) ([]SessionTimelineItem, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	msgIDs := make([]string, 0)
	toolIDs := make([]string, 0)
	skillIDs := make([]string, 0)
	for _, ref := range refs {
		switch ref.Kind {
		case "message":
			msgIDs = append(msgIDs, ref.ID)
		case "tool", "mcp":
			toolIDs = append(toolIDs, ref.ID)
		case "skill":
			skillIDs = append(skillIDs, ref.ID)
		}
	}

	msgByID := map[string]ChatMessage{}
	if len(msgIDs) > 0 {
		rows, err := uc.sessions.ListMessagesByIDs(ctx, sessionID, msgIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			msgByID[row.ID] = row
		}
	}
	toolByID := map[string]ToolInvocationView{}
	if len(toolIDs) > 0 {
		rows, err := uc.sessions.ListToolInvocationsByIDs(ctx, sessionID, toolIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			toolByID[row.ID] = row
		}
	}
	skillByID := map[string]SkillInvocationView{}
	if len(skillIDs) > 0 {
		rows, err := uc.sessions.ListSkillInvocationsByIDs(ctx, sessionID, skillIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			skillByID[row.ID] = row
		}
	}

	agentIDs := make([]string, 0)
	seen := map[string]struct{}{}
	collectAgentID := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		agentIDs = append(agentIDs, id)
	}
	for _, tool := range toolByID {
		collectAgentID(tool.AgentID)
	}
	for _, skill := range skillByID {
		collectAgentID(skill.AgentID)
	}
	agentNames := map[string]string{}
	if len(agentIDs) > 0 {
		names, err := uc.sessions.LookupAgentDisplayNames(ctx, agentIDs)
		if err != nil {
			return nil, err
		}
		agentNames = names
	}
	for id, tool := range toolByID {
		tool.AgentDisplayName = agentNames[tool.AgentID]
		toolByID[id] = tool
	}
	for id, skill := range skillByID {
		skill.AgentDisplayName = agentNames[skill.AgentID]
		skillByID[id] = skill
	}

	items := make([]SessionTimelineItem, 0, len(refs))
	for _, ref := range refs {
		switch ref.Kind {
		case "message":
			if msg, ok := msgByID[ref.ID]; ok {
				items = append(items, messageTimelineItem(msg))
			}
		case "tool", "mcp":
			if tool, ok := toolByID[ref.ID]; ok {
				items = append(items, toolTimelineItem(tool))
			}
		case "skill":
			if skill, ok := skillByID[ref.ID]; ok {
				items = append(items, skillTimelineItem(skill))
			}
		}
	}
	return items, nil
}
