package service

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// TestChatServiceOwnershipGuardRound2 pins the S-bucket follow-ups to the
// round-2 review: AwaitUserReply (S-1), PauseSession/ResumeSession (S-3) and
// SubmitMessageFeedback (S-6) must reject an authenticated caller whose user
// ID differs from the session owner. The orch is nil on purpose — the gate
// must fire before any orchestrator access (reaching it would panic loudly).
func TestChatServiceOwnershipGuardRound2(t *testing.T) {
	victim := biz.Session{ID: "victim-sess", UserID: "victim-1"}
	attackerCtx := ctxuser.WithUserID(context.Background(), "attacker-1")

	tests := []struct {
		name string
		call func(svc *ChatService) error
	}{
		{"AwaitUserReply", func(svc *ChatService) error {
			_, err := svc.AwaitUserReply(attackerCtx, &chatv1.AwaitUserReplyRequest{SessionId: "victim-sess", Reply: "yes"})
			return err
		}},
		{"PauseSession", func(svc *ChatService) error {
			_, err := svc.PauseSession(attackerCtx, &chatv1.PauseSessionRequest{SessionId: "victim-sess"})
			return err
		}},
		{"ResumeSession", func(svc *ChatService) error {
			_, err := svc.ResumeSession(attackerCtx, &chatv1.ResumeSessionRequest{SessionId: "victim-sess"})
			return err
		}},
		{"SubmitMessageFeedback", func(svc *ChatService) error {
			_, err := svc.SubmitMessageFeedback(attackerCtx, &chatv1.SubmitMessageFeedbackRequest{
				SessionId: "victim-sess", MessageId: "m1", Rating: "positive",
			})
			return err
		}},
		{"ListChatBackgroundJobs session scope", func(svc *ChatService) error {
			sid := "victim-sess"
			_, err := svc.ListChatBackgroundJobs(attackerCtx, &chatv1.ListChatBackgroundJobsRequest{SessionId: &sid})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newOwnershipChatService(victim, nil)
			if err := tt.call(svc); !apierror.IsCode(err, apierror.CodeForbidden) {
				t.Fatalf("expected Forbidden for cross-user access, got %v", err)
			}
		})
	}
}

// ownershipMapSessionRepo resolves sessions by ID so filter tests can mix
// owned and legacy rows in one service instance.
type ownershipMapSessionRepo struct {
	biz.SessionRepo
	byID map[string]biz.Session
}

func (r ownershipMapSessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	if s, ok := r.byID[id]; ok {
		return s, nil
	}
	return biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found")
}

// TestFilterChatBackgroundJobsBySessionAccess pins the S-5 agent-scoped
// post-filter: rows whose session the caller cannot access must be dropped
// so ContentPreview/ErrorMessage cannot cross tenants.
func TestFilterChatBackgroundJobsBySessionAccess(t *testing.T) {
	uc := biz.NewSessionUsecase(ownershipMapSessionRepo{byID: map[string]biz.Session{
		"victim-sess": {ID: "victim-sess", UserID: "victim-1"},
		"legacy-sess": {ID: "legacy-sess"},
	}}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	svc := &ChatService{sessions: uc}
	attackerCtx := ctxuser.WithUserID(context.Background(), "attacker-1")

	own := &chatv1.ChatBackgroundJob{Id: "own", SessionId: "legacy-sess"}        // empty UserID → allowed
	foreign := &chatv1.ChatBackgroundJob{Id: "foreign", SessionId: "victim-sess"} // victim-owned → dropped

	out := svc.filterChatBackgroundJobsBySessionAccess(attackerCtx, []*chatv1.ChatBackgroundJob{own, foreign, nil})
	if len(out) != 1 || out[0].GetId() != "own" {
		t.Fatalf("expected only the accessible row to survive, got %+v", out)
	}

	// default_user/system callers bypass (cron/channel entrypoints).
	out = svc.filterChatBackgroundJobsBySessionAccess(context.Background(), []*chatv1.ChatBackgroundJob{own, foreign})
	if len(out) != 2 {
		t.Fatalf("system caller should see all rows, got %d", len(out))
	}
}
