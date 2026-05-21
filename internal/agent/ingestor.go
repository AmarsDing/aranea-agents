package agent

import (
	"context"

	"aranea-agents/internal/event"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// BizSessionIngestor implements trpcsession.Ingestor for Aranea.
// The runner also calls memory.Service.EnqueueAutoMemoryJob when MemoryService
// is configured; this hook records ingest metadata for external backends (e.g. mem0)
// without duplicating the auto-memory queue job.
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
	io := resolveIngestOptions(opts)
	event.CtxFlowLogDone(ctx, "system.session.ingest", "会话摄入 hook",
		event.P("session_id", sess.ID),
		event.P("app", sess.AppName),
		event.P("user_id", sess.UserID),
		event.P("run_id", io.RunID),
		event.P("agent_id", io.AgentID),
		event.P("metadata_keys", len(io.Metadata)),
	)
	// External backends (mem0, etc.) can extend this type; auto-memory stays on
	// runner.enqueueAutoMemoryJob → EnqueueAutoMemoryJob.
	return nil
}

type ingestOptions struct {
	RunID    string
	AgentID  string
	Metadata map[string]any
}

func resolveIngestOptions(opts []trpcsession.IngestOption) ingestOptions {
	var req trpcsession.IngestOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	return ingestOptions{
		RunID:    req.RunID,
		AgentID:  req.AgentID,
		Metadata: req.Metadata,
	}
}
