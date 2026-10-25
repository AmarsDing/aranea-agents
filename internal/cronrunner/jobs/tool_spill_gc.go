package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"
)

// ToolSpillGC 是 toolspill.Store.GC 的生产调度器（Q7；93-toolspill-gc-scheduler
// 三件套）。Store 的 TTL/MaxTotalBytes 上界在接线前是已配置未生效——GC 全仓
// 只有测试调用方；本 worker 是唯一生产调用方（65 卡 1.55 ⚠️5：周期清理调度
// 一律挂 cronrunner/jobs，模块内禁私挂 cron）。
//
// 零默认纪律的登记豁免：conf 默认已承诺 TTL=168h/总量=64MiB，接线=补上生效侧，
// 故默认 ON；TOOLSPILL_GC_DISABLED=1 为杀闸回退。首跑即对存量执行一次性 TTL
// 清洗，且总量超上限时 <TTL 数据按 oldest-first 裁汰（read_tool_result 消费
// 契约 found:false）——该风险与回退动作已随推送向用户复述登记。
//
// 事件分级（双平面语义，同 spill_failed 先例）：GC 失败单周期一封 EmitGate
// （decision outbox 异步落表）+ aranea_toolspill_gc_runs_total{status=error}；
// worker 低频天然聚合故不带 deduper。panic 周期按 status=panic 计 + Warn，
// 不重投 EmitGate（D2-f-1）。PG 侧跨副本互斥由 store 内建单事务
// advisory 锁兜底（C-5 实现、C-f 评审确认为有意设计）：败方锁等待=预期互斥，
// ctx 可取消且表量有界，不算 GC 失败事件。关停期 ctx 取消按 neutral 计
// （status=cancelled），不产生审计事件。
type ToolSpillGC struct {
	store    toolspill.Store
	kind     string
	interval time.Duration
	dc       decision.Collector
	lg       loggateway.Logger
	now      func() time.Time
}

// toolSpillGCMinInterval 是周期下限：防误配置把单周期一封的 Important 级事件
// 放大成 decision_records 审计噪音。
const toolSpillGCMinInterval = 5 * time.Minute

// defaultToolSpillGCInterval 解析 TOOLSPILL_GC_INTERVAL（Go duration）；
// 缺省 1h，env 档<5min 钳到下限。无前缀 env 对齐 jobs 包内惯例
// （FLOW_LOG_CLEANUP_DISABLED/AUTO_HEAL_TTL_INTERVAL 同例）。
func defaultToolSpillGCInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("TOOLSPILL_GC_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			if d < toolSpillGCMinInterval {
				return toolSpillGCMinInterval
			}
			return d
		}
	}
	return time.Hour
}

// NewToolSpillGC 构造 GC 调度器。interval<=0 走 env 默认；显式正数原样采用
// （测试可传小周期）。kind 仅作可观测标签（fs/pg）；lg 为 nil 回落 Noop。
func NewToolSpillGC(store toolspill.Store, kind string, interval time.Duration, dc decision.Collector, lg loggateway.Logger) *ToolSpillGC {
	if interval <= 0 {
		interval = defaultToolSpillGCInterval()
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ToolSpillGC{store: store, kind: kind, interval: interval, dc: dc, lg: lg, now: time.Now}
}

// Start 运行 GC 循环直至 ctx 取消。typed-nil 安全网：provider 在 spill OFF/
// 构造失败时返 nil 指针，装进 BackgroundStarter 接口后 workers.go 的 != nil
// 判定对 typed-nil 失效，本首行兜底（FlowLogCleanup:32-35 先例）。首行 select
// 之后的 runOnce 为同步执行：单周期跑完才进下一拍，天然无重叠、一周期一封。
func (w *ToolSpillGC) Start(ctx context.Context) {
	if w == nil || w.store == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *ToolSpillGC) runOnce(ctx context.Context) {
	start := w.now()
	defer func() {
		if r := recover(); r != nil {
			// D2-f-1（评审确认）：panic 周期必须落 runs_total{panic} + duration
			// ——否则持续走火的 store 在三平面（runs/duration/safego）全部不可见
			// （safego 外层因本 recover 抢先消费而永远接不到）。不重投 EmitGate：
			// panic 可能正源于 EmitGate/dc/metrics 内部，重入即次生 panic 杀死循环。
			metrics.ToolSpillGCDuration.WithLabelValues(w.kind).Observe(w.now().Sub(start).Seconds())
			metrics.ToolSpillGCRunsTotal.WithLabelValues(w.kind, "panic").Inc()
			w.lg.Warn("toolspill gc panic recovered",
				loggateway.StepID("toolspill.gc"),
				loggateway.Str("store_kind", w.kind),
				loggateway.Str("panic", fmt.Sprint(r)))
		}
	}()
	stats, err := w.store.GC(ctx, start)
	metrics.ToolSpillGCDuration.WithLabelValues(w.kind).Observe(w.now().Sub(start).Seconds())
	switch {
	case err != nil && ctx.Err() != nil:
		// 关停期取消：neutral，不计失败也不发审计事件（防 shutdown 噪音）。
		metrics.ToolSpillGCRunsTotal.WithLabelValues(w.kind, "cancelled").Inc()
	case err != nil:
		metrics.ToolSpillGCRunsTotal.WithLabelValues(w.kind, "error").Inc()
		w.lg.Warn("toolspill gc cycle failed",
			loggateway.StepID("system.gate.spill_gc_failed"),
			loggateway.Str("store_kind", w.kind),
			loggateway.Err(err))
		event.EmitGate(ctx, w.dc, decision.GateDecision{
			TriggerRule: decision.TriggerSpillGCFailed,
			Outcome:     "tripped",
			Scenario:    "工具结果溢写 GC 周期失败",
			Reasoning:   err.Error(),
			GuardName:   "tool_result_spill_gc",
			Action:      "degrade",
			Extra: map[string]any{
				"store_kind": w.kind,
				"scanned":    stats.Scanned,
				"removed":    stats.Removed,
			},
		})
	default:
		metrics.ToolSpillGCRunsTotal.WithLabelValues(w.kind, "ok").Inc()
		if stats.Removed > 0 {
			w.lg.Info("toolspill gc cycle reclaimed payloads",
				loggateway.StepID("toolspill.gc"),
				loggateway.Str("store_kind", w.kind),
				loggateway.Int("scanned", stats.Scanned),
				loggateway.Int("removed", stats.Removed),
				loggateway.Int64("reclaimed_bytes", stats.ReclaimedBytes))
		}
	}
}

// ToolSpillGCDisabled 报告 TOOLSPILL_GC_DISABLED 杀闸（1/true/yes；解析形态
// 逐字对齐 FlowLogCleanupDisabled）。
func ToolSpillGCDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("TOOLSPILL_GC_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
