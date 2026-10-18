package main

import (
	"context"
	"time"

	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	"github.com/prometheus/client_golang/prometheus"
)

// statsReportInterval 是 logpipeline 自观测指标的上报周期。
const statsReportInterval = 15 * time.Second

// startLogPipelineStatsReporter 周期性读取 logpipeline 的累积计数并导出
// Prometheus 指标（2026-10-18 可观测性审计 P1-1：此前 Dropped/Throttled/
// SinkErrors/SinkPanics 只存在于进程内计数器，日志静默丢失完全不可见）。
//
// counter 采用 delta 累加：logpipeline 侧是进程生命周期单调累积值，
// Prometheus counter 只补增量，进程重启后从 0 重新累计（符合 counter 语义）。
func startLogPipelineStatsReporter(ctx context.Context, p logpipeline.Pipeline) {
	if p == nil {
		return
	}
	safego.Go(ctx, "logpipeline.stats_reporter", func() {
		ticker := time.NewTicker(statsReportInterval)
		defer ticker.Stop()
		var prev logpipeline.PipelineStats
		for {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
			s := p.Stats()
			addCounterDelta(metrics.LogPipelineDroppedTotal, prev.Dropped, s.Dropped)
			addCounterDelta(metrics.LogPipelineThrottledTotal, prev.Throttled, s.Throttled)
			addCounterDelta(metrics.LogPipelineSinkErrorsTotal, prev.SinkErrors, s.SinkErrors)
			addCounterDelta(metrics.LogPipelineSinkPanicsTotal, prev.SinkPanics, s.SinkPanics)
			if s.ChanCap > 0 {
				metrics.LogPipelineBufferUsage.Set(float64(s.ChanLen) / float64(s.ChanCap))
			}
			prev = s
		}
	})
}

func addCounterDelta(c prometheus.Counter, prev, cur uint64) {
	if cur > prev {
		c.Add(float64(cur - prev))
	}
}
