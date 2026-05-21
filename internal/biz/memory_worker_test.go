package biz

import (
	"testing"
	"time"

	memtrpc "aranea-agents/internal/memory/trpc"
)

func TestTurnMemoryWorker_OnRunnerCompletion_EnqueuesJob(t *testing.T) {
	q := memtrpc.NewMemoryJobQueue(8, 30*time.Second)
	prev := memtrpc.SetGlobalAutoMemoryQueueForTest(q)
	defer memtrpc.SetGlobalAutoMemoryQueueForTest(prev)

	w := NewTurnMemoryWorker()
	w.OnRunnerCompletion(nil, DomainEvent{SessionID: "sess-1", Author: "agent-a"})

	select {
	case job := <-q.Chan():
		if job.SessionID != "sess-1" || job.AppName != "agent-a" {
			t.Fatalf("unexpected job: %+v", job)
		}
	case <-time.After(time.Second):
		t.Fatal("expected auto-memory job")
	}
}

func TestTurnMemoryWorker_OnRunnerCompletion_SkipsEmptySession(t *testing.T) {
	q := memtrpc.NewMemoryJobQueue(8, 30*time.Second)
	prev := memtrpc.SetGlobalAutoMemoryQueueForTest(q)
	defer memtrpc.SetGlobalAutoMemoryQueueForTest(prev)

	w := NewTurnMemoryWorker()
	w.OnRunnerCompletion(nil, DomainEvent{SessionID: "  ", Author: "agent-a"})

	select {
	case job := <-q.Chan():
		t.Fatalf("unexpected job: %+v", job)
	default:
	}
}
