package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── mock ports ───────────────────────────────────────────────────────────────

type mockErrorClusterReader struct {
	clusters  []ErrorCluster
	err       error
	gotSince  time.Time
	gotMin    int
	callCount int
}

func (m *mockErrorClusterReader) ListErrorClusters(_ context.Context, since time.Time, minCount int) ([]ErrorCluster, error) {
	m.callCount++
	m.gotSince = since
	m.gotMin = minCount
	return m.clusters, m.err
}

type mockPerfMetricsReader struct {
	latency    []StepLatencyStat
	tokens     []TokenAnomaly
	latencyErr error
	tokenErr   error
	gotBaseWin string
	gotCurWin  string
}

func (m *mockPerfMetricsReader) GetStepLatencyStats(_ context.Context, baselineWindow, currentWindow string) ([]StepLatencyStat, error) {
	m.gotBaseWin, m.gotCurWin = baselineWindow, currentWindow
	return m.latency, m.latencyErr
}

func (m *mockPerfMetricsReader) GetTokenUsageStats(_ context.Context, _, _ string) ([]TokenAnomaly, error) {
	return m.tokens, m.tokenErr
}

type mockEvalBaselineReader struct {
	latest   *EvalBaseline
	previous *EvalBaseline
	err      error
}

func (m *mockEvalBaselineReader) GetLatestBaseline(_ context.Context) (*EvalBaseline, error) {
	return m.latest, m.err
}

func (m *mockEvalBaselineReader) GetPreviousBaseline(_ context.Context) (*EvalBaseline, error) {
	return m.previous, m.err
}

type mockTestRunReader struct {
	failures []TestFailure
	err      error
	gotRound int
}

