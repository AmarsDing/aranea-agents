// Package event re-exports types from contract and adds implementation-specific helpers.
// New code in biz should import event/contract directly.
package event

import (
	"aranea-agents/internal/event/contract"
)

// Re-export all contract types for backward compatibility.
type (
	EnvelopeType       = contract.EnvelopeType
	Envelope           = contract.Envelope
	EnvelopeContent    = contract.EnvelopeContent
	EnvelopeToolCall   = contract.EnvelopeToolCall
	EnvelopeStateDelta = contract.EnvelopeStateDelta
	EnvelopeTransfer   = contract.EnvelopeTransfer
	EnvelopeError      = contract.EnvelopeError
	EnvelopeUsage      = contract.EnvelopeUsage
	EnvelopeActions    = contract.EnvelopeActions
	EnvelopeTrace      = contract.EnvelopeTrace
)

// Re-export constants.
const (
	EnvelopeTypeStateDelta                      = contract.EnvelopeTypeStateDelta
	EnvelopeTypeContextUsage                    = contract.EnvelopeTypeContextUsage
	EnvelopeTypeRunStatus                       = contract.EnvelopeTypeRunStatus
	EnvelopeTypeError                           = contract.EnvelopeTypeError
	EnvelopeTypeLog                             = contract.EnvelopeTypeLog
	EnvelopeTypeFlowLog                         = contract.EnvelopeTypeFlowLog
	EnvelopeTypeGraphNodeStart                  = contract.EnvelopeTypeGraphNodeStart
	EnvelopeTypeGraphNodeEnd                    = contract.EnvelopeTypeGraphNodeEnd
	EnvelopeTypeCheckpoint                      = contract.EnvelopeTypeCheckpoint
	EnvelopeTypeIntentPass                      = contract.EnvelopeTypeIntentPass
	EnvelopeTypeTeamRunStarted                  = contract.EnvelopeTypeTeamRunStarted
	EnvelopeTypeTeamRunFinished                 = contract.EnvelopeTypeTeamRunFinished
	EnvelopeTypeTeamStepStarted                 = contract.EnvelopeTypeTeamStepStarted
	EnvelopeTypeTeamStepFinished                = contract.EnvelopeTypeTeamStepFinished
	EnvelopeTypeTeamRunFailed                   = contract.EnvelopeTypeTeamRunFailed
	EnvelopeTypeTeamSummary                     = contract.EnvelopeTypeTeamSummary
	EnvelopeTypeGraphStep                       = contract.EnvelopeTypeGraphStep
	EnvelopeTypeGraphExecutionDone              = contract.EnvelopeTypeGraphExecutionDone
	EnvelopeTypeGraphNodeError                  = contract.EnvelopeTypeGraphNodeError
	EnvelopeTypeGraphNodeCustom                 = contract.EnvelopeTypeGraphNodeCustom
	EnvelopeTypeGraphTaskStatus                 = contract.EnvelopeTypeGraphTaskStatus
	EnvelopeTypeKnowledgeIngest                 = contract.EnvelopeTypeKnowledgeIngest
	EnvelopeTypeMCPSessionReconnect             = contract.EnvelopeTypeMCPSessionReconnect
	EnvelopeTypeMCPHealthAlert                  = contract.EnvelopeTypeMCPHealthAlert
	EnvelopeTypeAlertNotify                     = contract.EnvelopeTypeAlertNotify
	EnvelopeTypeOrchestrationAgentStatus        = contract.EnvelopeTypeOrchestrationAgentStatus
	EnvelopeTypeUserFeedback                    = contract.EnvelopeTypeUserFeedback
	EnvelopeTypeSessionStatusChanged            = contract.EnvelopeTypeSessionStatusChanged
	EnvelopeTypeSpiritTeamAssembled             = contract.EnvelopeTypeSpiritTeamAssembled
	EnvelopeTypeSpiritTeamCompleted             = contract.EnvelopeTypeSpiritTeamCompleted
	EnvelopeTypeSpiritTeamFailed                = contract.EnvelopeTypeSpiritTeamFailed
	EnvelopeTypeSpiritTeamInterrupted           = contract.EnvelopeTypeSpiritTeamInterrupted
	EnvelopeTypeSpiritTeamProgress              = contract.EnvelopeTypeSpiritTeamProgress
	EnvelopeTypeSpiritTeamsAllCompleted         = contract.EnvelopeTypeSpiritTeamsAllCompleted
	EnvelopeTypeSpiritSynthesisCompleted        = contract.EnvelopeTypeSpiritSynthesisCompleted
	EnvelopeTypeSpiritPlanCreated               = contract.EnvelopeTypeSpiritPlanCreated
	EnvelopeTypeSpiritAllocationCreated         = contract.EnvelopeTypeSpiritAllocationCreated
	EnvelopeTypeSpiritOrchestrationStarted      = contract.EnvelopeTypeSpiritOrchestrationStarted
	EnvelopeTypeSpiritOrchestrationCheckpoint   = contract.EnvelopeTypeSpiritOrchestrationCheckpoint
	EnvelopeTypeSpiritOrchestrationInterrupted  = contract.EnvelopeTypeSpiritOrchestrationInterrupted
	EnvelopeTypeMetricsUpdated                  = contract.EnvelopeTypeMetricsUpdated
	EnvelopeTypeExecutionProgress               = contract.EnvelopeTypeExecutionProgress
	EnvelopeTypeButlerOrchestrationStarted      = contract.EnvelopeTypeButlerOrchestrationStarted
	EnvelopeTypeButlerOrchestrationCompleted    = contract.EnvelopeTypeButlerOrchestrationCompleted
	EnvelopeTypeButlerOrchestrationFailed       = contract.EnvelopeTypeButlerOrchestrationFailed
	EnvelopeTypeSkillHealthChanged              = contract.EnvelopeTypeSkillHealthChanged
	EnvelopeTypeSkillEvolutionProposed          = contract.EnvelopeTypeSkillEvolutionProposed
	EnvelopeTypeOrchestrationEvolutionSuggested = contract.EnvelopeTypeOrchestrationEvolutionSuggested
	EnvelopeTypeOrchestrationCacheHit           = contract.EnvelopeTypeOrchestrationCacheHit
	EnvelopeTypeMonitorAutoHealed               = contract.EnvelopeTypeMonitorAutoHealed
	EnvelopeTypeMonitorSelfCheckCompleted       = contract.EnvelopeTypeMonitorSelfCheckCompleted
	EnvelopeTypeTokenUsage                      = contract.EnvelopeTypeTokenUsage

	// EnvelopeToolCall error_code constants
	ErrorCodeToolTimeout          = contract.ErrorCodeToolTimeout
	ErrorCodeToolError            = contract.ErrorCodeToolError
	ErrorCodeConfirmationRequired = contract.ErrorCodeConfirmationRequired
	ErrorCodeConfirmationDenied   = contract.ErrorCodeConfirmationDenied
	ErrorCodeConfirmationTimeout  = contract.ErrorCodeConfirmationTimeout
)

// Re-export functions.
var (
	NewEnvelope     = contract.NewEnvelope
	RouteChannel    = contract.RouteChannel
	MatchFilterKey  = contract.MatchFilterKey
	ValidErrorCodes = contract.ValidErrorCodes
)
