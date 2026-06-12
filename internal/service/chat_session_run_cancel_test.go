package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

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
