package biz

import (
	"context"
	"encoding/json"

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

// monitorRunnerCompletionMeta builds JSON metadata for monitor_events.
func monitorRunnerCompletionMeta(de DomainEvent) string {
	meta := map[string]any{
		"session_id": de.SessionID,
		"author":     de.Author,
		"team_id":    de.TeamID,
	}
	if de.Usage != nil {
		meta["prompt_tokens"] = de.Usage.PromptTokens
		meta["completion_tokens"] = de.Usage.CompletionTokens
		meta["total_tokens"] = de.Usage.TotalTokens
	}
	if de.Error != nil {
		meta["error"] = de.Error.Message
	}
	raw, _ := json.Marshal(meta)
	return string(raw)
}
