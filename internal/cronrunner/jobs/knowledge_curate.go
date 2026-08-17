package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	// KnowledgeCurateDefaultInterval 默认治理周期。decay 每轮 ×0.9，不宜比梦境循环更密。
	KnowledgeCurateDefaultInterval = 6 * time.Hour
)

// KnowledgeCurator 一轮团队库治理（生产实现：*bizknowledge.Usecase）。
type KnowledgeCurator interface {
	CurateAllTeamKnowledge(ctx context.Context, opts bizknowledge.CurateOptions) ([]bizknowledge.CurateReport, error)
}

// KnowledgeCurateWorker 把 M4 词条治理从「仅 dream_cycle 且默认 dry_run」提升为后台周期实跑：
// 低风险任务（decay / promote / stale / distill）自动写入；高风险（conflict / orphan / moc）
// 只产 pending 提案，删除与事实冲突仍走工作台二审。
type KnowledgeCurateWorker struct {
	interval time.Duration
	curator  KnowledgeCurator
	lg       loggateway.Logger
}

func NewKnowledgeCurateWorker(
	interval time.Duration,
	curator KnowledgeCurator,
	lg loggateway.Logger,
) *KnowledgeCurateWorker {
	if curator == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeCurateDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeCurateWorker{
		interval: interval,
		curator:  curator,
		lg:       lg.With(loggateway.Domain("knowledge_curate")),
	}
}

func (w *KnowledgeCurateWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("knowledge curate worker started",
		loggateway.Str("interval", w.interval.String()))
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

func (w *KnowledgeCurateWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.lg.Error("knowledge curate panic recovered",
				loggateway.StepID("knowledge.curate"),
				loggateway.Any("panic", r))
		}
	}()
	reps, err := w.curator.CurateAllTeamKnowledge(ctx, bizknowledge.CurateOptions{DryRun: false})
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return
		}
		w.lg.Warn("knowledge curate pass failed",
			loggateway.StepID("knowledge.curate"),
			loggateway.Err(err))
		return
	}
	var decayed, closed, stale, distilled, pending int
	for _, r := range reps {
		decayed += r.DecayedEdges
		closed += r.ClosedEdges
		stale += r.StaleMarked
		distilled += r.DistilledFacts
		pending += r.ProposalsPending
	}
	if decayed > 0 || closed > 0 || stale > 0 || distilled > 0 || pending > 0 {
		w.lg.Info("knowledge curate pass completed",
			loggateway.StepID("knowledge.curate"),
			loggateway.Int("collections", len(reps)),
			loggateway.Int("decayed_edges", decayed),
			loggateway.Int("closed_edges", closed),
			loggateway.Int("stale_marked", stale),
			loggateway.Int("distilled_facts", distilled),
			loggateway.Int("proposals_pending", pending))
	}
}

// KnowledgeCurateDisabled 报告工人是否经 KNOWLEDGE_CURATE_DISABLED 禁用。
func KnowledgeCurateDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("KNOWLEDGE_CURATE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
