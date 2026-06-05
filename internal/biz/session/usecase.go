package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"

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
	StatusReason               string
	StatusChangedAt            string
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
	CompressVersion            int64
	ParentSessionID            string
	RootSessionID              string
	AgentDepth                 int
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
	TurnID           string
	TurnNumber       int
	SeqInTurn        int
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
	Status          *string
	StatusReason    *string
	StatusChangedAt *string
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
	TurnNumber          int
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

type SessionReader interface {
	SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	GetSessionByID(ctx context.Context, id string) (Session, error)
	GetSessionRevision(ctx context.Context, sessionID string) (int64, error)
	ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error)
	ListSessionsByIDs(ctx context.Context, ids []string) ([]Session, error)
}

type SessionTreeReader interface {
	ListByParentSessionID(ctx context.Context, parentSessionID string) ([]Session, error)
}

type SessionWriter interface {
	CreateSession(ctx context.Context, s Session) (Session, error)
	UpdateSessionTitle(ctx context.Context, id, title string) (Session, error)
	UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
	RestoreSession(ctx context.Context, id string) (Session, error)
	BumpSessionRevision(ctx context.Context, sessionID string) (int64, error)
}

type SessionMutator interface {
	ArchiveSession(ctx context.Context, id string) (int, error)
	DeleteSession(ctx context.Context, id string) (int, error)
	DeleteSessionsByAgentID(ctx context.Context, agentID string) error
	PinSession(ctx context.Context, id string) (Session, error)
	UnpinSession(ctx context.Context, id string) (Session, error)
}

type SessionBatchMutator interface {
	ArchiveSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
	DeleteSessionsByIDs(ctx context.Context, ids []string) (processed int, failed []string, err error)
}

type MessageReader interface {
	CountMessagesBySession(ctx context.Context, sessionID string) (int, error)
	ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
	ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error)
	ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	ListMessagesByIDs(ctx context.Context, sessionID string, ids []string) ([]ChatMessage, error)
}

type MessageSearchReader interface {
	ListMessagesByStatus(ctx context.Context, sessionID, status string, limit int) ([]ChatMessage, error)
	SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
	ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error)
}

type MessageWriter interface {
	AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
	AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error
	UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) (bool, error)
}

type TimelineReader interface {
	ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
	ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error)
	ListSkillInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error)
	LookupAgentDisplayNames(ctx context.Context, agentIDs []string) (map[string]string, error)
}

type InvocationReader interface {
	ListToolInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]ToolInvocationView, error)
	ListSkillInvocationsBySession(ctx context.Context, sessionID string, limit int) ([]SkillInvocationView, error)
}

type SummaryReader interface {
	MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error)
	ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error)
	LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error)
}

type SummaryWriter interface {
	InsertSessionSummary(ctx context.Context, row SessionSummary) error
	UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
	SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error)
}

type StateRepo interface {
	GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
	SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
	PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error
}

type TurnRepo interface {
	CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error)
	UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
	ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
	GetSessionTurn(ctx context.Context, id string) (SessionTurn, error)
}

type ContextUpdater interface {
	UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
	UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error
	UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error
	IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
	ApplyMetricsDelta(ctx context.Context, d *SessionMetricsDelta) error
}

type CompressRepo interface {
	TryIncrementCompressVersion(ctx context.Context, sessionID string) (oldVersion int64, err error)
	CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error
}

// SessionRepo aggregates all session sub-repositories for Wire binding only.
// Consumers should depend on the specific sub-interface they need.
// TECH-DEBT: SessionRepo aggregates 17+ methods; should be split into sub-interfaces.
// New code should depend on narrow interfaces like SessionRuntimeWriter, SessionMetricsReader, etc.
type SessionRepo interface {
	SessionReader
	SessionTreeReader
	SessionWriter
	SessionMutator
	SessionBatchMutator
	MessageReader
	MessageSearchReader
	MessageWriter
	MessageStatusWriter
	TimelineReader
	InvocationReader
	SummaryReader
	SummaryWriter
	StateRepo
	TurnRepo
	ContextUpdater
	CompressRepo
}

