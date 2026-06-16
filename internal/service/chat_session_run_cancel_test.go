package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// escalateSessionRunRepoStub implements biz.SessionRunRepo for the cancel test.
type escalateSessionRunRepoStub struct {
	run biz.SessionRun
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
func (s *escalateSessionRunRepoStub) ListBySession(_ context.Context, _ string, _, _ int) ([]biz.SessionRun, int, error) {
	return nil, 0, nil
}
func (s *escalateSessionRunRepoStub) ListForJobs(_ context.Context, _ biz.SessionRunListQuery) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *escalateSessionRunRepoStub) ListByPhase(_ context.Context, _ string, _ int) ([]biz.SessionRun, error) {
	return nil, nil
}
func (s *escalateSessionRunRepoStub) Create(_ context.Context, _ biz.SessionRun) (string, error) { return "", nil }
func (s *escalateSessionRunRepoStub) UpdatePhase(_ context.Context, _, _ string) error            { return nil }
func (s *escalateSessionRunRepoStub) UpdateCheckpointID(_ context.Context, _, _ string) error     { return nil }
func (s *escalateSessionRunRepoStub) MarkTerminal(_ context.Context, _, _, _ string) error        { return nil }
func (s *escalateSessionRunRepoStub) TryClaimDurableResume(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *escalateSessionRunRepoStub) ClearResumeClaim(_ context.Context, _ string) error { return nil }
func (s *escalateSessionRunRepoStub) MarkOrphanedRunsCancelled(_ context.Context) (int, error) {
	return 0, nil
}
func (s *escalateSessionRunRepoStub) TransitionPhase(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

func TestCancelSessionRunForCard_ownershipDenied(t *testing.T) {
	repo := &escalateSessionRunRepoStub{
		run: biz.SessionRun{
			ID:        "run-1",
			SessionID: "sess-owner",
			Phase:     biz.SessionRunPhaseInteractive,
		},
	}
	svc := &ChatService{
		lg: loggateway.NewNoop(),
		orch: &ChatOrchestrator{
			channelDeps: ChatChannelDeps{
				ChJobs: ChannelTurnJobDeps{
					SessionRuns: biz.NewSessionRunUsecase(repo, nil, nil),
				},
			},
		},
	}
	cancelled, reply := svc.CancelSessionRunForCard(context.Background(), "run-1", "sess-other")
	if cancelled {
		t.Fatal("expected not cancelled")
	}
	if reply != channelBackgroundReplyDenied {
		t.Fatalf("reply=%q", reply)
	}
}
