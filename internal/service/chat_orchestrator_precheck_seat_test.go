package service

// D4-b seat①（admitTurn durable 压缩预检）行为级钉板（D4-f 评审
// wf_0699f3d1 #10 确认项——双坐席零行为级钉回本批证据通道）：
//   - off：全静默（判级不跑、观测步零发射、不死读窄口）；
//   - observe（缺省）：恰一条 session.compress.precheck 观测步，字段面齐
//     （used/soft/hard/verdict/run_id join 键），afterNativeTurn 零触发；
//   - on：判级越软带才按正常触发语义调 AfterNativeTurn 恰一次，below_soft 不触发；
//   - Compress 未实现 biz.DurableTurnPrecheck 窄口：安全跳过、零观测步。

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

	"go.uber.org/zap/zapcore"
)

// precheckSeatCaptureLogger 捕获 Info 级字段面（step_id 归因断言用）。
type precheckSeatCaptureLogger struct {
	entries []map[string]any
}

func (l *precheckSeatCaptureLogger) Debug(string, ...loggateway.Field) {}
func (l *precheckSeatCaptureLogger) Info(_ string, fields ...loggateway.Field) {
	m := map[string]any{}
	for _, f := range fields {
		switch f.Type {
		case zapcore.StringType:
			m[f.Key] = f.String
		case zapcore.BoolType:
			m[f.Key] = f.Integer == 1
		default:
			m[f.Key] = f.Integer
		}
	}
	l.entries = append(l.entries, m)
}
func (l *precheckSeatCaptureLogger) Warn(string, ...loggateway.Field)  {}
func (l *precheckSeatCaptureLogger) Error(string, ...loggateway.Field) {}
func (l *precheckSeatCaptureLogger) With(...loggateway.Field) loggateway.Logger {
	return l
}

func (l *precheckSeatCaptureLogger) byStepID(stepID string) []map[string]any {
	var out []map[string]any
	for _, e := range l.entries {
		if s, _ := e["step_id"].(string); s == stepID {
			out = append(out, e)
		}
	}
	return out
}

// fakeDurablePrecheck 实现 biz.NativeTurnCompressor + biz.DurableTurnPrecheck
// 双口（压缩机形态），记录两坐席调用次数。
type fakeDurablePrecheck struct {
	report   biz.DurablePrecheckReport
	verdicts int
	after    int
}

func (f *fakeDurablePrecheck) PrecheckVerdict(sess biz.Session, _ biz.Agent) biz.DurablePrecheckReport {
	f.verdicts++
	rep := f.report
	rep.UsedTokens = sess.ContextUsedTokens
	return rep
}

func (f *fakeDurablePrecheck) AfterNativeTurn(_ context.Context, _ string, _ biz.Agent) { f.after++ }

// fakeNativeOnlyCompressor 只实现 biz.NativeTurnCompressor（未实现窄口）。
type fakeNativeOnlyCompressor struct{ after int }

func (f *fakeNativeOnlyCompressor) AfterNativeTurn(_ context.Context, _ string, _ biz.Agent) {
	f.after++
}

func newPrecheckSeatPhases(lg loggateway.Logger, comp biz.NativeTurnCompressor) *turnPhases {
	o := &ChatOrchestrator{}
	o.core.TD = rt.TurnDeps{Compress: comp}
	o.infraDeps.LG = lg
	return &turnPhases{ChatOrchestrator: o}
}

func TestAdmitTurnPrecheck_OffSilent(t *testing.T) {
	t.Setenv("ARANEA_DURABLE_PRECHECK", "off")
	fake := &fakeDurablePrecheck{report: biz.DurablePrecheckReport{Verdict: biz.DurablePrecheckHardBand, SoftTokens: 100, HardTokens: 200}}
	lg := &precheckSeatCaptureLogger{}
	_, err := newPrecheckSeatPhases(lg, fake).admitTurn(context.Background(),
		biz.Session{ID: "s1", ContextUsedTokens: 99999}, biz.TurnInput{SessionID: "s1"}, biz.Agent{}, "", "", "")
	if err != nil {
		t.Fatalf("admitTurn err = %v", err)
	}
	if fake.verdicts != 0 || fake.after != 0 {
		t.Fatalf("off mode must not touch compressor: verdicts=%d after=%d", fake.verdicts, fake.after)
	}
	if got := len(lg.byStepID("session.compress.precheck")); got != 0 {
		t.Fatalf("off mode must emit zero precheck steps, got %d", got)
	}
}