// AgentLookup checks agent existence (decoupled from biz.AgentRepository).
type AgentLookup interface {
	GetAgentByID(ctx context.Context, id string) (struct{}, error)
}

// TeamLookup checks team existence (decoupled from biz.TeamRepository).
type TeamLookup interface {
	GetTeamByID(ctx context.Context, id string) (struct{}, error)
}

// SessionStatusPublisher emits session status change events to realtime observers (WS).
// Implemented in service layer; set via SetStatusPublisher after construction.
type SessionStatusPublisher interface {
	PublishSessionStatusChanged(sessionID string, status string, statusReason string, statusChangedAt string)
}

// MetricsUpdatedPublisher emits metrics_updated events to realtime observers (WS).
// Implemented in service layer; set via SetMetricsUpdatedPublisher after construction.
type MetricsUpdatedPublisher interface {
	PublishMetricsUpdated(sessionID string)
}

// SessionUsecase handles session CRUD + timeline. Chat 写消息经 AppendChat* 等仓储方法，不经 SessionService RPC.
type SessionUsecase struct {
	sessionReader           SessionReader
	sessionTreeReader       SessionTreeReader
	sessionWriter           SessionWriter
	sessionMutator          SessionMutator
	sessionBatchMutator     SessionBatchMutator
	messageReader           MessageReader
	messageSearchReader     MessageSearchReader
	messageWriter           MessageWriter
	messageStatusWriter     MessageStatusWriter
	timelineReader          TimelineReader
	invocationReader        InvocationReader
	summaryReader           SummaryReader
	summaryWriter           SummaryWriter
	stateRepo               StateRepo
	turnRepo                TurnRepo
	contextUpdater          ContextUpdater
	compressRepo            CompressRepo
	runtimeWriter           SessionRuntimeWriter
	agents                  AgentLookup
	teams                   TeamLookup
	titleGenerator          SessionTitleGenerator
	participants            SessionParticipantRepository
	lg                      loggateway.Logger
	statusPublisher         SessionStatusPublisher
	metricsUpdatedPublisher MetricsUpdatedPublisher
	metricsDeltaMu          sync.Mutex
	metricsDeltas           map[string]*SessionMetricsDelta
	flushInterval           time.Duration
}

func NewSessionUsecase(sessions SessionRepo, agents AgentLookup, teams TeamLookup, titleGenerator SessionTitleGenerator, participants SessionParticipantRepository) *SessionUsecase {
	if titleGenerator == nil {
		titleGenerator = NewNoopSessionTitleGenerator()
	}
	return &SessionUsecase{
		sessionReader:       sessions,
		sessionTreeReader:   sessions,
		sessionWriter:       sessions,
		sessionMutator:      sessions,
		sessionBatchMutator: sessions,
		messageReader:       sessions,
		messageSearchReader: sessions,
		messageWriter:       sessions,
		messageStatusWriter: sessions,
		timelineReader:      sessions,
		invocationReader:    sessions,
		summaryReader:       sessions,
		summaryWriter:       sessions,
		stateRepo:           sessions,
		turnRepo:            sessions,
		contextUpdater:      sessions,
		compressRepo:        sessions,
		agents:              agents,
		teams:               teams,
		titleGenerator:      titleGenerator,
		participants:        participants,
		metricsDeltas:       make(map[string]*SessionMetricsDelta),
		flushInterval:       200 * time.Millisecond,
	}
}

func (uc *SessionUsecase) SetStatusPublisher(publisher SessionStatusPublisher) {
	uc.statusPublisher = publisher
}

func (uc *SessionUsecase) SetMetricsUpdatedPublisher(publisher MetricsUpdatedPublisher) {
	uc.metricsUpdatedPublisher = publisher
}

func (uc *SessionUsecase) SetRuntimeWriter(w SessionRuntimeWriter) {
	uc.runtimeWriter = w
}

