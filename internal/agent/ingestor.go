package agent

import (
	"context"
	"log/slog"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// BizSessionIngestor implements trpcsession.Ingestor for Aranea.
// The runner also calls memory.Service.EnqueueAutoMemoryJob when MemoryService
// is configured; this ingestor is a no-op today to avoid duplicate queue jobs.
// External backends (e.g. mem0) can replace or extend this type later.
type BizSessionIngestor struct {
	memory trpcmemory.Service
}

// NewBizSessionIngestor returns an ingestor when memory is available.
func NewBizSessionIngestor(memory trpcmemory.Service) trpcsession.Ingestor {
	if memory == nil {
		return nil
	}
	return &BizSessionIngestor{memory: memory}
}

func (ing *BizSessionIngestor) IngestSession(ctx context.Context, sess *trpcsession.Session, opts ...trpcsession.IngestOption) error {
	if ing == nil || sess == nil {
		return nil
	}
	_ = ctx
	_ = opts
	// Auto-memory extraction is handled by runner.enqueueAutoMemoryJob → EnqueueAutoMemoryJob.
	slog.Debug("session ingest hook (noop until external backend is wired)",
		"session_id", sess.ID, "app", sess.AppName, "user_id", sess.UserID)
	return nil
}
