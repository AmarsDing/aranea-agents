package service

import (
	"context"
	"testing"
)

func TestTerminalRunStatus_AwaitingUserPersists(t *testing.T) {
	if terminalRunStatus("awaiting_user") {
		t.Fatal("awaiting_user should persist in session state")
	}
}

func TestSessionAwaitingUser(t *testing.T) {
	s := &ChatService{orch: &ChatOrchestrator{}}
	_, ok := s.sessionAwaitingUser(context.Background(), "")
	if ok {
		t.Fatal("expected false for empty session")
	}
}

func TestTryBeginResume_dedup(t *testing.T) {
	s := &ChatService{orch: &ChatOrchestrator{}}
	if !s.tryBeginResume("sess-1") {
		t.Fatal("first resume should begin")
	}
	if s.tryBeginResume("sess-1") {
		t.Fatal("second resume should be rejected while in flight")
	}
	s.endResume("sess-1")
	if !s.tryBeginResume("sess-1") {
		t.Fatal("resume should begin again after end")
	}
	s.endResume("sess-1")
}
