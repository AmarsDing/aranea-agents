package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type sessionRunRepoStub struct {
	runs map[string]SessionRun
}

func (s *sessionRunRepoStub) Create(_ context.Context, run SessionRun) (string, error) {
	if s.runs == nil {
		s.runs = map[string]SessionRun{}
	}
	s.runs[run.ID] = run
	return run.ID, nil
}

func (s *sessionRunRepoStub) UpdateCheckpointID(_ context.Context, id, checkpointID string) error {
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.CheckpointID = checkpointID
	s.runs[id] = run
	return nil
}

func (s *sessionRunRepoStub) MarkTerminal(_ context.Context, id, phase, errMsg string) error {
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.Phase = NormalizeSessionRunPhase(phase)
	run.ErrorMessage = errMsg
	s.runs[id] = run
	return nil
}

func (s *sessionRunRepoStub) MarkTerminalWherePhase(_ context.Context, id, fromPhase, toPhase, errMsg string) (bool, error) {
	run, ok := s.runs[id]
	if !ok {
		return false, nil
	}
	if run.Phase != NormalizeSessionRunPhase(fromPhase) {
		return false, nil
	}
	run.Phase = NormalizeSessionRunPhase(toPhase)
	run.ErrorMessage = errMsg
	run.FinishedAt = sessionRunNow()
	s.runs[id] = run
	return true, nil
}

func (s *sessionRunRepoStub) Get(_ context.Context, id string) (SessionRun, error) {
	return s.runs[id], nil
}

func (s *sessionRunRepoStub) GetActiveForSession(_ context.Context, sessionID string) (SessionRun, error) {
	for _, run := range s.runs {
		if run.SessionID != sessionID {
			continue
		}
		switch run.Phase {
		case SessionRunPhaseInteractive, SessionRunPhaseDurable:
			if run.FinishedAt == "" {
				return run, nil
			}
		}
	}
	return SessionRun{}, nil
}

