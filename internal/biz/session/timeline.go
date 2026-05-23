package session

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (uc *SessionUsecase) Timeline(ctx context.Context, id string, q TimelineQuery) (SessionTimeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionTimeline{}, validationErr("session id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, id); err != nil {
		return SessionTimeline{}, err
	}

	msgFetch := timelineMessageFetchLimit(q)
	messages, err := uc.sessions.ListMessagesRecent(ctx, id, msgFetch)
	if err != nil {
		return SessionTimeline{}, err
	}
	invLimit := timelineInvocationLimit(q)
	tools, err := uc.sessions.ListToolInvocationsBySession(ctx, id, invLimit)
	if err != nil {
		return SessionTimeline{}, err
	}
	skills, err := uc.sessions.ListSkillInvocationsBySession(ctx, id, invLimit)
	if err != nil {
		return SessionTimeline{}, err
	}

	items := make([]SessionTimelineItem, 0, len(messages)+len(tools)+len(skills))
	for _, msg := range messages {
		items = append(items, messageTimelineItem(msg))
	}
	for _, t := range tools {
		items = append(items, toolTimelineItem(t))
	}
	for _, s := range skills {
		items = append(items, skillTimelineItem(s))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if q.SortOrder == "desc" {
			return items[i].OccurredAt > items[j].OccurredAt
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})

	if q.KindFilter != "" {
		filtered := make([]SessionTimelineItem, 0, len(items))
		for _, item := range items {
			if item.Kind == q.KindFilter {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	summary := SessionTimelineSummary{Total: len(items)}
	for _, item := range items {
		switch item.Kind {
		case "message":
			summary.MessageCount++
		case "tool":
			summary.ToolCount++
		case "skill":
			summary.SkillCount++
		case "mcp":
			summary.MCPCount++
		}
	}

	if q.Limit > 0 || q.Offset > 0 {
		limit := q.Limit
		if limit <= 0 {
			limit = len(items)
		}
		offset := q.Offset
		if offset < 0 {
			offset = 0
		}
		if offset > len(items) {
			offset = len(items)
		}
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		items = items[offset:end]
	}

	return SessionTimeline{
		SessionID: id,
		Items:     items,
		Summary:   summary,
	}, nil
}

func validationErr(msg string) error {
	return kerrors.BadRequest("SESSION", msg)
}

func messageTimelineItem(msg ChatMessage) SessionTimelineItem {
	title := "Agent 消息"
	subtitle := msg.Role
	tags := []string{"Agent"}
	actorID := ""
	actorName := ""
	if msg.Role == "user" {
		title = "用户消息"
		tags = []string{"User"}
		actorName = "User"
	} else if strings.EqualFold(msg.Role, "system") {
		title = "系统消息"
		tags = []string{"System"}
	}
	var opts struct {
		AgentID string `json:"agent_id"`
		Name    string `json:"name"`
		Agent   struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"agent"`
		TeamMember struct {
			AgentID string `json:"agent_id"`
			Name    string `json:"name"`
			Role    string `json:"role"`
		} `json:"team_member"`
	}
	if msg.OptionsJSON != "" && json.Unmarshal([]byte(msg.OptionsJSON), &opts) == nil {
		actorID = timelineFirstNonEmpty(opts.TeamMember.AgentID, opts.Agent.ID, opts.AgentID)
		actorName = timelineFirstNonEmpty(opts.TeamMember.Name, opts.Agent.DisplayName, opts.Name, actorName)
		if opts.TeamMember.Name != "" {
			title = opts.TeamMember.Name
			tags = []string{"Team"}
			if opts.TeamMember.Role != "" {
				subtitle = opts.TeamMember.Role
			}
		} else if actorName != "" && msg.Role != "user" {
			title = actorName
		}
	}
	status := msg.Status
	if status == "" {
		status = "ok"
	}
	return SessionTimelineItem{
		ID:              msg.ID,
		Kind:            "message",
		Side:            "left",
		Title:           title,
		Subtitle:        subtitle,
		ActorID:         actorID,
		ActorName:       actorName,
		Status:          status,
		OccurredAt:      msg.CreatedAt,
		DurationMS:      msg.LatencyMS,
		ContentMarkdown: msg.ContentMarkdown,
		Preview:         previewTimelineText(msg.ContentMarkdown, 180),
		DetailJSON:      msg.OptionsJSON,
		Tags:            tags,
	}
}

func toolTimelineItem(run ToolInvocationView) SessionTimelineItem {
	kind := "tool"
	tags := []string{"Tool"}
	if strings.EqualFold(run.Source, "mcp") || strings.Contains(strings.ToLower(run.ToolKey), "mcp") {
		kind = "mcp"
		tags = []string{"MCP"}
	}
	title := timelineFirstNonEmpty(run.ToolDisplayName, run.ToolKey, "工具调用")
	detail := marshalTimelineDetail(map[string]any{
		"input_preview":  run.InputPreview,
		"output_preview": run.OutputPreview,
		"error_code":     run.ErrorCode,
		"error_message":  run.ErrorMessage,
		"metadata_json":  run.MetadataJSON,
		"invocation_id":  run.ID,
	})
	return SessionTimelineItem{
		ID:         timelineFirstNonEmpty(run.ID, ""),
		Kind:       kind,
		Side:       "right",
		Title:      title,
		Subtitle:   run.ToolKey,
		ActorID:    run.AgentID,
		ActorName:  run.AgentDisplayName,
		Status:     timelineFirstNonEmpty(run.Status, "success"),
		OccurredAt: timelineFirstNonEmpty(run.StartedAt, run.CreatedAt),
		DurationMS: run.DurationMS,
		Preview:    previewTimelineText(timelineFirstNonEmpty(run.InputPreview, run.OutputPreview, run.ErrorMessage), 180),
		DetailJSON: detail,
		Tags:       tags,
	}
}

func skillTimelineItem(run SkillInvocationView) SessionTimelineItem {
	detail := marshalTimelineDetail(map[string]any{
		"input_preview":  run.InputPreview,
		"output_preview": run.OutputPreview,
		"error_code":     run.ErrorCode,
		"error_message":  run.ErrorMessage,
		"skill_version":  run.SkillVersion,
	})
	return SessionTimelineItem{
		ID:         run.ID,
		Kind:       "skill",
		Side:       "right",
		Title:      timelineFirstNonEmpty(run.SkillName, "Skill 调用"),
		Subtitle:   run.SkillVersion,
		ActorID:    run.AgentID,
		ActorName:  run.AgentDisplayName,
		Status:     timelineFirstNonEmpty(run.Status, "success"),
		OccurredAt: run.StartedAt,
		DurationMS: run.DurationMS,
		Preview:    previewTimelineText(timelineFirstNonEmpty(run.InputPreview, run.OutputPreview, run.ErrorMessage), 180),
		DetailJSON: detail,
		Tags:       []string{"Skill"},
	}
}

func marshalTimelineDetail(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func previewTimelineText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if maxRunes <= 0 || len([]rune(value)) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes]) + "..."
}

func timelineFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
