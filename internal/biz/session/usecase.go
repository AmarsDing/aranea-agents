package session

import (
	"context"
	"database/sql"
	"errors"
	"strings"

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
	PinnedAt                   string
	RunnerSnapshotJSON         string
	StateJSON                  string
	MetadataJSON               string
	SessionRevision            int64
}

// SessionSearchQuery filters sessions（对齐遗留 REST query）.
type SessionSearchQuery struct {
	OwnerType     string
	AgentID       string
	TeamID        string
	Status        string
	ContextStatus string
	Keyword       string
	UserID        string
	Limit         int
	Offset        int
	Page          int
	PageSize      int
	SortBy        string
	SortOrder     string
}

// SessionListResult is paged session search output.
type SessionListResult struct {
	Items  []Session
	Total  int
	Limit  int
	Offset int
}

// MessageSearchQuery filters message full-text search.
type MessageSearchQuery struct {
	SessionID string
	Keyword   string
	Limit     int
	Offset    int
}

// MessageSearchHit is one search result row.
type MessageSearchHit struct {
	ID              string
	SessionID       string
	Role            string
	ContentMarkdown string
	Highlight       string
	CreatedAt       string
}

// MessageSearchResult is paginated message search output.
type MessageSearchResult struct {
	Items []MessageSearchHit
	Total int
}

// MessageListResult is paginated session messages (DB limit/offset).
type MessageListResult struct {
	Items []ChatMessage
	Total int
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

// SessionUpdateFields holds optional fields for a partial session update.
type SessionUpdateFields struct {
	Title           *string
	TagsJSON        *string
	Visibility      *string
	MetadataJSON    *string
	DialogMode      *string
	DefaultProvider *string
	DefaultModel    *string
}

// TimelineQuery holds optional pagination and filter parameters for timeline.
type TimelineQuery struct {
	Limit      int
	Offset     int
	KindFilter string
	SortOrder  string
}

// TimelineEventRef is one row key from the merged timeline UNION query.
type TimelineEventRef struct {
	Kind       string
	ID         string
	OccurredAt string
}

const (
	timelineDefaultInvLimit = 100
	timelineMaxInvLimit     = 500

	MessageListDefaultLimit = 100
	MessageListMaxLimit     = 500
	TimelineMessageMaxFetch = 2000
	CompressMessageMaxRows  = 512
	ActivityCancelScanLimit = 64
)

type SessionTurn struct {
	ID                  string
	SessionID           string
	RunID               string
	TurnIndex           int
	UserMessageID       string
	AssistantMessageID  string
	OwnerType           string
	AgentID             string
	TeamID              string
	Status              string
	StartedAt           string
	EndedAt             string
	DurationMs          int
	FirstTokenMs        int
	ModelCallCount      int
	ToolCallCount       int
	SkillCallCount      int
	MCPCallCount        int
	InputTokens         int
	OutputTokens        int
	TotalTokens         int
	TotalCostMicroUSD   int64
	FinalProvider       string
	FinalModel          string
	FinalContentPreview string
	ErrorCode           string
	ErrorMessage        string
	MetadataJSON        string
	CreatedAt           string
	UpdatedAt           string
}

type SessionTurnListResult struct {
	Items []SessionTurn
	Total int
}

type SessionTurnUpdateFields struct {
	Status              *string
	EndedAt             *string
	UserMessageID       *string
	AssistantMessageID  *string
	OwnerType           *string
	AgentID             *string
	TeamID              *string
	DurationMs          *int
	FirstTokenMs        *int
	ModelCallCount      *int
	ToolCallCount       *int
	SkillCallCount      *int
	MCPCallCount        *int
	InputTokens         *int
	OutputTokens        *int
	TotalTokens         *int
	TotalCostMicroUSD   *int64
	FinalProvider       *string
	FinalModel          *string
	FinalContentPreview *string
	ErrorCode           *string
	ErrorMessage        *string
	MetadataJSON        *string
}

// SessionRepository persists sessions and reads timeline inputs（SQLite Ent）.
type SessionRepository interface {
	SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	CreateSession(ctx context.Context, s Session) (Session, error)
	GetSessionByID(ctx context.Context, id string) (Session, error)
	UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
	UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
	RestoreSession(ctx context.Context, id string) (Session, error)
	ArchiveSession(ctx context.Context, id string) error
	DeleteSession(ctx context.Context, id string) error
	DeleteSessionsByAgentID(ctx context.Context, agentID string) error
	CountMessagesBySession(ctx context.Context, sessionID string) (int, error)
	ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
	ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error)
	ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error)
	ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
	ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
	ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
	ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
	ListMessagesByIDs(ctx context.Context, sessionID string, ids []string) ([]ChatMessage, error)
	ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error)
	ListSkillInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error)
	// AppendChatTurn inserts user + assistant rows and updates session aggregates (native chat).
	AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
	// AppendChatMessage inserts a single message row and updates session aggregates.
	AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	// UpdateMessageFeedbackJSON merges user feedback into messages.options_json for an assistant row.
	UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error
	// UpsertChatActivityMessage inserts or updates a chat.activity/v1 row keyed by message id.
	UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) error
	// UpdateRunnerSnapshotJSON persists Runner session snapshot (events + KV state).
	UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
	// UpdateSessionContextFromLLMUsage updates context bar fields from the latest model call (prompt vs context window).
	UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
	// UpdateSessionContextAfterCompression sets estimated prompt usage after ADK snapshot compaction (summary + tail).
	UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
	// Session summaries (session_summaries DDL via data.EnsureSessionMemorySchema).
	InsertSessionSummary(ctx context.Context, row SessionSummary) error
	MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
	ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
	LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
	UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
	// GetSessionState reads the session state KV store (state_json column).
	GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
	// SaveSessionState writes the session state KV store (state_json column).
	SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
	// Session turns (session_turns table).
	CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
	UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
	ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
	GetSessionTurn(ctx context.Context, id string) (SessionTurn, error)
	// IncrementInvocationCounts bumps session-level tool / MCP / skill counters after a runtime invocation.
	IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
	// ListSessionsForBatch returns sessions matching search scope with limit/offset pagination.
	ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)
	ListSessionsByIDs(ctx context.Context, ids []string) ([]Session, error)
	ArchiveSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
	DeleteSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
	PinSession(ctx context.Context, id string) (Session, error)
	UnpinSession(ctx context.Context, id string) (Session, error)
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
	GetSessionRevision(ctx context.Context, sessionID string) (int64, error)
	ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error)
}