func (m *mockTestRunReader) ListRecentFailures(_ context.Context, minConsecutiveRounds int) ([]TestFailure, error) {
	m.gotRound = minConsecutiveRounds
	return m.failures, m.err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func assertPlatformSuggestion(t *testing.T, s UnifiedEvolutionSuggestion, wantSource string, wantAction EvolutionActionType) string {
	t.Helper()
	if s.ID == "" {
		t.Error("suggestion ID empty")
	}
	if s.TargetType != EvolutionTargetPlatform {
		t.Errorf("TargetType = %q, want platform", s.TargetType)
	}
	if s.TriggerSource != wantSource {
		t.Errorf("TriggerSource = %q, want %q", s.TriggerSource, wantSource)
	}
	if s.ActionType != wantAction {
		t.Errorf("ActionType = %q, want %q", s.ActionType, wantAction)
	}
	if s.Status != string(UnifiedEvolutionStatePending) {
		t.Errorf("Status = %q, want pending", s.Status)
	}
	if s.TriggerReason == "" {
		t.Error("TriggerReason empty")
	}
	// metadata must carry trigger_signature + pattern_hash mirror (DB dedup index).
	var meta map[string]any
	if err := json.Unmarshal(s.Metadata, &meta); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	sig, _ := meta[EvoMetaTriggerSignature].(string)
	if sig == "" {
		t.Fatal("metadata.trigger_signature missing")
	}
	if ph, _ := meta[EvoMetaPatternHash].(string); ph != sig {
		t.Errorf("metadata.pattern_hash = %q, want mirror of signature %q", ph, sig)
	}
	// TargetID = "<trigger_source>/<signature>" so per-signature pending dedup
	// and cooldown work through the generic orchestrator path.
	wantTarget := wantSource + "/" + sig
	if s.TargetID != wantTarget {
		t.Errorf("TargetID = %q, want %q", s.TargetID, wantTarget)
	}
	return sig
}

// ── ErrorClusterTrigger ──────────────────────────────────────────────────────

func TestErrorClusterTrigger_Interface(t *testing.T) {
	tr := NewErrorClusterTrigger(nil, 0, 0, loggateway.NewNoop())
	if tr.TargetType() != EvolutionTargetPlatform {
		t.Errorf("TargetType = %q", tr.TargetType())
	}
	if tr.ActionType() != EvolutionActionPatchCode {
		t.Errorf("ActionType = %q", tr.ActionType())
	}
	if tr.TriggerSource() != TriggerSourceErrorCluster {
		t.Errorf("TriggerSource = %q", tr.TriggerSource())
	}
}

func TestErrorClusterTrigger_NilReader(t *testing.T) {
	tr := NewErrorClusterTrigger(nil, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil || got != nil {
		t.Errorf("nil reader: got=%v err=%v, want nil/nil", got, err)
	}
}

func TestErrorClusterTrigger_WindowAndMinCount(t *testing.T) {
	m := &mockErrorClusterReader{}
	tr := NewErrorClusterTrigger(m, 7, 5, loggateway.NewNoop())
	before := time.Now().UTC()
	if _, err := tr.Check(context.Background(), "platform"); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC()
	if m.gotMin != 5 {
		t.Errorf("minCount = %d, want 5", m.gotMin)
	}
	// since ≈ now - 7d
	lo := before.Add(-7*24*time.Hour - time.Minute)
	hi := after.Add(-7*24*time.Hour + time.Minute)
	if m.gotSince.Before(lo) || m.gotSince.After(hi) {
		t.Errorf("since = %v, want within [%v, %v]", m.gotSince, lo, hi)
	}
}

func TestErrorClusterTrigger_Suggestion(t *testing.T) {
	m := &mockErrorClusterReader{clusters: []ErrorCluster{
		{ErrorCode: "nil_pointer_dereference", PatternHash: "sha256:abc", Count: 5, Component: "internal/biz/chat", SampleMessage: "nil deref", LastSeen: time.Now().UTC()},
		{ErrorCode: "rate_limit", PatternHash: "sha256:def", Count: 12, Component: "internal/agent", SampleMessage: "429", LastSeen: time.Now().UTC()},
	}}
	tr := NewErrorClusterTrigger(m, 7, 5, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	assertPlatformSuggestion(t, got[0], TriggerSourceErrorCluster, EvolutionActionPatchCode)
	// count=5 == minCount → priority 1; count=12 >= 2*minCount → priority 2.
	if got[0].Priority != 1 {
		t.Errorf("got[0].Priority = %d, want 1", got[0].Priority)
	}
	if got[1].Priority != 2 {
		t.Errorf("got[1].Priority = %d, want 2 (>=2x minCount)", got[1].Priority)
	}
	// signature must be stable per pattern hash: re-check produces identical TargetID.
	got2, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if got2[0].TargetID != got[0].TargetID {
		t.Errorf("unstable TargetID: %q vs %q", got2[0].TargetID, got[0].TargetID)
	}
}

func TestErrorClusterTrigger_ReaderError(t *testing.T) {
	m := &mockErrorClusterReader{err: errors.New("db down")}
	tr := NewErrorClusterTrigger(m, 7, 5, loggateway.NewNoop())
	if _, err := tr.Check(context.Background(), "platform"); err == nil {
		t.Error("want error propagation")
	}
}

func TestErrorClusterTrigger_FallbackSignatureWithoutHash(t *testing.T) {
	// Reader did not supply a PatternHash: signature derived from error code.
	m := &mockErrorClusterReader{clusters: []ErrorCluster{
		{ErrorCode: "connection_refused", Count: 6, SampleMessage: "dial tcp"},
	}}
	tr := NewErrorClusterTrigger(m, 7, 5, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	assertPlatformSuggestion(t, got[0], TriggerSourceErrorCluster, EvolutionActionPatchCode)
}

// ── PerfBottleneckTrigger ────────────────────────────────────────────────────

func TestPerfBottleneckTrigger_LatencyBoundaries(t *testing.T) {
	cases := []struct {
		name           string
		baseline, cur  float64
		wantSuggestion bool
	}{
		{"below factor", 100, 199, false},
		{"exactly at factor", 100, 200, true},
		{"above factor", 100, 350, true},
		{"zero baseline", 0, 500, false},
		{"zero current", 100, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockPerfMetricsReader{latency: []StepLatencyStat{
				{StepID: "llm.call", BaselineP95MS: tc.baseline, CurrentP95MS: tc.cur, SampleCount: 50},
			}}
			tr := NewPerfBottleneckTrigger(m, 2.0, 1.5, loggateway.NewNoop())
			got, err := tr.Check(context.Background(), "platform")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantSuggestion {
				if len(got) != 1 {
					t.Fatalf("len = %d, want 1", len(got))
				}
				assertPlatformSuggestion(t, got[0], TriggerSourcePerfBottleneck, EvolutionActionPatchCode)
			} else if len(got) != 0 {
				t.Fatalf("len = %d, want 0", len(got))
			}
		})
	}
}

func TestPerfBottleneckTrigger_TokenAnomaly(t *testing.T) {
	m := &mockPerfMetricsReader{tokens: []TokenAnomaly{
		{Scope: "agent:coder", BaselineTokens: 1000, CurrentTokens: 1600, SampleCount: 30},
		{Scope: "agent:chat", BaselineTokens: 1000, CurrentTokens: 1400, SampleCount: 30}, // 1.4x < 1.5x
	}}
	tr := NewPerfBottleneckTrigger(m, 2.0, 1.5, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (only 1.6x fires)", len(got))
	}
	assertPlatformSuggestion(t, got[0], TriggerSourcePerfBottleneck, EvolutionActionTuneConfig)
}

func TestPerfBottleneckTrigger_MixedSignals(t *testing.T) {
	m := &mockPerfMetricsReader{
		latency: []StepLatencyStat{{StepID: "tool.exec", BaselineP95MS: 100, CurrentP95MS: 300, SampleCount: 20}},
		tokens:  []TokenAnomaly{{Scope: "agent:coder", BaselineTokens: 100, CurrentTokens: 200, SampleCount: 20}},
	}
	tr := NewPerfBottleneckTrigger(m, 2.0, 1.5, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (latency + token)", len(got))
	}
	// Distinct signatures → distinct TargetIDs.
	if got[0].TargetID == got[1].TargetID {
		t.Error("latency and token suggestions must have distinct TargetIDs")
	}
}

func TestPerfBottleneckTrigger_NilReader(t *testing.T) {
	tr := NewPerfBottleneckTrigger(nil, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil || got != nil {
		t.Errorf("nil reader: got=%v err=%v", got, err)
	}
}

// ── EvalRegressionTrigger ────────────────────────────────────────────────────

func TestEvalRegressionTrigger_Boundaries(t *testing.T) {
	now := time.Now().UTC()
	mk := func(score float64) *EvalBaseline {
		return &EvalBaseline{RunID: "r", DatasetID: "ds1", AgentID: "a1", Score: score, CreatedAt: now}
	}
	cases := []struct {
		name             string
		previous, latest float64
		wantFire         bool
	}{
		{"drop 20%", 0.80, 0.64, true},
		{"drop just over 10%", 0.80, 0.719, true},
		{"drop exactly 10%", 0.80, 0.72, false}, // 设计：退化 >10% 才触发
		{"drop 5%", 0.80, 0.76, false},
		{"improved", 0.80, 0.85, false},
		{"zero previous", 0, 0.5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockEvalBaselineReader{latest: mk(tc.latest), previous: mk(tc.previous)}
			tr := NewEvalRegressionTrigger(m, 0.10, loggateway.NewNoop())
			got, err := tr.Check(context.Background(), "platform")
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantFire {
				if len(got) != 1 {
					t.Fatalf("len = %d, want 1", len(got))
				}
				sig := assertPlatformSuggestion(t, got[0], TriggerSourceEvalRegression, EvolutionActionPatchCode)
				if got[0].Priority != 2 {
					t.Errorf("Priority = %d, want 2 (quality regression)", got[0].Priority)
				}
				_ = sig
			} else if len(got) != 0 {
				t.Fatalf("len = %d, want 0", len(got))
			}
		})
	}
}

func TestEvalRegressionTrigger_NilBaselines(t *testing.T) {
	tr := NewEvalRegressionTrigger(&mockEvalBaselineReader{}, 0.10, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil || got != nil {
		t.Errorf("nil baselines: got=%v err=%v", got, err)
	}
	tr2 := NewEvalRegressionTrigger(nil, 0.10, loggateway.NewNoop())
	got2, err2 := tr2.Check(context.Background(), "platform")
	if err2 != nil || got2 != nil {
		t.Errorf("nil reader: got=%v err=%v", got2, err2)
	}
}

func TestEvalRegressionTrigger_SignatureStable(t *testing.T) {
	now := time.Now().UTC()
	m := &mockEvalBaselineReader{
		latest:   &EvalBaseline{RunID: "r2", DatasetID: "ds1", AgentID: "a1", Score: 0.6, CreatedAt: now},
		previous: &EvalBaseline{RunID: "r1", DatasetID: "ds1", AgentID: "a1", Score: 0.8, CreatedAt: now.Add(-time.Hour)},
	}
	tr := NewEvalRegressionTrigger(m, 0.10, loggateway.NewNoop())
	got1, _ := tr.Check(context.Background(), "platform")
	// Different run IDs, same dataset+agent → same signature (regression on the same baseline pair).
	m.latest.RunID = "r3"
	got2, _ := tr.Check(context.Background(), "platform")
	if len(got1) != 1 || len(got2) != 1 {
		t.Fatalf("got1=%d got2=%d", len(got1), len(got2))
	}
	if got1[0].TargetID != got2[0].TargetID {
		t.Errorf("signature must key on dataset+agent, not run ID: %q vs %q", got1[0].TargetID, got2[0].TargetID)
	}
}

// ── TestFailureTrigger ───────────────────────────────────────────────────────

func TestTestFailureTrigger_Basic(t *testing.T) {
	m := &mockTestRunReader{failures: []TestFailure{
		{Package: "./internal/biz", TestName: "TestFoo", ConsecutiveRounds: 3, LastError: "assert failed", LastSeen: time.Now().UTC()},
		{Package: "./internal/data", TestName: "TestBar", ConsecutiveRounds: 2, LastError: "timeout", LastSeen: time.Now().UTC()},
	}}
	tr := NewTestFailureTrigger(m, 2, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if m.gotRound != 2 {
		t.Errorf("rounds = %d, want 2", m.gotRound)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	assertPlatformSuggestion(t, got[0], TriggerSourceTestFailure, EvolutionActionPatchCode)
	// 连续失败轮次越多优先级越高：3 轮 → priority 2，2 轮 → priority 1。
	byName := map[string]UnifiedEvolutionSuggestion{}
	for _, s := range got {
		var meta map[string]any
		if err := json.Unmarshal(s.Metadata, &meta); err != nil {
			t.Fatal(err)
		}
		byName[meta["test_name"].(string)] = s
	}
	if byName["TestFoo"].Priority != 2 {
		t.Errorf("TestFoo priority = %d, want 2", byName["TestFoo"].Priority)
	}
	if byName["TestBar"].Priority != 1 {
		t.Errorf("TestBar priority = %d, want 1", byName["TestBar"].Priority)
	}
}

func TestTestFailureTrigger_MaxPerTickCap(t *testing.T) {
	var failures []TestFailure
	for i := 0; i < 12; i++ {
		failures = append(failures, TestFailure{
			Package: "./internal/x", TestName: "TestN" + string(rune('A'+i)),
			ConsecutiveRounds: 2, LastSeen: time.Now().UTC(),
		})
	}
	m := &mockTestRunReader{failures: failures}
	tr := NewTestFailureTrigger(m, 2, 5, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("len = %d, want capped at 5", len(got))
	}
	// 去重签名必须各不相同（每个测试一条）。
	seen := map[string]bool{}
	for _, s := range got {
		if seen[s.TargetID] {
			t.Errorf("duplicate TargetID %q", s.TargetID)
		}
		seen[s.TargetID] = true
	}
}

func TestTestFailureTrigger_NilReader(t *testing.T) {
	tr := NewTestFailureTrigger(nil, 0, 0, loggateway.NewNoop())
	got, err := tr.Check(context.Background(), "platform")
	if err != nil || got != nil {
		t.Errorf("nil reader: got=%v err=%v", got, err)
	}
}