func (uc *SessionUsecase) TransitionStatus(ctx context.Context, sessionID string, target SessionStatus, reason SessionStatusReason) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return validationErr("session id is required")
	}
	current, err := uc.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	machine := NewSessionStatusMachine(SessionStatus(current.Status), SessionStatusReason(current.StatusReason), current.StatusChangedAt)
	if err := machine.TransitionTo(target, reason); err != nil {
		return err
	}
	newStatus := string(machine.Status())
	newReason := string(machine.StatusReason())
	changedAt := machine.ChangedAt()
	// Route status transitions through SessionRuntimeWriter when available,
	// falling back to direct sessionWriter for backward compatibility.
	if uc.runtimeWriter != nil {
		if err := uc.runtimeWriter.TransitionSessionStatus(ctx, sessionID, current.Status, newStatus, newReason, changedAt); err != nil {
			return err
		}
	} else {
		if _, err := uc.sessionWriter.UpdateSession(ctx, sessionID, SessionUpdateFields{
			Status:          &newStatus,
			StatusReason:    &newReason,
			StatusChangedAt: &changedAt,
		}); err != nil {
			return err
		}
	}
	if uc.statusPublisher != nil {
		uc.statusPublisher.PublishSessionStatusChanged(sessionID, newStatus, newReason, changedAt)
	}
	return nil
}

func (uc *SessionUsecase) Search(ctx context.Context, q SessionSearchQuery) (SessionListResult, error) {
	normalizeSessionSearch(&q)
	return uc.sessionReader.SearchSessions(ctx, q)
}

func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error) {
	return uc.sessionReader.GetSessionByID(ctx, id)
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
	return uc.sessionWriter.CreateSession(ctx, in)
}

func (uc *SessionUsecase) Rename(ctx context.Context, id, title string) (Session, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Session{}, validationErr("title is required")
	}
	return uc.sessionWriter.UpdateSessionTitle(ctx, id, title)
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
	return uc.sessionWriter.UpdateSession(ctx, id, fields)
}

func (uc *SessionUsecase) Restore(ctx context.Context, id string) (Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, validationErr("session id is required")
	}
	return uc.sessionWriter.RestoreSession(ctx, id)
}

func (uc *SessionUsecase) Archive(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationErr("session id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if IsProtectedStatus(SessionStatus(sess.Status)) {
		return kerrors.Conflict("SESSION", fmt.Sprintf("session is %s, cannot archive", sess.Status))
	}
	n, err := uc.sessionMutator.ArchiveSession(ctx, id)
	if n == 0 {
		return kerrors.NotFound("SESSION", id)
	}
	return err
}

func (uc *SessionUsecase) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationErr("session id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if IsProtectedStatus(SessionStatus(sess.Status)) {
		return kerrors.Conflict("SESSION", fmt.Sprintf("session is %s, cannot delete", sess.Status))
	}
	n, err := uc.sessionMutator.DeleteSession(ctx, id)
	if n == 0 {
		return kerrors.NotFound("SESSION", id)
	}
	return err
}

func (uc *SessionUsecase) DeleteByAgent(ctx context.Context, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return validationErr("agent_id is required")
	}
	return uc.sessionMutator.DeleteSessionsByAgentID(ctx, agentID)
}

func (uc *SessionUsecase) ListChildSessions(ctx context.Context, parentSessionID string) ([]Session, error) {
	parentSessionID = strings.TrimSpace(parentSessionID)
	if parentSessionID == "" {
		return nil, validationErr("parent_session_id is required")
	}
	return uc.sessionTreeReader.ListByParentSessionID(ctx, parentSessionID)
}

func (uc *SessionUsecase) GetRootSession(ctx context.Context, sessionID string) (Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return Session{}, validationErr("session_id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	const maxDepth = 10
	for i := 0; i < maxDepth && sess.RootSessionID != ""; i++ {
		root, err := uc.sessionReader.GetSessionByID(ctx, sess.RootSessionID)
		if err != nil {
			return Session{}, err
		}
		if root.RootSessionID == "" || root.RootSessionID == root.ID {
			return root, nil
		}
		sess = root
	}
	return sess, nil
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
