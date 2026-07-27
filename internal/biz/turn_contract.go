package biz

import (
	"strings"
	"time"
)

// TurnSource is the user-visible origin of a turn.
type TurnSource string

const (
	TurnSourceWeb     TurnSource = "web"
	TurnSourceWS      TurnSource = "ws"
	TurnSourceChannel TurnSource = "channel"
	TurnSourceCron    TurnSource = "cron"
	TurnSourceA2A     TurnSource = "a2a"
	TurnSourceDurable TurnSource = "durable"
)

// ConversationTargetType is the business object that owns a conversation.
type ConversationTargetType string

const (
	ConversationTargetAgent ConversationTargetType = "agent"
	ConversationTargetTeam  ConversationTargetType = "team"
)

// CanonicalTurnStatus is the canonical lifecycle visible to Chat, Channel, Session, Run, and Job projections.
type CanonicalTurnStatus string

const (
	CanonicalTurnStatusQueued       CanonicalTurnStatus = "queued"
	CanonicalTurnStatusRunning      CanonicalTurnStatus = "running"
	CanonicalTurnStatusAwaitingUser CanonicalTurnStatus = "awaiting_user"
	CanonicalTurnStatusBackground   CanonicalTurnStatus = "background"
	CanonicalTurnStatusCompleted    CanonicalTurnStatus = "completed"
	CanonicalTurnStatusFailed       CanonicalTurnStatus = "failed"
	CanonicalTurnStatusRejected     CanonicalTurnStatus = "rejected"
	CanonicalTurnStatusCancelled    CanonicalTurnStatus = "cancelled"
)

// DeliveryStatus tracks whether an assistant answer has been delivered to a target surface.
type DeliveryStatus string

const (
	DeliveryStatusNotRequired DeliveryStatus = "not_required"
	DeliveryStatusPending     DeliveryStatus = "pending"
	DeliveryStatusSending     DeliveryStatus = "sending"
	DeliveryStatusDelivered   DeliveryStatus = "delivered"
	DeliveryStatusFailed      DeliveryStatus = "failed"
	DeliveryStatusSkipped     DeliveryStatus = "skipped"
)

// DeliveryTarget describes one output surface for a turn, such as the web chat or an IM peer.
type DeliveryTarget struct {
	Kind        string
	ChannelID   string
	Platform    string
	RecipientID string
	Status      DeliveryStatus
	Error       string
	UpdatedAt   time.Time
}

// TurnIntent is the transport-neutral input before admission creates a canonical Turn.
type TurnIntent struct {
	Source          TurnSource
	SessionID       string
	TargetType      ConversationTargetType
	AgentID         string
	AgentKey        string
	TeamID          string
	Content         string
	AttachmentIDs   []string
	IdempotencyKey  string
	DeliveryTargets []DeliveryTarget
	Options         TurnOptions
	Timeouts        TurnTimeouts
	EntryConfig     TurnEntryPointConfig
	ParentTaskID    string
	Synthesis       bool // 精灵总结 turn 标记（见 TurnInput.Synthesis）
}

// Canonicalize fills source/target defaults from legacy TurnInput-compatible fields.
func (i TurnIntent) Canonicalize() TurnIntent {
	i.Source = canonicalTurnSource(i.Source, i.EntryConfig.EntryPoint)
	if i.TargetType == "" {
		if strings.TrimSpace(i.TeamID) != "" {
			i.TargetType = ConversationTargetTeam
		} else {
			i.TargetType = ConversationTargetAgent
		}
	}
	i.SessionID = strings.TrimSpace(i.SessionID)
	i.AgentID = strings.TrimSpace(i.AgentID)
	i.AgentKey = strings.TrimSpace(i.AgentKey)
	i.TeamID = strings.TrimSpace(i.TeamID)
	i.Content = strings.TrimSpace(i.Content)
	i.IdempotencyKey = strings.TrimSpace(i.IdempotencyKey)
	return i
}

