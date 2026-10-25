package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/toolspill"
	"aranea-agents/pkg/loggateway"
)

// tool_spill_failed_emit.go —— P0-2 toolspill 批 C：spill_failed 的 Important
// 级（AS-EVT-01）EmitGate 双写。批 B 四消费点接线时留了 Hooks.OnSpillFailed
// 接缝（评审 §12.2 P0-2⑤ 批准项），本批把接缝接到三方证据面：
//   1. decision EmitGate → decision_records（异步 outbox，Important=BlockUpTo
//      之外的观测持久化面）+ flow_log system.gate.spill_failed 步骤；
//   2. SystemNoticeEvent（best-effort，ctx 有 ActivityEmitter 时）——失败对
//      用户可见：输出已被 partial 豁免在上下文保全，但外置副本未落盘；
//   3. 会话级 5min 去重——键形 spill_failed:<session>:<floor(ts,300)> 由
//      toolspill.SpillFailedDedupeKey 预切桶，同桶内重复失败只记第一笔，
//      防溢出目录不可写场景每个工具结果刷屏 notice/决策记录。
// collector 为 nil（CLI/未接管部署）时 EmitGate 内建 LogNilCollector 告警 +
// Warn 日志仍写——绝不静默（D1/0827 skip 先例）。
//
//洗涤边界：本钩子只做观测发射，不参与 spill 读写主链——发射自身出错一律
// Warn 级吞掉（观测链失败不炸工具调用）。

// spillFailedDeduper 记住已发射的 5min 桶键；带惰性清扫防长跑进程内存
// 线性增长（>512 键时丢弃早于两桶前的键）。
type spillFailedDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// seenOnce 报告键是否首次出现并记录；非首次返回 false（跳过发射）。
// 懒清扫：键数越界时全量重建保留当前桶与前一桶（键尾带 5min 桶序号）。
func (d *spillFailedDeduper) seenOnce(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, dup := d.seen[key]; dup {
		return false
	}
	if len(d.seen) > 512 {
		// 5min 桶序号在键尾（spill_failed:<session>:<bucket>）；只保留最近
		// 两桶——其余键即使重复到达，旧桶时间已过，重发属合理再观测。
		cur := time.Now().Unix() / 300
		fresh := make(map[string]struct{}, 64)
		for k := range d.seen {
			var bucket int64
			for i := len(k) - 1; i >= 0; i-- {
				if k[i] == ':' {
					if n, err := parseInt64(k[i+1:]); err == nil {
						bucket = n
					}
					break
				}
			}
			if bucket >= cur-1 {
				fresh[k] = struct{}{}
			}
		}
		d.seen = fresh
	}
	d.seen[key] = struct{}{}
	return true
}

// parseInt64 是无 fmt 开销的十进制解析（键尾桶序号，非法输入报错由调用方兜底）。
func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errInvalidBucket
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidBucket
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var errInvalidBucket = errorString("invalid spill dedupe bucket")

type errorString string

func (e errorString) Error() string { return string(e) }

// NewSpillFailedEmitHooks 构造 toolspill.Hooks 的生产发射闭包（wire 注入点：
// provideToolSpillDeps）。lg 为 nil 时回落 Noop。返回的 Hooks 仅填
// OnSpillFailed——其余接缝位保留未来扩展（评审批准仅本枚）。
func NewSpillFailedEmitHooks(c decision.Collector, lg loggateway.Logger) toolspill.Hooks {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	dedup := &spillFailedDeduper{seen: make(map[string]struct{})}
	return toolspill.Hooks{
		OnSpillFailed: func(ctx context.Context, dedupeKey string, cause error) {
			if dedupeKey == "" {
				dedupeKey = toolspill.SpillFailedDedupeKey("unknown", time.Now())
			}
			if !dedup.seenOnce(dedupeKey) {
				return // 同 5min 桶第二次起静默（观测已落，防刷屏）
			}
			sessionID := gateSessionID(ctx)
			lg.Warn("toolspill persist failed — 输出已在上下文保全但未落盘外置副本",
				loggateway.StepID("system.gate.spill_failed"),
				loggateway.Str("dedupe_key", dedupeKey),
				loggateway.SessionID(sessionID),
				loggateway.Err(cause))
			reasoning := "spill persistence failed"
			if cause != nil {
				reasoning = cause.Error()
			}
			event.EmitGate(ctx, c, decision.GateDecision{
				TriggerRule: decision.TriggerSpillFailed,
				Outcome:     "tripped",
				Scenario:    "工具结果溢写持久化失败（partial 豁免保全原文）",
				Reasoning:   reasoning,
				GuardName:   "tool_result_spill",
				RunID:       gateRunID(ctx),
				SessionID:   sessionID,
				Action:      "degrade",
				Extra:       map[string]any{"dedupe_key": dedupeKey, "preserved": true},
			})
			if emitter := biz.ActivityEmitterFromContext(ctx); emitter != nil {
				if nerr := emitter.EmitNotice(ctx, "工具结果外置存储失败：本次输出已在对话中完整保留，未截断丢失", decision.TriggerSpillFailed); nerr != nil {
					lg.Warn("spill_failed notice 发射失败", loggateway.Err(nerr))
				}
			}
		},
	}
}
