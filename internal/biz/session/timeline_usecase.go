package session

import (
	"context"
	"sort"
	"strings"

	"github.com/google/wire"
)

// SessionTimelineUsecase handles session timeline assembly (messages + tools + skills).
// Extracted from SessionUsecase to reduce God Object scope.
// Stability:evolving
type SessionTimelineUsecase struct {
	timelineReader   TimelineReader
	invocationReader InvocationReader
	messageReader    MessageReader
	sessionReader    SessionReader
}

// NewSessionTimelineUsecase creates a new SessionTimelineUsecase.
func NewSessionTimelineUsecase(timelineReader TimelineReader, invocationReader InvocationReader, messageReader MessageReader, sessionReader SessionReader) *SessionTimelineUsecase {
	return &SessionTimelineUsecase{
		timelineReader:   timelineReader,
		invocationReader: invocationReader,
		messageReader:    messageReader,
		sessionReader:    sessionReader,
	}
}

func (uc *SessionTimelineUsecase) Timeline(ctx context.Context, id string, q TimelineQuery) (SessionTimeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionTimeline{}, validationErr("session id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return SessionTimeline{}, err
	}
	if strings.TrimSpace(q.KindFilter) == "message" {
		return uc.timelineMessagesOnly(ctx, sess, q)
	}
	return uc.timelineUnionPaged(ctx, sess, q)
}

func (uc *SessionTimelineUsecase) timelineMessagesOnly(ctx context.Context, sess Session, q TimelineQuery) (SessionTimeline, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = MessageListDefaultLimit
	}
	if limit > MessageListMaxLimit {
		limit = MessageListMaxLimit
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	total, err := uc.messageReader.CountMessagesBySession(ctx, sess.ID)
	if err != nil {
		return SessionTimeline{}, err
	}
	items_data, err := uc.messageReader.ListMessagesBySession(ctx, sess.ID, limit, offset)
	if err != nil {
		return SessionTimeline{}, err
	}
	items := make([]SessionTimelineItem, 0, len(items_data))
	for _, msg := range items_data {
		items = append(items, messageTimelineItem(msg))
	}
	if q.SortOrder == "desc" {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].OccurredAt > items[j].OccurredAt
		})
	}
	summary := sessionTimelineSummary(sess, nil)
	summary.MessageCount = total
	summary.Total = total
	return SessionTimeline{
		SessionID: sess.ID,
		Items:     items,
		Summary:   summary,
	}, nil
}

func (uc *SessionTimelineUsecase) timelineUnionPaged(ctx context.Context, sess Session, q TimelineQuery) (SessionTimeline, error) {
	refs, total, err := uc.timelineReader.ListTimelineEventRefsPaged(ctx, sess.ID, q)
	if err != nil {
		return SessionTimeline{}, err
	}
	items, err := uc.hydrateTimelineRefs(ctx, sess.ID, refs)
	if err != nil {
		return SessionTimeline{}, err
	}
	summary := sessionTimelineSummary(sess, nil)
	summary.Total = total
	switch strings.TrimSpace(q.KindFilter) {
	case "tool":
		summary.ToolCount = total
	case "skill":
		summary.SkillCount = total
	case "mcp":
		summary.MCPCount = total
	}
	return SessionTimeline{
		SessionID: sess.ID,
		Items:     items,
		Summary:   summary,
	}, nil
}

func (uc *SessionTimelineUsecase) hydrateTimelineRefs(ctx context.Context, sessionID string, refs []TimelineEventRef) ([]SessionTimelineItem, error) {
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
		rows, err := uc.messageReader.ListMessagesByIDs(ctx, sessionID, msgIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			msgByID[row.ID] = row
		}
	}
	toolByID := map[string]ToolInvocationView{}
	if len(toolIDs) > 0 {
		rows, err := uc.timelineReader.ListToolInvocationsByIDs(ctx, sessionID, toolIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			toolByID[row.ID] = row
		}
	}
	skillByID := map[string]SkillInvocationView{}
	if len(skillIDs) > 0 {
		rows, err := uc.timelineReader.ListSkillInvocationsByIDs(ctx, sessionID, skillIDs)
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
		names, err := uc.timelineReader.LookupAgentDisplayNames(ctx, agentIDs)
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

// SessionTimelineProviderSet provides Wire bindings for SessionTimelineUsecase.
var SessionTimelineProviderSet = wire.NewSet(NewSessionTimelineUsecase)
