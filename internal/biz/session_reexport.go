package biz

import (
	"context"

	"aranea-agents/internal/biz/session"
)

type (
	Session                 = session.Session
	SessionSearchQuery      = session.SessionSearchQuery
	SessionListResult       = session.SessionListResult
	MessageSearchQuery      = session.MessageSearchQuery
	MessageSearchHit        = session.MessageSearchHit
	MessageSearchResult     = session.MessageSearchResult
	MessageListResult       = session.MessageListResult
	ChatMessage             = session.ChatMessage
	ToolInvocationView      = session.ToolInvocationView
	SkillInvocationView     = session.SkillInvocationView
	SessionTimelineItem     = session.SessionTimelineItem
	SessionTimelineSummary  = session.SessionTimelineSummary
	SessionTimeline         = session.SessionTimeline
	SessionUpdateFields     = session.SessionUpdateFields
	TimelineQuery           = session.TimelineQuery
	TimelineEventRef        = session.TimelineEventRef
	SessionTurn             = session.SessionTurn
	SessionTurnListResult   = session.SessionTurnListResult
	SessionTurnUpdateFields = session.SessionTurnUpdateFields
	SessionSummary          = session.SessionSummary
	StateDelta              = session.StateDelta
	SessionRepo             = session.SessionRepo
	SessionReader           = session.SessionReader
	SessionTreeReader       = session.SessionTreeReader
	SessionWriter           = session.SessionWriter
	SessionMutator          = session.SessionMutator
	SessionBatchMutator     = session.SessionBatchMutator
	MessageReader           = session.MessageReader
	MessageSearchReader     = session.MessageSearchReader
	MessageWriter           = session.MessageWriter
	MessageStatusWriter     = session.MessageStatusWriter
	TimelineReader          = session.TimelineReader
	InvocationReader        = session.InvocationReader
	SummaryReader           = session.SummaryReader
	SummaryWriter           = session.SummaryWriter
	StateRepo               = session.StateRepo
	TurnRepo                = session.TurnRepo
	ContextUpdater          = session.ContextUpdater
	CompressRepo            = session.CompressRepo
	SessionTitleGenerator   = session.SessionTitleGenerator
	SessionUsecase          = session.SessionUsecase
	SessionBatchScope       = session.SessionBatchScope
	SessionBatchPreview     = session.SessionBatchPreview
	SessionBatchResult      = session.SessionBatchResult
	BatchOperationParams    = session.BatchOperationParams
	SessionParticipant      = session.SessionParticipant
	SessionMetricsReader    = session.SessionMetricsReader
	SessionMetricsWriter    = session.SessionMetricsWriter
	SessionRuntimeReader    = session.SessionRuntimeReader
	SessionRuntimeWriter    = session.SessionRuntimeWriter
	SessionMetrics          = session.SessionMetrics
	SessionRuntime          = session.SessionRuntime

	// Phase 2: Session tree hierarchy types
	SessionType           = session.SessionType
	SessionTree           = session.SessionTree
	SessionTreeNode       = session.SessionTreeNode
	DepthValidationConfig = session.DepthValidationConfig
)

// Session tree type constants (Phase 2).
const (
	SessionTypeSpirit     = session.SessionTypeSpirit
	SessionTypeTeam       = session.SessionTypeTeam
	SessionTypeAgent      = session.SessionTypeAgent
	SessionTypeStandalone = session.SessionTypeStandalone
)

// ValidateDepth is re-exported from session package for use by biz callers
// (SpiritTeamUsecase, etc.) without importing the session subpackage directly.
var ValidateDepth = session.ValidateDepth

// Session interfaces for dependency injection.
type (
	SessionAgentLookup      = session.AgentLookup
	SessionTeamLookup       = session.TeamLookup
	SessionStatusPublisher  = session.SessionStatusPublisher
	MetricsUpdatedPublisher = session.MetricsUpdatedPublisher
)

const (
	SessionBatchPageSize = session.SessionBatchPageSize
)

var (
	NewSessionUsecase            = session.NewSessionUsecase
	NewNoopSessionTitleGenerator = session.NewNoopSessionTitleGenerator
)

// agentLookupAdapter adapts biz.AgentRepository to session.AgentLookup.
type agentLookupAdapter struct {
	agents AgentRepository
}

func (a *agentLookupAdapter) GetAgentByID(ctx context.Context, id string) (struct{}, error) {
	_, err := a.agents.GetAgentByID(ctx, id)
	if err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}

// teamLookupAdapter adapts biz.TeamReader to session.TeamLookup.
type teamLookupAdapter struct {
	teams TeamReader
}

func (a *teamLookupAdapter) GetTeamByID(ctx context.Context, id string) (struct{}, error) {
	_, err := a.teams.GetTeamByID(ctx, id)
	if err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}

// NewSessionAgentLookup creates a session.AgentLookup from biz.AgentRepository.
func NewSessionAgentLookup(agents AgentRepository) session.AgentLookup {
	return &agentLookupAdapter{agents: agents}
}

// NewSessionTeamLookup creates a session.TeamLookup from biz.TeamReader.
func NewSessionTeamLookup(teams TeamReader) session.TeamLookup {
	return &teamLookupAdapter{teams: teams}
}
