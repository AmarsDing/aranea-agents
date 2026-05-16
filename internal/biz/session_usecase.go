package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// Session mirrors legacy conversation sessions row.
type Session struct {
	ID                         string
	WorkspaceID                string
	UserID                     string
	OwnerType                  string
	AgentID                    string
	TeamID                     string
	Title                      string
	Summary                    string
	TagsJSON                   string
	DialogMode                 string
	DefaultProvider            string
	DefaultModel               string
	DefaultContextWindowTokens int
	LastProvider               string
	LastModel                  string
	LastContextWindowTokens    int
	Status                     string
	Visibility                 string
	MessageCount               int
	RunCount                   int
	ModelCallCount             int
	ToolCallCount              int
	SkillCallCount             int
	MCPCallCount               int
	InputTokens                int
	OutputTokens               int
	TotalTokens                int
	TotalCostMicroUSD          int64
	AvgLatencyMs               float64
	ErrorCount                 int
	ContextUsedTokens          int
	ContextUsedRatio           float64
	MaxContextUsedRatio        float64
	ContextStatus              string
	FirstMessageAt             string
	LastMessageAt              string
	LastRunAt                  string
	CreatedAt                  string
	UpdatedAt                  string
	ArchivedAt                 string
	DeletedAt                  string
	RunnerSnapshotJSON         string
	StateJSON                  string
	MetadataJSON               string
}

// SessionSearchQuery filters sessions（对齐遗留 REST query）.
type SessionSearchQuery struct {
	OwnerType     string
	AgentID       string
	TeamID        string
	Status        string
	ContextStatus string
	Keyword       string
	Limit         int
	Offset        int
	Page          int
	PageSize      int
}

// SessionListResult is paged session search output.
type SessionListResult struct {
	Items  []Session
	Total  int
	Limit  int
	Offset int
}

// ChatMessage is one messages row for timeline assembly.
type ChatMessage struct {
	ID               string
	SessionID        string
	ParentMessageID  string
	TurnIndex        int
	Role             string
	ContentMarkdown  string
	ModelName        string
	TokenIn          int
	TokenOut         int
	LatencyMS        int
	Status           string
	AttachmentsCount int
	OptionsJSON      string
	ErrorMessage     string
	CreatedAt        string
}

// ToolInvocationView loads fields needed for session timeline tool rows.
type ToolInvocationView struct {
	ID               string
	ToolKey          string
	ToolDisplayName  string
	AgentID          string
	AgentDisplayName string
	SessionID        string
	Source           string
	Status           string
	StartedAt        string
	EndedAt          string
	DurationMS       int
	InputPreview     string
	OutputPreview    string
	ErrorCode        string
	ErrorMessage     string
	MetadataJSON     string
	CreatedAt        string
}

// SkillInvocationView loads skill_invocation + resolved skill display name.
type SkillInvocationView struct {
	ID               string
	SkillID          string
	SkillName        string
	SkillVersion     string
	AgentID          string
	AgentDisplayName string
	SessionID        string
	Status           string
	DurationMS       int
	StartedAt        string
	EndedAt          string
	InputPreview     string
	OutputPreview    string
	ErrorCode        string
	ErrorMessage     string
}

// SessionTimelineItem matches legacy JSON timeline item shape.
type SessionTimelineItem struct {
	ID              string
	Kind            string
	Side            string
	Title           string
	Subtitle        string
	ActorID         string
	ActorName       string
	Status          string
	OccurredAt      string
	DurationMS      int
	ContentMarkdown string
	Preview         string
	DetailJSON      string
	Tags            []string
}

// SessionTimelineSummary aggregates timeline counts.
type SessionTimelineSummary struct {
	Total        int
	MessageCount int
	ToolCount    int
	SkillCount   int
	MCPCount     int
}

// SessionTimeline merges messages / tools / skills for GET timeline API.
type SessionTimeline struct {
	SessionID string
	Items     []SessionTimelineItem
	Summary   SessionTimelineSummary
}

