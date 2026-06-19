package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const (
	// Legacy string constants — prefer the typed SessionRunPhase constants in
	// session_run_phase_machine.go (PhaseInteractive, PhaseDurable, etc.).
	// These are kept only for DB string comparisons where a typed constant
	// would require an explicit string() cast. New code must use PhaseXxx.
	//
	// TODO(debt): phase params in repo interfaces should use SessionRunPhase
	// instead of string, after which these legacy constants can be removed.
	SessionRunPhaseInteractive = string(PhaseInteractive)
	SessionRunPhaseDurable     = string(PhaseDurable)
	SessionRunPhaseCompleted   = string(PhaseCompleted)
	SessionRunPhaseFailed      = string(PhaseFailed)
	SessionRunPhaseCancelled   = string(PhaseCancelled)
)

// sessionRunPhaseMachine is the shared, stateless phase machine for all SessionRun transitions.
var sessionRunPhaseMachine = NewSessionRunPhaseMachine()

type SessionRun struct {
	ID              string
	SessionID       string
	TurnID          string
	RuntimeRunID    string
	AgentID         string
	Source          string
	Phase           string
	SoftBudgetSec   int // Deprecated: budget mechanism removed, kept for DB compatibility
	HardBudgetSec   int // Deprecated: budget mechanism removed, kept for DB compatibility
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

// Stability:stable
type SessionRunReader interface {
	Get(ctx context.Context, id string) (SessionRun, error)
	GetActiveForSession(ctx context.Context, sessionID string) (SessionRun, error)
	ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]SessionRun, int, error)
	ListForJobs(ctx context.Context, q SessionRunListQuery) ([]SessionRun, error)
	ListByPhase(ctx context.Context, phase string, limit int) ([]SessionRun, error)
}

// Stability:stable
type SessionRunWriter interface {
	Create(ctx context.Context, run SessionRun) (string, error)
	UpdatePhase(ctx context.Context, id, phase string) error
	UpdateCheckpointID(ctx context.Context, id, checkpointID string) error
	MarkTerminal(ctx context.Context, id, phase, errMsg string) error
}

// Stability:evolving
type SessionRunDurableRepo interface {
	TryClaimDurableResume(ctx context.Context, id, staleBefore string) (bool, error)
	ClearResumeClaim(ctx context.Context, id string) error
	MarkOrphanedRunsCancelled(ctx context.Context) (int, error)
	TransitionPhase(ctx context.Context, id, fromPhase, toPhase string) (bool, error)
}

// Stability:stable
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

// Stability:evolving
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
	p := ParseSessionRunPhase(phase)
	if p == PhaseInteractive && strings.TrimSpace(strings.ToLower(phase)) != "interactive" {
		// Unrecognised value normalised to interactive (matches ParseSessionRunPhase default).
		return SessionRunPhaseInteractive
	}
	return string(p)
}

func sessionRunNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

var errSessionRunNotInit = apierror.Internal("SESSION_RUN", "SessionRunUsecase: not initialized")

func (u *SessionRunUsecase) StartInteractive(ctx context.Context, sessionID, turnID, runtimeRunID, source, agentID string) (SessionRun, error) {
	if u == nil || u.repo == nil {
		return SessionRun{}, errSessionRunNotInit
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return SessionRun{}, apierror.BadRequest("SESSION_RUN", "sessionID and turnID are required")
	}
	// 并发守卫：拒绝同一 Session 重复创建活跃 Run。
	// TODO(debt): 这是 TOCTOU 检查，最终保障应通过 DB 部分唯一索引实现：
	//   CREATE UNIQUE INDEX idx_session_runs_active ON session_runs(session_id)
	//   WHERE finished_at = '' AND phase IN ('interactive', 'durable')
	// 配合 entErrToBizErr 的 CodeConflict 翻译，可在 DB 层兜底。
	if existing, err := u.repo.GetActiveForSession(ctx, sessionID); err == nil && existing.ID != "" {
		return SessionRun{}, apierror.Conflict("SESSION_RUN", "active run already exists for session")
	} else if err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
		// 非 NotFound 的查询错误应原样返回，避免静默吞错误。
		return SessionRun{}, err
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
		SoftBudgetSec:  0,
		HardBudgetSec:  0,
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

// TransitionPhase performs a CAS phase transition validated by the state machine.
// It atomically transitions from the expected phase to the target phase only if
// the current DB phase matches fromPhase, preventing TOCTOU races (N-04 fix).
// Returns (true, nil) on success, (false, nil) if CAS failed (concurrent modification).
func (u *SessionRunUsecase) TransitionPhase(ctx context.Context, id string, event SessionRunPhaseEvent) (bool, error) {
	if u == nil || u.repo == nil {
		return false, errSessionRunNotInit
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, apierror.BadRequest("SESSION_RUN", "id is required")
	}
	cur, err := u.repo.Get(ctx, id)
	if err != nil {
		return false, err
	}
	fromPhase := ParseSessionRunPhase(cur.Phase)
	toPhase, err := sessionRunPhaseMachine.Transition(fromPhase, event)
	if err != nil {
		return false, fmt.Errorf("invalid phase transition from %s via %s: %w", fromPhase, event, err)
	}
	ok, err := u.repo.TransitionPhase(ctx, id, string(fromPhase), string(toPhase))
	if err != nil {
		return false, err
	}
	if !ok {
		u.lg.Warn("transition phase CAS failed: phase changed concurrently",
			loggateway.Str("id", id), loggateway.Str("from", string(fromPhase)), loggateway.Str("to", string(toPhase)))
	}
	return ok, nil
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
