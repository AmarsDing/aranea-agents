package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	// KnowledgeWriteBackSweeperDefaultInterval 重放周期。写回失败多为瞬时故障
	// （团队库维护窗/DB 抖动），2 分钟粒度兼顾恢复时延与扫描成本。
	KnowledgeWriteBackSweeperDefaultInterval = 2 * time.Minute
	// knowledgeWriteBackSweepBatch 单轮扫描上限（与 decision replay 同口径）。
	knowledgeWriteBackSweepBatch = 100
)

// KnowledgeWriteBackSweeperWorker 重放 G-10 写回重试队列：AutoMemory 写回团队库
// 失败时已落 knowledge_writeback_outbox（AS-EVT-01 Important 分级），本 worker
// 把 pending 批次重投至 WriteBackSessionFacts——成功标 published、失败记 attempt
// （触上限翻 dead）、poison 载荷直接翻 dead 终态。
type KnowledgeWriteBackSweeperWorker struct {
	interval  time.Duration
	outbox    biz.KnowledgeWriteBackOutboxRepo
	writeBack biz.KnowledgeWriteBack
	lg        loggateway.Logger
}

// NewKnowledgeWriteBackSweeperWorker 装配重放 worker；outbox/writeBack 任一缺失
// 返回 nil（ByDesign 不拉空转任务——装配期可见）。
func NewKnowledgeWriteBackSweeperWorker(
	interval time.Duration,
	outbox biz.KnowledgeWriteBackOutboxRepo,
	writeBack biz.KnowledgeWriteBack,
	lg loggateway.Logger,
) *KnowledgeWriteBackSweeperWorker {
	if outbox == nil || writeBack == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeWriteBackSweeperDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeWriteBackSweeperWorker{
		interval:  interval,
		outbox:    outbox,
		writeBack: writeBack,
		lg:        lg.With(loggateway.Domain("knowledge_writeback_sweeper")),
	}
}

func (w *KnowledgeWriteBackSweeperWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("knowledge writeback sweeper worker started",
		loggateway.Str("interval", w.interval.String()))
	w.RunOnce(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.lg.Info("knowledge writeback sweeper worker stopped")
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce 重放一轮 pending 批次（最老优先）。返回 (published, dead)，测试可断言。
func (w *KnowledgeWriteBackSweeperWorker) RunOnce(ctx context.Context) (published, dead int) {
	if w == nil {
		return 0, 0
	}
	rows, err := w.outbox.ListPendingWriteBackOutbox(ctx, knowledgeWriteBackSweepBatch)
	if err != nil {
		w.lg.Warn("知识库写回重试队列扫描失败",
			loggateway.StepID("knowledge.writeback.sweep_scan"),
			loggateway.Err(err))
		return 0, 0
	}
	if len(rows) == 0 {
		return 0, 0
	}
	publishedIDs := make([]int64, 0, len(rows))
	deadIDs := make([]int64, 0, 1)
	for _, row := range rows {
		in, decErr := biz.KnowledgeDecodeWriteBackPayload([]byte(row.Payload))
		if decErr != nil {
			// Poison 行：重试永不会成功，直接翻 dead（attempts 不增——非投递失败）。
			w.lg.Error("知识库写回重试队列 poison 行",
				loggateway.StepID("knowledge.writeback.poison"),
				loggateway.Int64("outbox_id", row.ID),
				loggateway.Err(decErr))
			deadIDs = append(deadIDs, row.ID)
			continue
		}
		if _, wbErr := w.writeBack.WriteBackSessionFacts(ctx, in); wbErr != nil {
			if mErr := w.outbox.MarkWriteBackOutboxAttempt(ctx, row.ID, wbErr.Error()); mErr != nil {
				w.lg.Warn("知识库写回重试 attempt 标记失败",
					loggateway.StepID("knowledge.writeback.sweep_mark"),
					loggateway.Err(mErr))
			}
			continue
		}
		publishedIDs = append(publishedIDs, row.ID)
	}
	if len(deadIDs) > 0 {
		if err := w.outbox.MarkWriteBackOutboxDead(ctx, deadIDs, "poison: payload undecodable"); err != nil {
			w.lg.Warn("知识库写回重试 dead 标记失败",
				loggateway.StepID("knowledge.writeback.sweep_dead"),
				loggateway.Err(err))
		}
	}
	if len(publishedIDs) > 0 {
		if err := w.outbox.MarkWriteBackOutboxPublished(ctx, publishedIDs, time.Now().UTC()); err != nil {
			w.lg.Warn("知识库写回重试 published 标记失败",
				loggateway.StepID("knowledge.writeback.sweep_publish"),
				loggateway.Err(err))
		}
	}
	w.lg.Info("知识库写回重试队列重放完成",
		loggateway.StepID("knowledge.writeback.sweep"),
		loggateway.Int("scanned", len(rows)),
		loggateway.Int("published", len(publishedIDs)),
		loggateway.Int("dead", len(deadIDs)))
	return len(publishedIDs), len(deadIDs)
}
