package biz

import (
	"aranea-agents/internal/event"
)

type EnvelopeType = event.EnvelopeType

const (
	EnvelopeTypeTextDelta          = event.EnvelopeTypeTextDelta
	EnvelopeTypeTextDone           = event.EnvelopeTypeTextDone
	EnvelopeTypeToolCall           = event.EnvelopeTypeToolCall
	EnvelopeTypeToolResult         = event.EnvelopeTypeToolResult
	EnvelopeTypeStateDelta         = event.EnvelopeTypeStateDelta
	EnvelopeTypeTransfer           = event.EnvelopeTypeTransfer
	EnvelopeTypeRunnerCompletion   = event.EnvelopeTypeRunnerCompletion
	EnvelopeTypeError              = event.EnvelopeTypeError
	EnvelopeTypeLog                = event.EnvelopeTypeLog
	EnvelopeTypeGraphNodeStart     = event.EnvelopeTypeGraphNodeStart
	EnvelopeTypeGraphNodeEnd       = event.EnvelopeTypeGraphNodeEnd
	EnvelopeTypeCheckpoint         = event.EnvelopeTypeCheckpoint
	EnvelopeTypeIntentPass         = event.EnvelopeTypeIntentPass
	EnvelopeTypeMemberMessageStart = event.EnvelopeTypeMemberMessageStart
	EnvelopeTypeMemberDelta        = event.EnvelopeTypeMemberDelta
	EnvelopeTypeMemberMessageDone  = event.EnvelopeTypeMemberMessageDone
	EnvelopeTypeTeamRunStarted     = event.EnvelopeTypeTeamRunStarted
	EnvelopeTypeTeamRunFinished    = event.EnvelopeTypeTeamRunFinished
	EnvelopeTypeTeamStepStarted    = event.EnvelopeTypeTeamStepStarted
	EnvelopeTypeTeamStepFinished   = event.EnvelopeTypeTeamStepFinished
	EnvelopeTypeTeamRunFailed      = event.EnvelopeTypeTeamRunFailed
)

type Envelope = event.Envelope
type EnvelopeContent = event.EnvelopeContent
type EnvelopeToolCall = event.EnvelopeToolCall
type EnvelopeStateDelta = event.EnvelopeStateDelta
type EnvelopeTransfer = event.EnvelopeTransfer
type EnvelopeError = event.EnvelopeError
type EnvelopeUsage = event.EnvelopeUsage
type EnvelopeActions = event.EnvelopeActions
type EnvelopeTrace = event.EnvelopeTrace

func NewEnvelope(typ EnvelopeType, author, sessionID string) Envelope {
	return event.NewEnvelope(typ, author, sessionID)
}

func RouteChannel(env Envelope) string {
	return event.RouteChannel(env)
}

func MatchFilterKey(subscriberKey, eventKey string) bool {
	return event.MatchFilterKey(subscriberKey, eventKey)
}
