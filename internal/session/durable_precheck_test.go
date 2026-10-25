package session

// D4-b durable 压缩预检钉板（就绪不翻）：PrecheckVerdict 与
// compressTriggerThresholds 逐点对齐（判级口径=runCompress 触线口径）、
// nil receiver 安全回落 below_soft、duration_ms 埋点落 decision metadata。

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/llmcontext"
)

// 编译期 pin：Compressor 兑现 biz.DurableTurnPrecheck 窄口（seat① 断言面）。
var _ biz.DurableTurnPrecheck = (*Compressor)(nil)

// TestPrecheckVerdict_AlignedWithThresholds 钉判级与触线口径逐点对齐：
// 静态 buffer 支路（显式关 adaptive——adaptive 读内存会话态、连续调用有
// 序参依赖，非纯函数）下阈值为 (ag, window) 纯函数，软/硬阈一次自校准即
// 为全局真值；verdict 落带语义与 runCompress 触线分支一致（hard 优先）。
func TestPrecheckVerdict_AlignedWithThresholds(t *testing.T) {
	c := NewCompressor(CompressorConfig{})
	defer c.Close()
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{}} // CompressionBufferAdaptive=false → 静态支路
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})

	probe := biz.Session{ID: "s1"}
	soft, hard := c.compressTriggerThresholds("s1", probe, ag, 0, window)
	if soft <= 0 || hard <= 0 {
		t.Skipf("non-positive default thresholds soft=%d hard=%d; band matrix vacuous", soft, hard)
	}

	usedSet := []int{0, soft / 2, soft, (soft + hard) / 2, hard, hard + 100, hard * 2}
	for _, used := range usedSet {
		sess := biz.Session{ID: "s1", ContextUsedTokens: used}
		rep := c.PrecheckVerdict(sess, ag)

		var want biz.DurablePrecheckVerdict
		switch {
		case used >= hard:
			want = biz.DurablePrecheckHardBand
		case used >= soft:
			want = biz.DurablePrecheckSoftBand
		default:
			want = biz.DurablePrecheckBelowSoft
		}
		if rep.Verdict != want {
			t.Fatalf("used=%d (soft=%d hard=%d): verdict=%q, want %q", used, soft, hard, rep.Verdict, want)
		}
		if rep.UsedTokens != used || rep.SoftTokens != soft || rep.HardTokens != hard || rep.ContextWindow != window {
			t.Fatalf("used=%d: report fields mismatch: %+v (want used=%d soft=%d hard=%d window=%d)",
				used, rep, used, soft, hard, window)
		}
		if got := rep.AtOrAboveSoft(); got != (want != biz.DurablePrecheckBelowSoft) {
			t.Fatalf("used=%d: AtOrAboveSoft=%v inconsistent with verdict=%q", used, got, want)
		}
	}
}

// TestPrecheckVerdict_NilReceiverFallsThrough 钉 nil-receiver 安全回落：
// 不 panic、below_soft、透传 used，零阈值面（seat① 据此自然不动
// compressor——observe 语义下 nil 亦只观测）。
func TestPrecheckVerdict_NilReceiverFallsThrough(t *testing.T) {
	var nilC *Compressor
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
	sess := biz.Session{ID: "s1", ContextUsedTokens: 999}
	rep := nilC.PrecheckVerdict(sess, biz.Agent{})
	if rep.Verdict != biz.DurablePrecheckBelowSoft {
		t.Fatalf("nil receiver verdict=%q, want below_soft", rep.Verdict)
	}
	if rep.UsedTokens != 999 {
		t.Fatalf("nil receiver must pass through used tokens, got %d", rep.UsedTokens)
	}
	if rep.SoftTokens != 0 || rep.HardTokens != 0 {
		t.Fatalf("nil receiver must report zero thresholds, got soft=%d hard=%d", rep.SoftTokens, rep.HardTokens)
	}
	if rep.ContextWindow != window {
		t.Fatalf("nil receiver window=%d, want resolved default %d", rep.ContextWindow, window)
	}
	if rep.AtOrAboveSoft() {
		t.Fatal("nil receiver must never trigger the on-mode seat")
	}
}

