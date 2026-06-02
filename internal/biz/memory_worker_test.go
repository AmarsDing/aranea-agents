package biz

import (
	"context"
	"testing"
	"time"
)

type noopSessionLogWriter struct{}

func (noopSessionLogWriter) LogSessionWarn(_ context.Context, _, _, _ string, _ ...LogPair)  {}
func (noopSessionLogWriter) LogSessionError(_ context.Context, _, _, _ string, _ ...LogPair) {}

func TestTurnMemoryWorker_OnRunnerCompletion_NoDuplicateEnqueue(t *testing.T) {
	w := NewTurnMemoryWorker(nil, noopSessionLogWriter{})
	w.OnRunnerCompletion(context.Background(), DomainEvent{SessionID: "sess-1", Author: "agent-a"})
}

func TestTurnMemoryWorker_OnRunnerCompletion_SkipsEmptySession(t *testing.T) {
	w := NewTurnMemoryWorker(nil, noopSessionLogWriter{})
	w.OnRunnerCompletion(context.Background(), DomainEvent{SessionID: "  ", Author: "agent-a"})
}

func TestTurnMemoryWorker_OnUserFeedback_EnqueuesJob(t *testing.T) {
	var capturedSessionID, capturedMessageID, capturedRating string
	feedback := FeedbackMemoryEnqueuerFunc(func(sessionID, messageID, rating, _ string, _ time.Time) {
		capturedSessionID = sessionID
		capturedMessageID = messageID
		capturedRating = rating
	})
	w := NewTurnMemoryWorker(feedback, noopSessionLogWriter{})
	w.OnUserFeedback(context.Background(), "sess-1", "msg-1", "positive", "great")
	if capturedSessionID != "sess-1" {
		t.Fatalf("expected sessionID=sess-1, got %s", capturedSessionID)
	}
	if capturedMessageID != "msg-1" {
		t.Fatalf("expected messageID=msg-1, got %s", capturedMessageID)
	}
	if capturedRating != "positive" {
		t.Fatalf("expected rating=positive, got %s", capturedRating)
	}
}