// SessionRepository persists sessions and reads timeline inputs（SQLite Ent）.
type SessionRepository interface {
	SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	CreateSession(ctx context.Context, s Session) (Session, error)
	GetSessionByID(ctx context.Context, id string) (Session, error)
	UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
	ArchiveSession(ctx context.Context, id string) error
	DeleteSession(ctx context.Context, id string) error
	DeleteSessionsByAgentID(ctx context.Context, agentID string) error
	ListMessagesBySession(ctx context.Context, sessionID string) ([]ChatMessage, error)
	ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
	ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
	// AppendChatTurn inserts user + assistant rows and updates session aggregates (native chat).
	AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
	// AppendChatMessage inserts a single message row and updates session aggregates.
	AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	// UpdateRunnerSnapshotJSON persists Runner session snapshot (events + KV state).
	UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
	// UpdateSessionContextFromLLMUsage updates context bar fields from the latest model call (prompt vs context window).
	UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
	// UpdateSessionContextAfterCompression sets estimated prompt usage after ADK snapshot compaction (summary + tail).
	UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
	// Session summaries (session_summaries DDL via sessionmemory.EnsureSchema).
	InsertSessionSummary(ctx context.Context, row SessionSummary) error
	MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
	ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
	LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
	UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
	// GetSessionState reads the session state KV store (state_json column).
	GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
	// SaveSessionState writes the session state KV store (state_json column).
	SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
}

// SessionUsecase handles session CRUD + timeline（不包含发送消息）.
type SessionUsecase struct {
	sessions       SessionRepository
	agents         AgentRepository
	teams          TeamRepository
	titleGenerator SessionTitleGenerator
}

func NewSessionUsecase(sessions SessionRepository, agents AgentRepository, teams TeamRepository, titleGenerator SessionTitleGenerator) *SessionUsecase {
	if titleGenerator == nil {
		titleGenerator = NewNoopSessionTitleGenerator()
	}
	return &SessionUsecase{sessions: sessions, agents: agents, teams: teams, titleGenerator: titleGenerator}
}

func (uc *SessionUsecase) Search(ctx context.Context, q SessionSearchQuery) (SessionListResult, error) {
	normalizeSessionSearch(&q)
	return uc.sessions.SearchSessions(ctx, q)
}

func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error) {
	return uc.sessions.GetSessionByID(ctx, id)
}

func (uc *SessionUsecase) Create(ctx context.Context, in Session) (Session, error) {
	switch strings.TrimSpace(in.OwnerType) {
	case "team":
		if strings.TrimSpace(in.TeamID) == "" {
			return Session{}, validationErr("team_id is required")
		}
		if _, err := uc.teams.GetTeamByID(ctx, in.TeamID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Session{}, kerrors.NotFound("SESSION", fmt.Sprintf("team %q was not found", in.TeamID))
			}
			return Session{}, err
		}
	case "agent", "":
		if strings.TrimSpace(in.AgentID) == "" {
			return Session{}, validationErr("agent_id is required")
		}
		if _, err := uc.agents.GetAgentByID(ctx, in.AgentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Session{}, kerrors.NotFound("SESSION", fmt.Sprintf("agent %q was not found", in.AgentID))
			}
			return Session{}, err
		}
		in.OwnerType = "agent"
	default:
		return Session{}, validationErr("owner_type must be agent or team")
	}
	in.ID = uuid.NewString()
	return uc.sessions.CreateSession(ctx, in)
}

func (uc *SessionUsecase) Rename(ctx context.Context, id, title string) (Session, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Session{}, validationErr("title is required")
	}
	return uc.sessions.UpdateSessionTitle(ctx, id, title)
}

func (uc *SessionUsecase) Archive(ctx context.Context, id string) error {
	return uc.sessions.ArchiveSession(ctx, id)
}

func (uc *SessionUsecase) Delete(ctx context.Context, id string) error {
	return uc.sessions.DeleteSession(ctx, id)
}

func (uc *SessionUsecase) DeleteByAgent(ctx context.Context, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return validationErr("agent_id is required")
	}
	return uc.sessions.DeleteSessionsByAgentID(ctx, agentID)
}

// ListMessages returns raw chat rows for a session (same store as timeline messages).
func (uc *SessionUsecase) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, validationErr("session id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, sessionID); err != nil {
		return nil, err
	}
	return uc.sessions.ListMessagesBySession(ctx, sessionID)
}

// AppendChatTurn persists a user + assistant pair (native chat).
func (uc *SessionUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error {
	if err := uc.sessions.AppendChatTurn(ctx, sessionID, user, assistant); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(user.Role), "user") {
		_ = uc.maybeAutoTitleFromUserMessage(ctx, sessionID, user.ContentMarkdown)
	}
	return nil
}

// AppendChatMessage persists one chat row (streamed native turns).
func (uc *SessionUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	if err := uc.sessions.AppendChatMessage(ctx, sessionID, msg, bumpModelCall); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
		_ = uc.maybeAutoTitleFromUserMessage(ctx, sessionID, msg.ContentMarkdown)
	}
	return nil
}

// UpdateRunnerSnapshotJSON persists the Runner session snapshot.
func (uc *SessionUsecase) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	return uc.sessions.UpdateRunnerSnapshotJSON(ctx, sessionID, snapshotJSON)
}

