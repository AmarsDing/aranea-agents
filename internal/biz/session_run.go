package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	SessionRunPhaseInteractive = "interactive"
	SessionRunPhaseEscalating  = "escalating"
	SessionRunPhaseDurable     = "durable"
	SessionRunPhaseCompleted   = "completed"
	SessionRunPhaseFailed      = "failed"
)

type SessionRunBudget struct {
	SoftBudgetSec int
	HardBudgetSec int
}

func DefaultSessionRunBudget() SessionRunBudget {
	return SessionRunBudget{SoftBudgetSec: 180, HardBudgetSec: 900}
}

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

const DefaultDurableResumeClaimStaleSec = 300

type SessionRunReader interface {
	Get(ctx context.Context, id string) (SessionRun, error)
	GetActiveForSession(ctx context.Context, sessionID string) (SessionRun, error)
	ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]SessionRun, int, error)
	ListForJobs(ctx context.Context, q SessionRunListQuery) ([]SessionRun, error)
	ListByPhase(ctx context.Context, phase string, limit int) ([]SessionRun, error)
}

type SessionRunWriter interface {
	Create(ctx context.Context, run SessionRun) (string, error)
	UpdatePhase(ctx context.Context, id, phase string) error
	UpdateCheckpointID(ctx context.Context, id, checkpointID string) error
	MarkTerminal(ctx context.Context, id, phase, errMsg string) error
}

type SessionRunDurableRepo interface {
	TryClaimDurableResume(ctx context.Context, id, staleBefore string) (bool, error)
	ClearResumeClaim(ctx context.Context, id string) error
	MarkOrphanedRunsCancelled(ctx context.Context) (int, error)
}

type SessionRunRepo interface {
	SessionRunReader
	SessionRunWriter
	SessionRunDurableRepo
}

type SessionRunListQuery struct {
	SessionID string
	AgentID   string
	Status    string
	Limit     int
}

type SessionRunCheckpoint struct {
	ID           string
	SessionRunID string
	SessionID    string
	TurnID       string
	AgentID      string
	PayloadJSON  string
	CreatedAt    string
}

type SessionRunCheckpointRepo interface {
	Create(ctx context.Context, cp SessionRunCheckpoint) (string, error)
	Get(ctx context.Context, id string) (SessionRunCheckpoint, error)
	GetBySessionRunID(ctx context.Context, sessionRunID string) (SessionRunCheckpoint, error)
}

type SessionRunUsecase struct {
	repo SessionRunRepo
	cps  SessionRunCheckpointRepo
	lg   loggateway.Logger
}

func NewSessionRunUsecase(repo SessionRunRepo, cps SessionRunCheckpointRepo, lg loggateway.Logger) *SessionRunUsecase {
	return &SessionRunUsecase{repo: repo, cps: cps, lg: lg}
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

var errSessionRunNotInit = apierror.Internal("SESSION_RUN", "SessionRunUsecase: not initialized")

func (u *SessionRunUsecase) StartInteractive(ctx context.Context, sessionID, turnID, runtimeRunID, source, agentID string, budget SessionRunBudget) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, errSessionRunNotInit
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return SessionRun{}, apierror.BadRequest("SESSION_RUN", "sessionID and turnID are required")
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
		return errSessionRunNotInit
	}
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("SESSION_RUN", "id is required")
	}
	return u.repo.UpdatePhase(ctx, id, NormalizeSessionRunPhase(phase))
}

func (u *SessionRunUsecase) Complete(ctx context.Context, id string) error {
	if u == nil || u.repo == nil {
		return errSessionRunNotInit
	}
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("SESSION_RUN", "id is required")
	}
	return u.repo.MarkTerminal(ctx, id, SessionRunPhaseCompleted, "")
}

func (u *SessionRunUsecase) Fail(ctx context.Context, id, errMsg string) error {
	if u == nil || u.repo == nil {
		return errSessionRunNotInit
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apierror.BadRequest("SESSION_RUN", "id is required")
	}
	if err := u.repo.MarkTerminal(ctx, id, SessionRunPhaseFailed, strings.TrimSpace(errMsg)); err != nil {
		return err
	}
	if err := u.repo.ClearResumeClaim(ctx, id); err != nil {
		u.lg.Warn("clear resume claim failed", loggateway.StepID("session_run"), loggateway.Str("id", id), loggateway.Err(err))
	}
	return nil
}

func (u *SessionRunUsecase) TryClaimDurableResume(ctx context.Context, id string) (bool, error) {
	if u == nil || u.repo == nil {
		return false, errSessionRunNotInit
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, apierror.BadRequest("SESSION_RUN", "id is required")
	}
	staleBefore := time.Now().UTC().Add(-time.Duration(DefaultDurableResumeClaimStaleSec) * time.Second).Format(time.RFC3339)
	return u.repo.TryClaimDurableResume(ctx, id, staleBefore)
}

func (u *SessionRunUsecase) ClearResumeClaim(ctx context.Context, id string) error {
	if u == nil || u.repo == nil {
		return errSessionRunNotInit
	}
	return u.repo.ClearResumeClaim(ctx, strings.TrimSpace(id))
}

func (u *SessionRunUsecase) Get(ctx context.Context, id string) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, errSessionRunNotInit
	}
	return u.repo.Get(ctx, id)
}

func (u *SessionRunUsecase) GetActiveForSession(ctx context.Context, sessionID string) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, errSessionRunNotInit
	}
	return u.repo.GetActiveForSession(ctx, sessionID)
}

func (u *SessionRunUsecase) CleanupOrphanedRuns(ctx context.Context) (int, error) {
	if u == nil || u.repo == nil {
		return 0, nil
	}
	return u.repo.MarkOrphanedRunsCancelled(ctx)
}

func (u *SessionRunUsecase) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]SessionRun, int, error) {
	if u == nil || u.repo == nil {
		return nil, 0, nil
	}
	return u.repo.ListBySession(ctx, sessionID, limit, offset)
}
