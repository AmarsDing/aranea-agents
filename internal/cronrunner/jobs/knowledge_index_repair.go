package jobs

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	KnowledgeIndexRepairDefaultInterval = 5 * time.Minute
	knowledgeIndexRepairMaxPerPass      = 20
)

// KnowledgeIndexRepairer repairs documents whose durable content exists but
// derived chunks/embeddings are missing. Implemented by KnowledgeService.
type KnowledgeIndexRepairer interface {
	RepairPendingKnowledgeIndexes(ctx context.Context, limit int) (repaired, failed int, err error)
}

// KnowledgeIndexRepairWorker closes the eventual-consistency loop for failed
// or interrupted team-vault index writes.
type KnowledgeIndexRepairWorker struct {
	interval time.Duration
	repairer KnowledgeIndexRepairer
	lg       loggateway.Logger
}

func NewKnowledgeIndexRepairWorker(
	interval time.Duration,
	repairer KnowledgeIndexRepairer,
	lg loggateway.Logger,
) *KnowledgeIndexRepairWorker {
	if repairer == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeIndexRepairDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeIndexRepairWorker{
		interval: interval,
		repairer: repairer,
		lg:       lg.With(loggateway.Domain("knowledge_index_repair")),
	}
}

func (w *KnowledgeIndexRepairWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.RunOnce(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

func (w *KnowledgeIndexRepairWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	repaired, failed, err := w.repairer.RepairPendingKnowledgeIndexes(ctx, knowledgeIndexRepairMaxPerPass)
	if err != nil {
		w.lg.Warn("knowledge index repair pass failed",
			loggateway.StepID("knowledge.index_repair"),
			loggateway.Err(err))
		return
	}
	if repaired > 0 || failed > 0 {
		w.lg.Info("knowledge index repair pass completed",
			loggateway.StepID("knowledge.index_repair"),
			loggateway.Int("repaired", repaired),
			loggateway.Int("failed", failed))
	}
}
