package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

type escalateSessionRunRepoStub struct {
	run biz.SessionRun
}

func (s *escalateSessionRunRepoStub) Create(_ context.Context, run biz.SessionRun) (string, error) {
	s.run = run
	return run.ID, nil
}
func (s *escalateSessionRunRepoStub) UpdatePhase(_ context.Context, id, phase string) error {
	if s.run.ID == id {
		s.run.Phase = biz.NormalizeSessionRunPhase(phase)
	}
	return nil
}
func (s *escalateSessionRunRepoStub) UpdateCheckpointID(_ context.Context, id, checkpointID string) error {
	if s.run.ID == id {
		s.run.CheckpointID = checkpointID
	}
	return nil
}
func (s *escalateSessionRunRepoStub) MarkTerminal(_ context.Context, id, phase, errMsg string) error {
	if s.run.ID == id {
		s.run.Phase = biz.NormalizeSessionRunPhase(phase)
		s.run.ErrorMessage = errMsg
	}
	return nil
}
func (s *escalateSessionRunRepoStub) Get(_ context.Context, id string) (biz.SessionRun, error) {
	if s.run.ID == id {
		return s.run, nil
	}
	return biz.SessionRun{}, nil
}
func (s *escalateSessionRunRepoStub) GetActiveForSession(_ context.Context, _ string) (biz.SessionRun, error) {
	return biz.SessionRun{}, nil
}
func (s *escalateSessionRunRepoStub) ListByPhase(_ context.Context, _ string, _ int) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *escalateSessionRunRepoStub) ListForJobs(_ context.Context, _ biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *escalateSessionRunRepoStub) ListBySession(_ context.Context, _ string, _, _ int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *escalateSessionRunRepoStub) TryClaimDurableResume(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}
func (s *escalateSessionRunRepoStub) ClearResumeClaim(_ context.Context, _ string) error {
	return nil
}
func (s *escalateSessionRunRepoStub) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, nil
}

type escalateCheckpointRepoStub struct {
	cps map[string]biz.SessionRunCheckpoint
}

func (s *escalateCheckpointRepoStub) Create(_ context.Context, cp biz.SessionRunCheckpoint) (string, error) {
	if s.cps == nil {
		s.cps = map[string]biz.SessionRunCheckpoint{}
	}
	s.cps[cp.ID] = cp
	return cp.ID, nil
}
func (s *escalateCheckpointRepoStub) Get(_ context.Context, id string) (biz.SessionRunCheckpoint, error) {
	return s.cps[id], nil
}
func (s *escalateCheckpointRepoStub) GetBySessionRunID(_ context.Context, sessionRunID string) (biz.SessionRunCheckpoint, error) {
	for _, cp := range s.cps {
		if cp.SessionRunID == sessionRunID {
			return cp, nil
		}
	}
	return biz.SessionRunCheckpoint{}, nil
}

func TestEscalateSessionRun_ownershipDenied(t *testing.T) {
	repo := &escalateSessionRunRepoStub{
		run: biz.SessionRun{
			ID:        "run-1",
			SessionID: "sess-owner",
			Phase:     biz.SessionRunPhaseEscalating,
		},
	}
	svc := &ChatService{
		orch: &ChatOrchestrator{
			chTurn: ChannelTurnDeps{
				SessionRuns: biz.NewSessionRunUsecase(repo, nil),
			},
		},
	}
	reply, err := svc.EscalateSessionRun(context.Background(), "run-1", "sess-other")
	if err != nil {
		t.Fatal(err)
	}
	if reply != channelBackgroundReplyDenied {
		t.Fatalf("reply=%q", reply)
	}
	if repo.run.Phase != biz.SessionRunPhaseEscalating {
		t.Fatalf("phase should not change; got=%q", repo.run.Phase)
	}
}

func TestEscalateSessionRun_ownershipAllowed(t *testing.T) {
	repo := &escalateSessionRunRepoStub{
		run: biz.SessionRun{
			ID:        "run-1",
			SessionID: "sess-owner",
			Phase:     biz.SessionRunPhaseInteractive,
			AgentID:   "agent-1",
		},
	}
	cps := &escalateCheckpointRepoStub{cps: map[string]biz.SessionRunCheckpoint{}}
	svc := &ChatService{
		orch: &ChatOrchestrator{
			chTurn: ChannelTurnDeps{
				SessionRuns: biz.NewSessionRunUsecase(repo, cps),
			},
		},
	}
	reply, err := svc.EscalateSessionRun(context.Background(), "run-1", "sess-owner")
	if err != nil {
		t.Fatal(err)
	}
	if reply != channelBackgroundReplyOK {
		t.Fatalf("reply=%q", reply)
	}
	if repo.run.Phase != biz.SessionRunPhaseDurable {
		t.Fatalf("phase=%q", repo.run.Phase)
	}
	if repo.run.CheckpointID == "" {
		t.Fatal("expected checkpoint")
	}
}
