package agent

import (
	"context"

	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// BizSessionIngestor implements trpcsession.Ingestor for Aranea.
// The runner also calls memory.Service.EnqueueAutoMemoryJob when MemoryService
// is configured; this hook records ingest metadata for external backends (e.g. mem0)
// without duplicating the auto-memory queue job.
type BizSessionIngestor struct {
	memory trpcmemory.Service
	lg     loggateway.Logger
}

func NewBizSessionIngestor(memory trpcmemory.Service, lg loggateway.Logger) trpcsession.Ingestor {
	if memory == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &BizSessionIngestor{memory: memory, lg: lg}
}

func (ing *BizSessionIngestor) IngestSession(ctx context.Context, sess *trpcsession.Session, opts ...trpcsession.IngestOption) error {
	if ing == nil || sess == nil {
		return nil
	}
	io := resolveIngestOptions(opts)
	ing.lg.Info("会话摄入 hook", loggateway.StepID("agent.session.ingest"), loggateway.Phase("done"),
		loggateway.Str("session_id", sess.ID),
		loggateway.Str("app", sess.AppName),
		loggateway.Str("user_id", sess.UserID),
		loggateway.Str("run_id", io.RunID),
		loggateway.Str("agent_id", io.AgentID),
		loggateway.Int("metadata_keys", len(io.Metadata)),
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
