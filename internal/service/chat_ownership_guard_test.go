package service

import (
	"context"
	"errors"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ownershipSessionRepo fakes SessionRepo for assertSessionAccess tests: only
// GetSessionByID is exercised by the ownership gate; the embedded nil interface
// satisfies the remaining 12 port interfaces without implementations.
type ownershipSessionRepo struct {
	biz.SessionRepo
	sess biz.Session
	err  error
}

func (r ownershipSessionRepo) GetSessionByID(_ context.Context, _ string) (biz.Session, error) {
	return r.sess, r.err
}

func newOwnershipChatService(sess biz.Session, err error) *ChatService {
	uc := biz.NewSessionUsecase(ownershipSessionRepo{sess: sess, err: err},
		nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	return &ChatService{sessions: uc}
}

// TestChatServiceOwnershipGuardRejectsCrossUser pins cluster A (E2E-P0-01):
// every chat entry RPC that touches a session must reject an authenticated
// caller whose user ID differs from the session owner. The orch is left nil
// on purpose: the guard must fire before any orchestrator access, so reaching
// the nil orch would panic and fail the test loudly.
func TestChatServiceOwnershipGuardRejectsCrossUser(t *testing.T) {
	victim := biz.Session{ID: "victim-sess", UserID: "victim-1"}
	attackerCtx := ctxuser.WithUserID(context.Background(), "attacker-1")

	tests := []struct {
		name string
		call func(svc *ChatService) error
	}{
		{"SendChatMessage", func(svc *ChatService) error {
			_, err := svc.SendChatMessage(attackerCtx, &chatv1.SendChatMessageRequest{SessionId: "victim-sess"})
			return err
		}},
		{"SubmitChatMessage", func(svc *ChatService) error {
			_, err := svc.SubmitChatMessage(attackerCtx, &chatv1.SendChatMessageRequest{SessionId: "victim-sess"})
			return err
		}},
		{"RetrySession", func(svc *ChatService) error {
			_, err := svc.RetrySession(attackerCtx, &chatv1.RetrySessionRequest{SessionId: "victim-sess"})
			return err
		}},
		{"GetPendingMessages", func(svc *ChatService) error {
			_, err := svc.GetPendingMessages(attackerCtx, &chatv1.GetPendingMessagesRequest{SessionId: "victim-sess"})
			return err
		}},
		{"CancelPendingMessage", func(svc *ChatService) error {
			_, err := svc.CancelPendingMessage(attackerCtx, &chatv1.CancelPendingMessageRequest{SessionId: "victim-sess", PendingId: "p1"})
			return err
		}},
		{"UpdatePendingMessage", func(svc *ChatService) error {
			_, err := svc.UpdatePendingMessage(attackerCtx, &chatv1.UpdatePendingMessageRequest{SessionId: "victim-sess", PendingId: "p1", Content: "x"})
			return err
		}},
		{"InterruptAndSendMessage", func(svc *ChatService) error {
			_, err := svc.InterruptAndSendMessage(attackerCtx, &chatv1.InterruptAndSendMessageRequest{SessionId: "victim-sess", PendingId: "p1"})
			return err
		}},
		{"EnqueueUserMessage", func(svc *ChatService) error {
			_, err := svc.EnqueueUserMessage(attackerCtx, &chatv1.EnqueueUserMessageRequest{SessionId: "victim-sess", Content: "x"})
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

// TestChatServiceOwnershipGuardAllows pins the gate's allow paths so the fix
// does not break cron/channel/durable system callers or the session owner.
func TestChatServiceOwnershipGuardAllows(t *testing.T) {
	victim := biz.Session{ID: "victim-sess", UserID: "victim-1"}

	t.Run("default system caller bypasses", func(t *testing.T) {
		svc := newOwnershipChatService(victim, nil)
		// DefaultUserID (cron/channel/durable entrypoints) skips the check.
		if err := svc.assertSessionAccess(context.Background(), "victim-sess"); err != nil {
			t.Fatalf("default user should pass, got %v", err)
		}
	})
	t.Run("session owner passes", func(t *testing.T) {
		svc := newOwnershipChatService(victim, nil)
		ownerCtx := ctxuser.WithUserID(context.Background(), "victim-1")
		if err := svc.assertSessionAccess(ownerCtx, "victim-sess"); err != nil {
			t.Fatalf("owner should pass, got %v", err)
		}
	})
	t.Run("legacy empty-UserID session passes", func(t *testing.T) {
		svc := newOwnershipChatService(biz.Session{ID: "legacy-sess"}, nil)
		attackerCtx := ctxuser.WithUserID(context.Background(), "attacker-1")
		if err := svc.assertSessionAccess(attackerCtx, "legacy-sess"); err != nil {
			t.Fatalf("empty-UserID session should pass, got %v", err)
		}
	})
	t.Run("missing session maps to NotFound", func(t *testing.T) {
		svc := newOwnershipChatService(biz.Session{}, apierror.NotFound(apierror.DomainSession, "session not found"))
		ownerCtx := ctxuser.WithUserID(context.Background(), "victim-1")
		if err := svc.assertSessionAccess(ownerCtx, "missing-sess"); !apierror.IsCode(err, apierror.CodeNotFound) {
			t.Fatalf("expected NotFound, got %v", err)
		}
	})
	t.Run("repo error propagates", func(t *testing.T) {
		boom := errors.New("boom")
		svc := newOwnershipChatService(biz.Session{}, boom)
		ownerCtx := ctxuser.WithUserID(context.Background(), "victim-1")
		if err := svc.assertSessionAccess(ownerCtx, "victim-sess"); !errors.Is(err, boom) {
			t.Fatalf("expected repo error to propagate, got %v", err)
		}
	})
}

// TestChatServiceOwnershipGuardEmptySessionID pins BadRequest-before-guard so
// a blank session_id cannot probe ownership error distinguishability.
func TestChatServiceOwnershipGuardEmptySessionID(t *testing.T) {
	svc := newOwnershipChatService(biz.Session{}, nil)
	attackerCtx := ctxuser.WithUserID(context.Background(), "attacker-1")
	if _, err := svc.SendChatMessage(attackerCtx, &chatv1.SendChatMessageRequest{SessionId: "  "}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest for blank session_id, got %v", err)
	}
}