// TestPrecheckVerdict_AdaptiveStateUntouched 钉 D4-f 修复（对抗评审
// wf_0699f3d1 #1/#2/#5/#7/#8 确认根因——adaptive 臂 getAdaptiveBufferRatio
// 是写函数且对 AdaptiveBufferState 无锁读写，只允许 runCompress 飞行串行
// 区访问）：PrecheckVerdict 走静态阈值臂，adaptive 开启（缺省）时既不播种
// 也不推进 AdaptiveBufferState，谓词级纯函数。
func TestPrecheckVerdict_AdaptiveStateUntouched(t *testing.T) {
	c := NewCompressor(CompressorConfig{})
	defer c.Close()
	ag := biz.Agent{} // Settings==nil → DefaultCompressPolicy AdaptiveBufferEnabled=true（生产缺省）

	// A. 未播种态：多次判级（零增量读数，adaptive 旧实现必落低增量臂）不得
	// 建任何内存态入口。
	sess := biz.Session{ID: "s1", ContextUsedTokens: 50000}
	for i := 0; i < 6; i++ {
		_ = c.PrecheckVerdict(sess, ag)
	}
	if _, ok := c.buf.buffer.Load("s1"); ok {
		t.Fatal("precheck must not seed adaptive buffer state (adaptive write path belongs to runCompress alone)")
	}

	// B. 已播种态（由权威路径播种+推进一次）：连续判级不得推进序参。
	window := llmcontext.ResolveWindow(llmcontext.ResolveInput{})
	_ = c.buf.getAdaptiveBufferRatio("s1", ag, 50000, window, 0, 0) // 播种
	_ = c.buf.getAdaptiveBufferRatio("s1", ag, 53000, window, 0, 0) // 推进一次
	v, ok := c.buf.buffer.Load("s1")
	if !ok {
		t.Fatal("seeded adaptive state must exist")
	}
	sb := v.(*AdaptiveBufferState)
	ratioB, lowB, usedB := sb.CurrentRatio, sb.ConsecutiveLowCount, sb.LastUsedTokens
	for i := 0; i < 6; i++ {
		_ = c.PrecheckVerdict(sess, ag) // used=50000 < 已播 53000：旧实现必落低增量臂推进
	}
	sa := v.(*AdaptiveBufferState)
	if sa.CurrentRatio != ratioB || sa.ConsecutiveLowCount != lowB || sa.LastUsedTokens != usedB {
		t.Fatalf("precheck must not advance adaptive state: before (%v,%d,%d) after (%v,%d,%d)",
			ratioB, lowB, usedB, sa.CurrentRatio, sa.ConsecutiveLowCount, sa.LastUsedTokens)
	}
	// 静态臂仍给三带判级（adaptive 开时阈值以策略 bufferRatio 近似）——
	// 数值与静态公式逐点相等，不依赖 adaptive 态。
	hard := hardTriggerTokens(ag, window)
	sessHard := biz.Session{ID: "s1", ContextUsedTokens: hard + 100}
	if rep := c.PrecheckVerdict(sessHard, ag); rep.Verdict != biz.DurablePrecheckHardBand {
		t.Fatalf("adaptive-on static approximation: used=%d verdict=%q, want hard_band", hard+100, rep.Verdict)
	}
}

// fakeDecisionCollector 是 decision.Collector 的内存实现（Emit 单方法口）。
type fakeDecisionCollector struct {
	recs []decision.Record
}

func (f *fakeDecisionCollector) Emit(_ context.Context, r decision.Record) {
	f.recs = append(f.recs, r)
}

// TestEmitCompressDecision_DurationMS 钉 D4-b 同批 duration_ms 埋点：
// extra["duration_ms"] 原样落 Record.Metadata（EmitGate 的 Extra→Metadata
// 映射不保留同名字段覆盖），collector nil 静默降级不 panic。
func TestEmitCompressDecision_DurationMS(t *testing.T) {
	fc := &fakeDecisionCollector{}
	c := &Compressor{collector: fc}

	c.emitCompressDecision("s1", "truncated", 1000, 900, 10, 300, compressOutcome{}, false, 12345, "r", "compress")
	if len(fc.recs) != 1 {
		t.Fatalf("expected 1 decision record, got %d", len(fc.recs))
	}
	if got, want := fc.recs[0].Metadata["duration_ms"], int64(12345); got != want {
		t.Fatalf("metadata[duration_ms]=%v (%T), want %v", got, got, want)
	}
	if got := fc.recs[0].Metadata["messages_before"]; got != 10 {
		t.Fatalf("metadata[messages_before]=%v, want 10", got)
	}
	if got := fc.recs[0].Metadata["forced"]; got != false {
		t.Fatalf("metadata[forced]=%v, want false", got)
	}

	// collector 未装配：静默降级。
	(&Compressor{}).emitCompressDecision("s1", "x", 1, 1, 1, 0, compressOutcome{}, false, 1, "r", "compress")
}
