package biz

import (
	"context"
	"time"
)

type ObservationKind string

const (
	ObservationKindToolCall   ObservationKind = "tool_call"
	ObservationKindFeedback   ObservationKind = "feedback"
	ObservationKindMemoryHit  ObservationKind = "memory_hit"
	ObservationKindMemoryMiss ObservationKind = "memory_miss"
)

type Observation struct {
	ID         string
	AgentID    string
	SessionID  string
	Kind       ObservationKind
	Content    string
	Metadata   string
	ObservedAt time.Time
}

type PatternStatus string

const (
	PatternStatusDetected  PatternStatus = "detected"
	PatternStatusConfirmed PatternStatus = "confirmed"
	PatternStatusDismissed PatternStatus = "dismissed"
)

type Pattern struct {
	ID          string
	AgentID     string
	Kind        string
	Description string
	Frequency   int
	Confidence  float64
	Evidence    string
	Status      PatternStatus
	DetectedAt  time.Time
}

type ProposalStatus string

const (
	ProposalStatusDraft     ProposalStatus = "draft"
	ProposalStatusValidated ProposalStatus = "validated"
	ProposalStatusApproved  ProposalStatus = "approved"
	ProposalStatusRejected  ProposalStatus = "rejected"
	ProposalStatusApplied   ProposalStatus = "applied"
	ProposalStatusConflict  ProposalStatus = "conflict"
	ProposalStatusExpired   ProposalStatus = "expired"
)

type KnowledgeProposal struct {
	ID          string
	AgentID     string
	PatternID   string
	Title       string
	Content     string
	Kind        string
	Status      ProposalStatus
	ValidatedAt *time.Time
	ApprovedBy  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ObservationRecorder is the narrow write port for recording learning
// observations from runtime paths (tool invocation recorder, etc.).
// *LearningLoopUsecase implements it.
// Stability:evolving
type ObservationRecorder interface {
	RecordObservation(ctx context.Context, obs Observation) (Observation, error)
}

type ObservationReader interface {
	ListByAgent(ctx context.Context, agentID string, since time.Time) ([]Observation, error)
	CountByAgent(ctx context.Context, agentID string, since time.Time) (int64, error)
}

type ObservationWriter interface {
	Create(ctx context.Context, obs Observation) (Observation, error)
	BatchCreate(ctx context.Context, obs []Observation) error
}

type PatternReader interface {
	ListByAgent(ctx context.Context, agentID string, status string) ([]Pattern, error)
	GetByID(ctx context.Context, id string) (Pattern, error)
}

type PatternWriter interface {
	Create(ctx context.Context, p Pattern) (Pattern, error)
	UpdateStatus(ctx context.Context, id string, status PatternStatus) (Pattern, error)
}

type ProposalReader interface {
	ListByAgent(ctx context.Context, agentID string, status string) ([]KnowledgeProposal, error)
	GetByID(ctx context.Context, id string) (KnowledgeProposal, error)
}

type ProposalWriter interface {
	Create(ctx context.Context, p KnowledgeProposal) (KnowledgeProposal, error)
	UpdateStatus(ctx context.Context, id string, status ProposalStatus, approvedBy string) (KnowledgeProposal, error)
	// UpdateStatusCAS atomically transitions the proposal from one of
	// expectedStatuses to newStatus. Returns (updated, true, nil) on success,
	// (zero, false, nil) when the current status is not in expectedStatuses
	// (concurrent modification), or (zero, false, err) on DB error.
	UpdateStatusCAS(ctx context.Context, id string, expectedStatuses []ProposalStatus, newStatus ProposalStatus, approvedBy string) (KnowledgeProposal, bool, error)
}
