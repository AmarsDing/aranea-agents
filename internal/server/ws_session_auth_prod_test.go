package server

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// stubSessionReader embeds session.SessionReader (nil) and only overrides
// GetSessionByID. Other methods will panic if called; tests only exercise
// CheckOwnership which solely depends on GetSessionByID.
type stubSessionReader struct {
	session.SessionReader
	sess session.Session
	err  error
}

func (s *stubSessionReader) GetSessionByID(_ context.Context, _ string) (session.Session, error) {
	return s.sess, s.err
}

func TestSessionAuthorizer_AllowsOwner(t *testing.T) {
	a := NewSessionAuthorizer(
		&stubSessionReader{sess: session.Session{ID: "s1", UserID: "u1"}},
		loggateway.NewNoop(),
	)
	if err := a.CheckOwnership(context.Background(), "s1", "u1"); err != nil {
		t.Fatalf("expected nil for owner, got %v", err)
	}
}

func TestSessionAuthorizer_DeniesNonOwner(t *testing.T) {
	a := NewSessionAuthorizer(
		&stubSessionReader{sess: session.Session{ID: "s1", UserID: "owner"}},
		loggateway.NewNoop(),
	)
	err := a.CheckOwnership(context.Background(), "s1", "intruder")
	if err == nil {
		t.Fatal("expected error for non-owner")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeForbidden {
		t.Fatalf("expected Forbidden, got %s", ae.Code)
	}
}

func TestSessionAuthorizer_SessionNotFound(t *testing.T) {
	a := NewSessionAuthorizer(
		&stubSessionReader{err: apierror.NotFound("SESSION", "missing")},
		loggateway.NewNoop(),
	)
	err := a.CheckOwnership(context.Background(), "missing", "u1")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeNotFound {
		t.Fatalf("expected NotFound preserved, got %v", err)
	}
}

func TestSessionAuthorizer_PropagatesRepoError(t *testing.T) {
	dbErr := errors.New("db down")
	a := NewSessionAuthorizer(
		&stubSessionReader{err: dbErr},
		loggateway.NewNoop(),
	)
	err := a.CheckOwnership(context.Background(), "s1", "u1")
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected db error propagated, got %v", err)
	}
}
