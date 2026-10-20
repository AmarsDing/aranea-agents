package jobs

import (
	"context"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// KnowledgeGraphProjectionDefaultInterval 卡片投影周期（ADR-KN-20261020-H 写侧
// 决策：cron 5m；实体/边抽取本身由 EntityPipeline/relation extract 热度闸产生，
// 工人只投影不扩产，5 分钟级延迟可接受——即时钩子 v1 刻意不做，避免双写点）。
const KnowledgeGraphProjectionDefaultInterval = 5 * time.Minute

// KnowledgeGraphProjectionJob 图谱卡片对账投影 cron 壳：每轮调用
// biz.GraphProjectionWorker.RunOnce（缺卡/漂移重嵌、孤儿与闭合边墓碑，
// 全局预算 256 行/轮），统计落 Info 日志。
type KnowledgeGraphProjectionJob struct {
	interval time.Duration
	worker   *bizknowledge.GraphProjectionWorker
	lg       loggateway.Logger
}

func NewKnowledgeGraphProjectionJob(
	interval time.Duration,
	worker *bizknowledge.GraphProjectionWorker,
	lg loggateway.Logger,
) *KnowledgeGraphProjectionJob {
	if worker == nil {
		return nil
	}
	if interval <= 0 {
		interval = KnowledgeGraphProjectionDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &KnowledgeGraphProjectionJob{
		interval: interval,
		worker:   worker,
		lg:       lg.With(loggateway.Domain("knowledge_graph_projection")),
	}
}

func (j *KnowledgeGraphProjectionJob) Start(ctx context.Context) {
	if j == nil {
		return
	}
	j.lg.Info("knowledge graph projection worker started",
		loggateway.Str("interval", j.interval.String()))
	j.RunOnce(ctx) // 启动即对账一轮（与 sweeper 同形制——启动修卡不 delay 5m）
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			j.lg.Info("knowledge graph projection worker stopped")
			return
		case <-ticker.C:
			j.RunOnce(ctx)
		}
	}
}

// RunOnce 单轮。
func (j *KnowledgeGraphProjectionJob) RunOnce(ctx context.Context) {
	if j == nil {
		return
	}
	stats, err := j.worker.RunOnce(ctx)
	if err != nil {
		j.lg.Warn("图谱卡片投影轮失败",
			loggateway.StepID("knowledge.graph_projection.run"),
			loggateway.Err(err))
		return
	}
	if stats.Upserted > 0 || stats.Deleted > 0 || stats.Failed > 0 {
		j.lg.Info("图谱卡片投影轮完成",
			loggateway.StepID("knowledge.graph_projection.run"),
			loggateway.Int("collections", stats.Collections),
			loggateway.Int("upserted", stats.Upserted),
			loggateway.Int("deleted", stats.Deleted),
			loggateway.Int("failed", stats.Failed))
	}
}