func TestAdmitTurnPrecheck_ObserveEmitsOneStep(t *testing.T) {
	t.Setenv("ARANEA_DURABLE_PRECHECK", "") // 缺省回落 observe（并覆盖空值回落面）
	fake := &fakeDurablePrecheck{report: biz.DurablePrecheckReport{
		Verdict: biz.DurablePrecheckBelowSoft, SoftTokens: 100, HardTokens: 200, ContextWindow: 256000,
	}}
	lg := &precheckSeatCaptureLogger{}
	res, err := newPrecheckSeatPhases(lg, fake).admitTurn(context.Background(),
		biz.Session{ID: "s1", ContextUsedTokens: 50}, biz.TurnInput{SessionID: "s1"}, biz.Agent{}, "", "", "")
	if err != nil {
		t.Fatalf("admitTurn err = %v", err)
	}
	if fake.verdicts != 1 || fake.after != 0 {
		t.Fatalf("observe exactly one verdict, zero trigger: verdicts=%d after=%d", fake.verdicts, fake.after)
	}
	entries := lg.byStepID("session.compress.precheck")
	if len(entries) != 1 {
		t.Fatalf("observe must emit exactly one precheck step, got %d", len(entries))
	}
	md := entries[0]
	if md["used_tokens"] != int64(50) || md["soft_tokens"] != int64(100) || md["hard_tokens"] != int64(200) {
		t.Fatalf("threshold fields mismatch: %+v", md)
	}
	if md["verdict"] != "below_soft" || md["mode"] != "observe" {
		t.Fatalf("verdict/mode mismatch: %+v", md)
	}
	if md["run_id"] != res.runID || md["run_id"] == "" {
		t.Fatalf("run_id join key mismatch: md=%v res.runID=%q", md["run_id"], res.runID)
	}
	if md["durable_turn"] != false {
		t.Fatalf("native turn must log durable_turn=false, got %v", md["durable_turn"])
	}
}

func TestAdmitTurnPrecheck_OnTriggersAfterNativeTurn(t *testing.T) {
	t.Setenv("ARANEA_DURABLE_PRECHECK", "on")
	for _, tc := range []struct {
		name      string
		verdict   biz.DurablePrecheckVerdict
		wantAfter int
	}{
		{"soft_band_triggers", biz.DurablePrecheckSoftBand, 1},
		{"hard_band_triggers", biz.DurablePrecheckHardBand, 1},
		{"below_soft_silent", biz.DurablePrecheckBelowSoft, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDurablePrecheck{report: biz.DurablePrecheckReport{Verdict: tc.verdict, SoftTokens: 100, HardTokens: 200}}
			lg := &precheckSeatCaptureLogger{}
			_, err := newPrecheckSeatPhases(lg, fake).admitTurn(context.Background(),
				biz.Session{ID: "s1", ContextUsedTokens: 50}, biz.TurnInput{SessionID: "s1"}, biz.Agent{}, "", "", "")
			if err != nil {
				t.Fatalf("admitTurn err = %v", err)
			}
			if fake.verdicts != 1 || fake.after != tc.wantAfter {
				t.Fatalf("verdicts=%d after=%d, want (1, %d)", fake.verdicts, fake.after, tc.wantAfter)
			}
			if got := len(lg.byStepID("session.compress.precheck")); got != 1 {
				t.Fatalf("on mode still logs the observation step, got %d", got)
			}
		})
	}
}

func TestAdmitTurnPrecheck_NarrowPortMissing_SkipsSafely(t *testing.T) {
	t.Setenv("ARANEA_DURABLE_PRECHECK", "observe")
	fake := &fakeNativeOnlyCompressor{}
	lg := &precheckSeatCaptureLogger{}
	_, err := newPrecheckSeatPhases(lg, fake).admitTurn(context.Background(),
		biz.Session{ID: "s1", ContextUsedTokens: 50}, biz.TurnInput{SessionID: "s1"}, biz.Agent{}, "", "", "")
	if err != nil {
		t.Fatalf("admitTurn err = %v", err)
	}
	if fake.after != 0 {
		t.Fatalf("observe must not call AfterNativeTurn via the static port, after=%d", fake.after)
	}
	if got := len(lg.byStepID("session.compress.precheck")); got != 0 {
		t.Fatalf("unimplemented narrow port must skip silently, got %d steps", got)
	}
}