// TurnInput converts a canonical intent back to the existing executor input.
func (i TurnIntent) TurnInput() TurnInput {
	i = i.Canonicalize()
	return TurnInput{
		SessionID: i.SessionID,
		Content:   i.Content,
		AgentKey:  i.AgentKey,
		TeamID:    i.TeamID,
		Options:   i.Options,
		Timeouts:  i.Timeouts,
		EntryConfig: TurnEntryPointConfig{
			EntryPoint:  entryPointFromTurnSource(i.Source),
			AllowQueue:  i.EntryConfig.AllowQueue,
			AllowStream: i.EntryConfig.AllowStream,
			Platform:    i.EntryConfig.Platform,
		},
		ParentTaskID: i.ParentTaskID,
		Synthesis:    i.Synthesis,
	}
}

// CanonicalTurn is the aggregate root tying messages, runtime run state, jobs, and delivery together.
type CanonicalTurn struct {
	ID              string
	SessionID       string
	RunID           string
	SessionRunID    string
	Source          TurnSource
	TargetType      ConversationTargetType
	AgentID         string
	TeamID          string
	Status          CanonicalTurnStatus
	SessionRevision int64
	DeliveryTargets []DeliveryTarget
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TurnEventType names the domain event projected to chat, channel, monitor, and session views.
type TurnEventType string

const (
	TurnEventUserMessagePersisted TurnEventType = "user_message_persisted"
	TurnEventAssistantDelta       TurnEventType = "assistant_delta"
	TurnEventToolCall             TurnEventType = "tool_call"
	TurnEventMemberDelta          TurnEventType = "member_delta"
	TurnEventAwaitUser            TurnEventType = "await_user"
	TurnEventQueued               TurnEventType = "queued"
	TurnEventCompleted            TurnEventType = "completed"
	TurnEventFailed               TurnEventType = "failed"
	TurnEventDelivered            TurnEventType = "delivered"
)

// TurnEvent is the canonical event shape consumed by projectors.
type TurnEvent struct {
	ID              string
	TurnID          string
	SessionID       string
	Type            TurnEventType
	Source          TurnSource
	Status          CanonicalTurnStatus
	SessionRevision int64
	Delivery        *DeliveryTarget
	OccurredAt      time.Time
	Metadata        map[string]string
}

// CanonicalTurnStatusFromOutcome maps the unified turn outcome into the canonical lifecycle.
func CanonicalTurnStatusFromOutcome(outcome TurnOutcome) CanonicalTurnStatus {
	switch outcome {
	case TurnOutcomeCompleted:
		return CanonicalTurnStatusCompleted
	case TurnOutcomeQueued:
		return CanonicalTurnStatusQueued
	case TurnOutcomeRejected:
		return CanonicalTurnStatusRejected
	case TurnOutcomeFailed:
		return CanonicalTurnStatusFailed
	default:
		return ""
	}
}

// DeliveryStatusFromChannelRecord maps existing Channel delivery status strings into canonical delivery state.
func DeliveryStatusFromChannelRecord(status string) DeliveryStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending":
		return DeliveryStatusPending
	case "sending", "streaming", "streamed":
		return DeliveryStatusSending
	case "sent", "delivered", "ok", "success":
		return DeliveryStatusDelivered
	case "failed", "error", "timeout":
		return DeliveryStatusFailed
	case "skipped", "skipped_duplicate", "skipped_access", "skipped_empty":
		return DeliveryStatusSkipped
	default:
		return ""
	}
}

func canonicalTurnSource(source TurnSource, entry TurnEntryPoint) TurnSource {
	if source != "" {
		return source
	}
	switch entry {
	case EntryPointWS:
		return TurnSourceWS
	case EntryPointChannel:
		return TurnSourceChannel
	case EntryPointCron:
		return TurnSourceCron
	case EntryPointA2A:
		return TurnSourceA2A
	case EntryPointDurable:
		return TurnSourceDurable
	default:
		return TurnSourceWeb
	}
}

func entryPointFromTurnSource(source TurnSource) TurnEntryPoint {
	switch source {
	case TurnSourceWS:
		return EntryPointWS
	case TurnSourceChannel:
		return EntryPointChannel
	case TurnSourceCron:
		return EntryPointCron
	case TurnSourceA2A:
		return EntryPointA2A
	case TurnSourceDurable:
		return EntryPointDurable
	default:
		return EntryPointWeb
	}
}
