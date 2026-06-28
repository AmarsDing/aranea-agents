package service

import (
	"context"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/pkg/apierror"
)

// TestRetrySession_EmptySessionID verifies that an empty session_id is rejected
// with BAD_REQUEST before any backend lookup is performed.
func TestRetrySession_EmptySessionID(t *testing.T) {
	svc := &ChatService{}
	_, err := svc.RetrySession(context.Background(), &chatv1.RetrySessionRequest{SessionId: "  "})
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BAD_REQUEST, got %v", err)
	}
}

// TestRetrySession_NilSessions verifies that a ChatService without a wired
// SessionUsecase (sessions==nil, e.g. test stubs) returns INTERNAL rather than
// panicking on a nil pointer dereference.
func TestRetrySession_NilSessions(t *testing.T) {
	svc := &ChatService{} // sessions is nil
	_, err := svc.RetrySession(context.Background(), &chatv1.RetrySessionRequest{SessionId: "sess-1"})
	if err == nil {
		t.Fatal("expected error when sessions is nil")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected INTERNAL, got %v", err)
	}
}
