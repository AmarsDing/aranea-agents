package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Session run lifecycle phases (M55 §2.6 · CC-R-01).
const (
	SessionRunPhaseInteractive = "interactive"
	SessionRunPhaseEscalating  = "escalating"
	SessionRunPhaseDurable     = "durable"
	SessionRunPhaseCompleted   = "completed"
	SessionRunPhaseFailed      = "failed"
)

// DefaultSessionRunBudget matches blueprint run_policy defaults (§2.6.5).
type SessionRunBudget struct {
	SoftBudgetSec int
	HardBudgetSec int
}

func DefaultSessionRunBudget() SessionRunBudget {
	return SessionRunBudget{SoftBudgetSec: 180, HardBudgetSec: 900}
}

// SessionRun tracks one user-message execution lifecycle across Interactive/Durable phases.
type SessionRun struct {
	ID              string
	SessionID       string
	TurnID          string
	RuntimeRunID    string
	AgentID         string
	Source          string
	Phase           string
	SoftBudgetSec   int
	HardBudgetSec   int
	CheckpointID    string
	WorkflowJobID   string
	ErrorMessage    string
	StartedAt       string
	PhaseChangedAt  string
	FinishedAt      string
	ResumeStartedAt string
	CreatedAt       string
	UpdatedAt       string
}

// DefaultDurableResumeClaimStaleSec is how long before a stale resume claim may be retried (CC-R-OPT-01).
const DefaultDurableResumeClaimStaleSec = 300

// SessionRunRepo persists session run rows (CC-R-01).
type SessionRunRepo interface {
	Create(ctx context.Context, run SessionRun) (string, error)
	UpdatePhase(ctx context.Context, id, phase string) error
	UpdateCheckpointID(ctx context.Context, id, checkpointID string) error
	MarkTerminal(ctx context.Context, id, phase, errMsg string) error
	Get(ctx context.Context, id string) (SessionRun, error)
	GetActiveForSession(ctx context.Context, sessionID string) (SessionRun, error)
	ListByPhase(ctx context.Context, phase string, limit int) ([]SessionRun, error)
	ListForJobs(ctx context.Context, q SessionRunListQuery) ([]SessionRun, error)
	ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]SessionRun, int, error)
	TryClaimDurableResume(ctx context.Context, id, staleBefore string) (bool, error)
	ClearResumeClaim(ctx context.Context, id string) error
}

// SessionRunListQuery filters session runs for chat jobs panel (CC-R-04).
type SessionRunListQuery struct {
	SessionID string
	AgentID   string
	Status    string
	Limit     int
}

// SessionRunCheckpoint stores durable resume payload (CC-R-03).
type SessionRunCheckpoint struct {
	ID           string
	SessionRunID string
	SessionID    string
	TurnID       string
	AgentID      string
	PayloadJSON  string
	CreatedAt    string
}

// SessionRunCheckpointRepo persists checkpoint rows.
type SessionRunCheckpointRepo interface {
	Create(ctx context.Context, cp SessionRunCheckpoint) (string, error)
	Get(ctx context.Context, id string) (SessionRunCheckpoint, error)
	GetBySessionRunID(ctx context.Context, sessionRunID string) (SessionRunCheckpoint, error)
}

// SessionRunUsecase owns Interactive→Durable lifecycle bookkeeping.
type SessionRunUsecase struct {
	repo SessionRunRepo
	cps  SessionRunCheckpointRepo
}

func NewSessionRunUsecase(repo SessionRunRepo, cps SessionRunCheckpointRepo) *SessionRunUsecase {
	return &SessionRunUsecase{repo: repo, cps: cps}
}

func NormalizeSessionRunPhase(phase string) string {
	switch strings.TrimSpace(strings.ToLower(phase)) {
	case SessionRunPhaseInteractive, SessionRunPhaseEscalating, SessionRunPhaseDurable,
		SessionRunPhaseCompleted, SessionRunPhaseFailed:
		return strings.TrimSpace(strings.ToLower(phase))
	default:
		return SessionRunPhaseInteractive
	}
}

func sessionRunNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// StartInteractive creates a run row in interactive phase before agent execution begins.
func (u *SessionRunUsecase) StartInteractive(ctx context.Context, sessionID, turnID, runtimeRunID, source, agentID string, budget SessionRunBudget) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, fmt.Errorf("SessionRunUsecase: not initialized")
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return SessionRun{}, fmt.Errorf("SessionRunUsecase.StartInteractive: sessionID and turnID are required")
	}
	if budget.SoftBudgetSec <= 0 {
		budget = DefaultSessionRunBudget()
	}
	if budget.HardBudgetSec <= 0 {
		budget.HardBudgetSec = DefaultSessionRunBudget().HardBudgetSec
	}
	now := sessionRunNow()
	run := SessionRun{
		ID:             uuid.NewString(),
		SessionID:      sessionID,
		TurnID:         turnID,
		RuntimeRunID:   strings.TrimSpace(runtimeRunID),
		AgentID:        strings.TrimSpace(agentID),
		Source:         strings.TrimSpace(source),
		Phase:          SessionRunPhaseInteractive,
		SoftBudgetSec:  budget.SoftBudgetSec,
		HardBudgetSec:  budget.HardBudgetSec,
		StartedAt:      now,
		PhaseChangedAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	id, err := u.repo.Create(ctx, run)
	if err != nil {
		return SessionRun{}, err
	}
	run.ID = id
	return run, nil
}

func (u *SessionRunUsecase) MarkPhase(ctx context.Context, id, phase string) error {
	if u == nil || u.repo == nil {
		return fmt.Errorf("SessionRunUsecase: not initialized")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("SessionRunUsecase.MarkPhase: id is required")
	}
	return u.repo.UpdatePhase(ctx, id, NormalizeSessionRunPhase(phase))
}

func (u *SessionRunUsecase) Complete(ctx context.Context, id string) error {
	if u == nil || u.repo == nil {
		return fmt.Errorf("SessionRunUsecase: not initialized")
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("SessionRunUsecase.Complete: id is required")
	}
	return u.repo.MarkTerminal(ctx, id, SessionRunPhaseCompleted, "")
}

func (u *SessionRunUsecase) Fail(ctx context.Context, id, errMsg string) error {
	if u == nil || u.repo == nil {
		return fmt.Errorf("SessionRunUsecase: not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("SessionRunUsecase.Fail: id is required")
	}
	if err := u.repo.MarkTerminal(ctx, id, SessionRunPhaseFailed, strings.TrimSpace(errMsg)); err != nil {
		return err
	}
	_ = u.repo.ClearResumeClaim(ctx, id)
	return nil
}

// TryClaimDurableResume atomically marks a durable run as resume-in-flight (CC-R-OPT-01).
func (u *SessionRunUsecase) TryClaimDurableResume(ctx context.Context, id string) (bool, error) {
	if u == nil || u.repo == nil {
		return false, fmt.Errorf("SessionRunUsecase: not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("SessionRunUsecase.TryClaimDurableResume: id is required")
	}
	staleBefore := time.Now().UTC().Add(-time.Duration(DefaultDurableResumeClaimStaleSec) * time.Second).Format(time.RFC3339)
	return u.repo.TryClaimDurableResume(ctx, id, staleBefore)
}

// ClearResumeClaim releases a durable resume claim so the worker may retry (CC-R-OPT-01).
func (u *SessionRunUsecase) ClearResumeClaim(ctx context.Context, id string) error {
	if u == nil || u.repo == nil {
		return fmt.Errorf("SessionRunUsecase: not initialized")
	}
	return u.repo.ClearResumeClaim(ctx, strings.TrimSpace(id))
}

func (u *SessionRunUsecase) Get(ctx context.Context, id string) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, fmt.Errorf("SessionRunUsecase: not initialized")
	}
	return u.repo.Get(ctx, id)
}

func (u *SessionRunUsecase) GetActiveForSession(ctx context.Context, sessionID string) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, fmt.Errorf("SessionRunUsecase: not initialized")
	}
	return u.repo.GetActiveForSession(ctx, sessionID)
}