func (s *sessionRunRepoStub) ListByPhase(_ context.Context, phase string, _ int) ([]SessionRun, error) {
	var out []SessionRun
	for _, run := range s.runs {
		if run.Phase == NormalizeSessionRunPhase(phase) {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *sessionRunRepoStub) ListForJobs(_ context.Context, q SessionRunListQuery) ([]SessionRun, error) {
	var out []SessionRun
	for _, run := range s.runs {
		if q.SessionID != "" && run.SessionID != q.SessionID {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

func (s *sessionRunRepoStub) ListBySession(_ context.Context, sessionID string, limit, offset int) ([]SessionRun, int, error) {
	var out []SessionRun
	for _, run := range s.runs {
		if run.SessionID == sessionID {
			out = append(out, run)
		}
	}
	total := len(out)
	if offset > total {
		return nil, total, nil
	}
	if limit > 0 && offset+limit < total {
		out = out[offset : offset+limit]
	} else {
		out = out[offset:]
	}
	return out, total, nil
}

func (s *sessionRunRepoStub) TryClaimDurableResume(_ context.Context, id, _ string) (bool, error) {
	run, ok := s.runs[id]
	if !ok || run.Phase != SessionRunPhaseDurable || strings.TrimSpace(run.CheckpointID) == "" {
		return false, nil
	}
	if run.FinishedAt != "" || strings.TrimSpace(run.ResumeStartedAt) != "" {
		return false, nil
	}
	run.ResumeStartedAt = sessionRunNow()
	s.runs[id] = run
	return true, nil
}

func (s *sessionRunRepoStub) ClearResumeClaim(_ context.Context, id string) error {
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.ResumeStartedAt = ""
	s.runs[id] = run
	return nil
}

func (s *sessionRunRepoStub) TransitionPhase(_ context.Context, id, fromPhase, toPhase string) (bool, error) {
	run, ok := s.runs[id]
	if !ok {
		return false, nil
	}
	if run.Phase != NormalizeSessionRunPhase(fromPhase) {
		return false, nil
	}
	run.Phase = NormalizeSessionRunPhase(toPhase)
	s.runs[id] = run
	return true, nil
}

func (s *sessionRunRepoStub) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, nil
}

type sessionRunCheckpointRepoStub struct {
	cps map[string]SessionRunCheckpoint
}

func (s *sessionRunCheckpointRepoStub) Create(_ context.Context, cp SessionRunCheckpoint) (string, error) {
	if s.cps == nil {
		s.cps = map[string]SessionRunCheckpoint{}
	}
	s.cps[cp.ID] = cp
	return cp.ID, nil
}

func (s *sessionRunCheckpointRepoStub) Get(_ context.Context, id string) (SessionRunCheckpoint, error) {
	return s.cps[id], nil
}

func (s *sessionRunCheckpointRepoStub) GetBySessionRunID(_ context.Context, sessionRunID string) (SessionRunCheckpoint, error) {
	for _, cp := range s.cps {
		if cp.SessionRunID == sessionRunID {
			return cp, nil
		}
	}
	return SessionRunCheckpoint{}, nil
}

func TestSessionRunUsecaseStartInteractive(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())
	run, err := uc.StartInteractive(context.Background(), "sess-1", "turn-1", "rt-1", "channel", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Phase != SessionRunPhaseInteractive {
		t.Fatalf("phase=%q", run.Phase)
	}
	if run.Source != "channel" {
		t.Fatalf("source=%q", run.Source)
	}
}

// TestSessionRunUsecase_StartInteractive_RejectsConcurrentActiveRun 验证 INV-1
// 并发守卫：当 Session 已存在活跃 Run（interactive/durable 且未结束）时，
// 再次调用 StartInteractive 必须返回 CodeConflict，避免违反
// "One Session one active Run" 不变量。
func TestSessionRunUsecase_StartInteractive_RejectsConcurrentActiveRun(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-existing": {
			ID:        "run-existing",
			SessionID: "sess-conflict",
			TurnID:    "turn-prev",
			Phase:     SessionRunPhaseInteractive,
			StartedAt: sessionRunNow(),
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())

	_, err := uc.StartInteractive(context.Background(), "sess-conflict", "turn-2", "rt-2", "channel", "agent-1")
	if err == nil {
		t.Fatal("expected Conflict error when active run exists, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("expected CodeConflict, got %v", err)
	}
}

// TestSessionRunUsecase_StartInteractive_AllowsAfterTerminalRun 验证 INV-1
// 守卫只阻断活跃 Run：当 Session 的前一个 Run 已终态（completed/failed/cancelled）
// 或 FinishedAt 已设置时，允许创建新 Run。
func TestSessionRunUsecase_StartInteractive_AllowsAfterTerminalRun(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-prev": {
			ID:         "run-prev",
			SessionID:  "sess-term",
			TurnID:     "turn-prev",
			Phase:      SessionRunPhaseCompleted,
			StartedAt:  sessionRunNow(),
			FinishedAt: sessionRunNow(),
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())

	run, err := uc.StartInteractive(context.Background(), "sess-term", "turn-2", "rt-2", "channel", "agent-1")
	if err != nil {
		t.Fatalf("expected success after terminal run, got %v", err)
	}
	if run.Phase != SessionRunPhaseInteractive {
		t.Fatalf("phase=%q", run.Phase)
	}
}

// TestSessionRunUsecase_StartInteractive_PropagatesRepoError 验证 INV-1
// 守卫不会静默吞掉 GetActiveForSession 的非 NotFound 错误。
func TestSessionRunUsecase_StartInteractive_PropagatesRepoError(t *testing.T) {
	repo := &sessionRunRepoStubErr{
		err: errors.New("db connection lost"),
	}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())

	_, err := uc.StartInteractive(context.Background(), "sess-err", "turn-1", "rt-1", "channel", "agent-1")
	if err == nil {
		t.Fatal("expected repo error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "db connection lost") {
		t.Fatalf("expected error to contain 'db connection lost', got %v", err)
	}
}

// sessionRunRepoStubErr 是一个总是返回指定错误的 stub，用于测试错误传播。
type sessionRunRepoStubErr struct {
	err error
}

func (s *sessionRunRepoStubErr) Create(_ context.Context, _ SessionRun) (string, error) {
	return "", s.err
}
func (s *sessionRunRepoStubErr) UpdateCheckpointID(_ context.Context, _, _ string) error {
	return s.err
}
func (s *sessionRunRepoStubErr) MarkTerminal(_ context.Context, _, _, _ string) error { return s.err }
func (s *sessionRunRepoStubErr) MarkTerminalWherePhase(_ context.Context, _, _, _, _ string) (bool, error) {
	return false, s.err
}
func (s *sessionRunRepoStubErr) Get(_ context.Context, _ string) (SessionRun, error) {
	return SessionRun{}, s.err
}
func (s *sessionRunRepoStubErr) GetActiveForSession(_ context.Context, _ string) (SessionRun, error) {
	return SessionRun{}, s.err
}
func (s *sessionRunRepoStubErr) ListByPhase(_ context.Context, _ string, _ int) ([]SessionRun, error) {
	return nil, s.err
}
func (s *sessionRunRepoStubErr) ListForJobs(_ context.Context, _ SessionRunListQuery) ([]SessionRun, error) {
	return nil, s.err
}
func (s *sessionRunRepoStubErr) ListBySession(_ context.Context, _ string, _, _ int) ([]SessionRun, int, error) {
	return nil, 0, s.err
}
func (s *sessionRunRepoStubErr) TryClaimDurableResume(_ context.Context, _, _ string) (bool, error) {
	return false, s.err
}
func (s *sessionRunRepoStubErr) ClearResumeClaim(_ context.Context, _ string) error { return s.err }
func (s *sessionRunRepoStubErr) TransitionPhase(_ context.Context, _, _, _ string) (bool, error) {
	return false, s.err
}
func (s *sessionRunRepoStubErr) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, s.err
}

func TestSuggestDurableRun_autoKeywords(t *testing.T) {
	cfg := ParseChannelLongTaskConfig(`{"config":{"execution_mode":"auto","async_graph_id":"g1"}}`)
	if !cfg.SuggestDurableRun("请做全量分析") {
		t.Fatal("expected suggest")
	}
	if cfg.ShouldRunAsync("请做全量分析") {
		t.Fatal("keywords must not route in auto mode (CC-R-05)")
	}
	if !cfg.ShouldRunAsync("/async help") {
		t.Fatal("/async should still route")
	}
}

func TestSessionRunTryClaimDurableResume(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-1": {
			ID:           "run-1",
			SessionID:    "sess-1",
			Phase:        SessionRunPhaseDurable,
			CheckpointID: "cp-1",
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())
	claimed, err := uc.TryClaimDurableResume(context.Background(), "run-1")
	if err != nil || !claimed {
		t.Fatalf("first claim: claimed=%v err=%v", claimed, err)
	}
	claimed2, err := uc.TryClaimDurableResume(context.Background(), "run-1")
	if err != nil || claimed2 {
		t.Fatalf("duplicate claim: claimed=%v err=%v", claimed2, err)
	}
	if err := uc.ClearResumeClaim(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	claimed3, err := uc.TryClaimDurableResume(context.Background(), "run-1")
	if err != nil || !claimed3 {
		t.Fatalf("after clear: claimed=%v err=%v", claimed3, err)
	}
}

// TestSessionRunUsecase_Complete_ValidTransition verifies that Complete
// transitions an interactive run to completed via the state machine.
func TestSessionRunUsecase_Complete_ValidTransition(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-1": {
			ID:        "run-1",
			SessionID: "sess-1",
			Phase:     SessionRunPhaseInteractive,
			StartedAt: sessionRunNow(),
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())
	if err := uc.Complete(context.Background(), "run-1"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	run := repo.runs["run-1"]
	if run.Phase != SessionRunPhaseCompleted {
		t.Errorf("phase=%q want %q", run.Phase, SessionRunPhaseCompleted)
	}
	if run.FinishedAt == "" {
		t.Error("expected FinishedAt to be set")
	}
}

// TestSessionRunUsecase_Complete_RejectedFromTerminal verifies that Complete
// is rejected when the run is already terminal (state machine enforcement).
func TestSessionRunUsecase_Complete_RejectedFromTerminal(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-1": {
			ID:         "run-1",
			SessionID:  "sess-1",
			Phase:      SessionRunPhaseCompleted,
			StartedAt:  sessionRunNow(),
			FinishedAt: sessionRunNow(),
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())
	err := uc.Complete(context.Background(), "run-1")
	if err == nil {
		t.Fatal("expected error for terminal->terminal transition, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected CodeBadRequest, got %v", err)
	}
}

// TestSessionRunUsecase_Fail_SetsErrorMessage verifies that Fail transitions
// an interactive run to failed and records the error message.
func TestSessionRunUsecase_Fail_SetsErrorMessage(t *testing.T) {
	repo := &sessionRunRepoStub{runs: map[string]SessionRun{
		"run-1": {
			ID:        "run-1",
			SessionID: "sess-1",
			Phase:     SessionRunPhaseInteractive,
			StartedAt: sessionRunNow(),
		},
	}}
	uc := NewSessionRunUsecase(repo, nil, loggateway.NewNoop())
	if err := uc.Fail(context.Background(), "run-1", "something went wrong"); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
	run := repo.runs["run-1"]
	if run.Phase != SessionRunPhaseFailed {
		t.Errorf("phase=%q want %q", run.Phase, SessionRunPhaseFailed)
	}
	if run.ErrorMessage != "something went wrong" {
		t.Errorf("error_message=%q want %q", run.ErrorMessage, "something went wrong")
	}
	if run.FinishedAt == "" {
		t.Error("expected FinishedAt to be set")
	}
}