func (uc *SessionUsecase) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	return uc.sessions.GetSessionState(ctx, sessionID)
}

func (uc *SessionUsecase) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	return uc.sessions.SaveSessionState(ctx, sessionID, state)
}

func (uc *SessionUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta EnvelopeStateDelta) error {
	if delta.Path == "" {
		return nil
	}
	state, err := uc.sessions.GetSessionState(ctx, sessionID)
	if err != nil {
		return err
	}
	switch delta.Operation {
	case "set":
		state[delta.Path] = delta.ValueJSON
	case "append":
		existing, _ := state[delta.Path]
		state[delta.Path] = existing + delta.ValueJSON
	case "delete":
		delete(state, delta.Path)
	default:
		state[delta.Path] = delta.ValueJSON
	}
	return uc.sessions.SaveSessionState(ctx, sessionID, state)
}

// UpdateSessionContextFromLLMUsage refreshes sessions.context_used_ratio after a native LLM turn.
func (uc *SessionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error {
	return uc.sessions.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTokens, completionTokens, contextWindow)
}

// UpdateSessionContextAfterCompression refreshes context_used_* from an estimate of the compacted prompt.
func (uc *SessionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	return uc.sessions.UpdateSessionContextAfterCompression(ctx, sessionID, estimatedPromptTokens, contextWindow)
}

// InsertSessionSummary appends a rolling summary row.
func (uc *SessionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	return uc.sessions.InsertSessionSummary(ctx, row)
}

// MaxSessionSummaryToTurn returns the largest to_turn stored for the session (0 if none).
func (uc *SessionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	return uc.sessions.MaxSessionSummaryToTurn(ctx, sessionID)
}

// ListSessionSummaries returns summary rows in chronological order.
func (uc *SessionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	return uc.sessions.ListSessionSummaries(ctx, sessionID)
}

// LatestSessionSummaryTime returns created_at of the newest summary row or empty string.
func (uc *SessionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	return uc.sessions.LatestSessionSummaryTime(ctx, sessionID)
}

// UpdateSessionListSummary updates sessions.summary (short UI line).
func (uc *SessionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	return uc.sessions.UpdateSessionListSummary(ctx, sessionID, summary)
}

func sessionTitleFromUserSnippet(snippet string) string {
	s := strings.TrimSpace(snippet)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 56 {
		return string(r[:56]) + "…"
	}
	return s
}

func shouldAutoNameSession(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return true
	}
	lower := strings.ToLower(t)
	switch lower {
	case "untitled", "new chat":
		return true
	}
	// Matches i18n `chat.untitledSession` (zh/en) and common placeholders.
	if strings.Contains(t, "未命名") || strings.Contains(t, "新会话") || strings.Contains(t, "新对话") {
		return true
	}
	return false
}

func (uc *SessionUsecase) maybeAutoTitleFromUserMessage(ctx context.Context, sessionID, content string) error {
	sess, err := uc.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !shouldAutoNameSession(sess.Title) {
		return nil
	}
	snippet := sessionTitleFromUserSnippet(content)
	if snippet != "" {
		_, _ = uc.Rename(ctx, sessionID, snippet)
	}
	go uc.generateTitleAsync(sessionID, content)
	return nil
}

func (uc *SessionUsecase) generateTitleAsync(sessionID, content string) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	title, err := uc.titleGenerator.Generate(bgCtx, content)
	if err != nil || title == "" {
		return
	}
	_, _ = uc.Rename(bgCtx, sessionID, title)
}

func (uc *SessionUsecase) Timeline(ctx context.Context, id string) (SessionTimeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionTimeline{}, validationErr("session id is required")
	}
	if _, err := uc.sessions.GetSessionByID(ctx, id); err != nil {
		return SessionTimeline{}, err
	}

	messages, err := uc.sessions.ListMessagesBySession(ctx, id)
	if err != nil {
		return SessionTimeline{}, err
	}
	tools, err := uc.sessions.ListToolInvocationsBySession(ctx, id, 100)
	if err != nil {
		return SessionTimeline{}, err
	}
	skills, err := uc.sessions.ListSkillInvocationsBySession(ctx, id, 100)
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
		return items[i].OccurredAt < items[j].OccurredAt
	})

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

	return SessionTimeline{
		SessionID: id,
		Items:     items,
		Summary:   summary,
	}, nil
}

func normalizeSessionSearch(q *SessionSearchQuery) {
	if q.PageSize > 0 {
		q.Limit = q.PageSize
		q.Offset = (q.Page - 1) * q.PageSize
		if q.Page <= 0 {
			q.Offset = 0
		}
	}
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 20
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
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
