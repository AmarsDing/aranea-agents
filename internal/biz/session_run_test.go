package biz

import (
	"context"
	"strings"
	"testing"

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

func (s *sessionRunRepoStub) UpdatePhase(_ context.Context, id, phase string) error {
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.Phase = NormalizeSessionRunPhase(phase)
	s.runs[id] = run
	return nil
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


