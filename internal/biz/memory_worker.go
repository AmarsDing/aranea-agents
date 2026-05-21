package biz

import (
	"context"

	"aranea-agents/internal/event"
	"strings"
	"time"

	memtrpc "aranea-agents/internal/memory/trpc"
)

// TurnMemoryWorker schedules post-turn memory extraction (EP-MEM-01).
type TurnMemoryWorker struct{}

// NewTurnMemoryWorker constructs a turn memory worker.
func NewTurnMemoryWorker() *TurnMemoryWorker {
	return &TurnMemoryWorker{}
}

// OnRunnerCompletion enqueues heuristic extraction after runner completion.
func (w *TurnMemoryWorker) OnRunnerCompletion(_ context.Context, de DomainEvent) {
	_ = w
	sid := strings.TrimSpace(de.SessionID)
	if sid == "" {
		return
	}
	memtrpc.EnqueueAutoMemory(memtrpc.AutoMemoryJobRequest{
		AppName:    strings.TrimSpace(de.Author),
		SessionID:  sid,
		EnqueuedAt: time.Now().UTC(),
	})
	event.SessionSysLogWarn(context.Background(), sid, "system.memory_worker.enqueue", "自动记忆任务已入队")
}

