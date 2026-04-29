package application

import (
	"arenea/backend/internal/kernel/errs"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type SessionService struct {
	repo repository.Store
}

func NewSessionService(repo repository.Store) *SessionService {
	return &SessionService{repo: repo}
}

func (s *SessionService) Create(in domain.Session) (domain.Session, error) {
	switch in.OwnerType {
	case "team":
		if in.TeamID == "" {
			return domain.Session{}, validationError("team_id is required")
		}
		if _, err := s.repo.GetTeamByID(in.TeamID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Session{}, fmt.Errorf("%w: team %q was not found", errs.ErrNotFound, in.TeamID)
			}
			return domain.Session{}, err
		}
	case "agent", "":
		if in.AgentID == "" {
			return domain.Session{}, validationError("agent_id is required")
		}
		if _, err := s.repo.GetAgentByID(in.AgentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Session{}, fmt.Errorf("%w: agent %q was not found", errs.ErrNotFound, in.AgentID)
			}
			return domain.Session{}, err
		}
	default:
		return domain.Session{}, validationError("owner_type must be agent or team")
	}
	in.ID = newID()
	return s.repo.CreateSession(in)
}

func (s *SessionService) Get(id string) (domain.Session, error) {
	return s.repo.GetSessionByID(id)
}

func (s *SessionService) List(agentID string) ([]domain.Session, error) {
	return s.repo.ListSessions(agentID)
}

func (s *SessionService) ListTeam(teamID string) ([]domain.Session, error) {
	return s.repo.ListTeamSessions(teamID)
}

func (s *SessionService) Search(query domain.SessionSearchQuery) (domain.SessionListResult, error) {
	return s.repo.SearchSessions(query)
}

func (s *SessionService) Timeline(id string) (domain.SessionTimeline, error) {
	if strings.TrimSpace(id) == "" {
		return domain.SessionTimeline{}, validationError("session id is required")
	}
	if _, err := s.repo.GetSessionByID(id); err != nil {
		return domain.SessionTimeline{}, err
	}

	items := []domain.SessionTimelineItem{}
	messages, err := s.repo.ListMessages(id)
	if err != nil {
		return domain.SessionTimeline{}, err
	}
	for _, msg := range messages {
		items = append(items, messageTimelineItem(msg))
	}

	tools, err := s.repo.SearchToolInvocations(domain.ToolRunQuery{SessionID: id, Limit: 500})
	if err != nil {
		return domain.SessionTimeline{}, err
	}
	for _, tool := range tools.Items {
		items = append(items, toolTimelineItem(tool))
	}

	skills, err := s.repo.SearchSkillInvocations(domain.SkillRunQuery{SessionID: id, Limit: 500})
	if err != nil {
		return domain.SessionTimeline{}, err
	}
	for _, skill := range skills.Items {
		items = append(items, skillTimelineItem(skill))
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OccurredAt < items[j].OccurredAt
	})

	summary := domain.SessionTimelineSummary{Total: len(items)}
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

	return domain.SessionTimeline{SessionID: id, Items: items, Summary: summary}, nil
}

func (s *SessionService) Rename(id string, title string) (domain.Session, error) {
	return s.repo.UpdateSessionTitle(id, title)
}

func (s *SessionService) Archive(id string) error {
	return s.repo.ArchiveSession(id)
}

func (s *SessionService) Delete(id string) error {
	return s.repo.DeleteSession(id)
}

func (s *SessionService) DeleteByAgent(agentID string) error {
	return s.repo.DeleteSessionsByAgentID(agentID)
}

func messageTimelineItem(msg domain.Message) domain.SessionTimelineItem {
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
	return domain.SessionTimelineItem{
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
		ContentMarkdown: msg.Content,
		Preview:         previewTimelineText(msg.Content, 180),
		DetailJSON:      msg.OptionsJSON,
		Tags:            tags,
	}
}

func toolTimelineItem(run domain.ToolInvocation) domain.SessionTimelineItem {
	kind := "tool"
	tags := []string{"Tool"}
	if strings.EqualFold(run.Source, "mcp") || strings.Contains(strings.ToLower(run.ToolKey), "mcp") {
		kind = "mcp"
		tags = []string{"MCP"}
	}
	title := timelineFirstNonEmpty(run.ToolDisplayName, run.ToolKey, "工具调用")
	detail := marshalTimelineDetail(map[string]any{
		"input_preview":     run.InputPreview,
		"output_preview":    run.OutputPreview,
		"error_code":        run.ErrorCode,
		"error_message":     run.ErrorMessage,
		"redaction_applied": run.RedactionApplied,
		"metadata_json":     run.MetadataJSON,
		"request_id":        run.RequestID,
		"invocation_id":     run.InvocationID,
	})
	return domain.SessionTimelineItem{
		ID:         timelineFirstNonEmpty(run.ID, run.InvocationID),
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

func skillTimelineItem(run domain.SkillInvocation) domain.SessionTimelineItem {
	detail := marshalTimelineDetail(map[string]any{
		"input_preview":  run.InputPreview,
		"output_preview": run.OutputPreview,
		"error_code":     run.ErrorCode,
		"error_message":  run.ErrorMessage,
		"skill_version":  run.SkillVersion,
	})
	return domain.SessionTimelineItem{
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
