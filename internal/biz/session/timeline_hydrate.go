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
