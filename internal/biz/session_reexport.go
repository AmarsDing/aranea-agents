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
	SessionRepo               = session.SessionRepo
	SessionReader           = session.SessionReader
	SessionWriter           = session.SessionWriter
	SessionBatchWriter      = session.SessionBatchWriter
	SessionPinWriter        = session.SessionPinWriter
	SessionRevisionWriter   = session.SessionRevisionWriter
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
	SessionParticipant      = session.SessionParticipant
)

// Session interfaces for dependency injection.
type (
	SessionAgentLookup = session.AgentLookup
	SessionTeamLookup  = session.TeamLookup
)

const (
	MessageListDefaultLimit = session.MessageListDefaultLimit
	MessageListMaxLimit     = session.MessageListMaxLimit
	TimelineMessageMaxFetch = session.TimelineMessageMaxFetch
	CompressMessageMaxRows  = session.CompressMessageMaxRows
	ActivityCancelScanLimit = session.ActivityCancelScanLimit
	SessionBatchPageSize    = session.SessionBatchPageSize
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

// teamLookupAdapter adapts biz.TeamRepository to session.TeamLookup.
type teamLookupAdapter struct {
	teams TeamRepository
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

// NewSessionTeamLookup creates a session.TeamLookup from biz.TeamRepository.
func NewSessionTeamLookup(teams TeamRepository) session.TeamLookup {
	return &teamLookupAdapter{teams: teams}
}