// AgentLookup checks agent existence (decoupled from biz.AgentRepository).
type AgentLookup interface {
	GetAgentByID(ctx context.Context, id string) (struct{}, error)
}

// TeamLookup checks team existence (decoupled from biz.TeamRepository).
type TeamLookup interface {
	GetTeamByID(ctx context.Context, id string) (struct{}, error)
}

// SessionUsecase handles session CRUD + timeline. Chat 写消息经 AppendChat* 等仓储方法，不经 SessionService RPC.
type SessionUsecase struct {
	sessions       SessionRepository
	agents         AgentLookup
	teams          TeamLookup
	titleGenerator SessionTitleGenerator
}

func NewSessionUsecase(sessions SessionRepository, agents AgentLookup, teams TeamLookup, titleGenerator SessionTitleGenerator) *SessionUsecase {
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
				return Session{}, kerrors.NotFound("SESSION", "team not found")
			}
			return Session{}, err
		}
	case "agent", "":
		if strings.TrimSpace(in.AgentID) == "" {
			return Session{}, validationErr("agent_id is required")
		}
		if _, err := uc.agents.GetAgentByID(ctx, in.AgentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Session{}, kerrors.NotFound("SESSION", "agent not found")
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

func (uc *SessionUsecase) Update(ctx context.Context, id string, fields SessionUpdateFields) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, validationErr("session id is required")
	}
	if fields.Title != nil {
		v := strings.TrimSpace(*fields.Title)
		fields.Title = &v
	}
	return uc.sessions.UpdateSession(ctx, id, fields)
}

func (uc *SessionUsecase) Restore(ctx context.Context, id string) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, validationErr("session id is required")
	}
	return uc.sessions.RestoreSession(ctx, id)
}

func (uc *SessionUsecase) Archive(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationErr("session id is required")
	}
	sess, err := uc.sessions.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if sess.Status == "running" {
		return validationErr("running session cannot be archived")
	}
	if sess.Status == "archived" {
		return nil
	}
	return uc.sessions.ArchiveSession(ctx, id)
}

func (uc *SessionUsecase) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationErr("session id is required")
	}
	sess, err := uc.sessions.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if sess.Status == "running" {
		return validationErr("running session cannot be deleted")
	}
	if strings.TrimSpace(sess.DeletedAt) != "" {
		return nil
	}
	return uc.sessions.DeleteSession(ctx, id)
}

func (uc *SessionUsecase) DeleteByAgent(ctx context.Context, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return validationErr("agent_id is required")
	}
	return uc.sessions.DeleteSessionsByAgentID(ctx, agentID)
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
