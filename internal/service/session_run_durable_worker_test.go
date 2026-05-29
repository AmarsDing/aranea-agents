package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

type durableWorkerRepoStub struct {
	runs map[string]biz.SessionRun
}

func (s *durableWorkerRepoStub) Create(_ context.Context, run biz.SessionRun) (string, error) {
	if s.runs == nil {
		s.runs = map[string]biz.SessionRun{}
	}
	s.runs[run.ID] = run
	return run.ID, nil
}
func (s *durableWorkerRepoStub) UpdatePhase(_ context.Context, id, phase string) error {
	run := s.runs[id]
	run.Phase = biz.NormalizeSessionRunPhase(phase)
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) UpdateCheckpointID(_ context.Context, id, checkpointID string) error {
	run := s.runs[id]
	run.CheckpointID = checkpointID
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) MarkTerminal(_ context.Context, id, phase, errMsg string) error {
	run := s.runs[id]
	run.Phase = biz.NormalizeSessionRunPhase(phase)
	run.ErrorMessage = errMsg
	run.ResumeStartedAt = ""
	s.runs[id] = run
	return nil
}
func (s *durableWorkerRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	return s.runs[id], nil
}
func (s *durableWorkerRepoStub) GetActiveForSession(_ context.Context, _ string) (biz.SessionRun, error) {
	return biz.SessionRun{}, nil
}
func (s *durableWorkerRepoStub) ListByPhase(_ context.Context, phase string, _ int) ([]biz.SessionRun, error) {
	var out []biz.SessionRun
	for _, run := range s.runs {
		if run.Phase == biz.NormalizeSessionRunPhase(phase) && run.FinishedAt == "" {
			out = append(out, run)
		}
	}
	return out, nil
}
func (s *durableWorkerRepoStub) ListForJobs(_ context.Context, _ biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *durableWorkerRepoStub) ListBySession(_ context.Context, _ string, _, _ int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *durableWorkerRepoStub) TryClaimDurableResume(_ context.Context, id, _ string) (bool, error) {
	run, ok := s.runs[id]
	if !ok || run.Phase != biz.SessionRunPhaseDurable || run.CheckpointID == "" {
		return false, nil
	}
	if run.ResumeStartedAt != "" {
		return false, nil
	}
	run.ResumeStartedAt = "claimed"
	s.runs[id] = run
	return true, nil
}
func (s *durableWorkerRepoStub) ClearResumeClaim(_ context.Context, id string) error {
	run := s.runs[id]
	run.ResumeStartedAt = ""
	s.runs[id] = run
	return nil
}

func (s *durableWorkerRepoStub) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, nil
}

type durableWorkerCheckpointStub struct {
	cps map[string]biz.SessionRunCheckpoint
}

func (s *durableWorkerCheckpointStub) Create(_ context.Context, cp biz.SessionRunCheckpoint) (string, error) {
	if s.cps == nil {
		s.cps = map[string]biz.SessionRunCheckpoint{}
	}
	s.cps[cp.ID] = cp
	return cp.ID, nil
}
func (s *durableWorkerCheckpointStub) Get(_ context.Context, id string) (biz.SessionRunCheckpoint, error) {
	return s.cps[id], nil
}
func (s *durableWorkerCheckpointStub) GetBySessionRunID(_ context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	for _, cp := range s.cps {
		if cp.SessionRunID == sessionRunID {
			return cp, nil
		}
	}
	return biz.SessionRunCheckpoint{}, nil
}

func TestSessionRunDurableWorker_skipsUnclaimedDuplicate(t *testing.T) {
	repo := &durableWorkerRepoStub{runs: map[string]biz.SessionRun{
		"run-1": {
			ID:           "run-1",
			SessionID:    "sess-1",
			Phase:        biz.SessionRunPhaseDurable,
			CheckpointID: "cp-1",
		},
	}}
	cps := &durableWorkerCheckpointStub{
		cps: map[string]biz.SessionRunCheckpoint{
			"cp-1": {ID: "cp-1", SessionRunID: "run-1", PayloadJSON: `{"session_id":"sess-1","turn_id":"t1","agent_id":"a1"}`},
		},
	}
	uc := biz.NewSessionRunUsecase(repo, cps)
	chat := &ChatService{orch: &ChatOrchestrator{chTurn: ChannelTurnDeps{SessionRuns: uc}, runs: nil}}
	w := NewSessionRunDurableWorker(uc, chat, chat)
	w.processOnce(context.Background())
	if repo.runs["run-1"].ResumeStartedAt == "" {
		t.Fatal("expected claim on first process")
	}
	before := repo.runs["run-1"].ResumeStartedAt
	w.processOnce(context.Background())
	if repo.runs["run-1"].ResumeStartedAt != before {
		t.Fatal("second processOnce should not re-claim")
	}
}
