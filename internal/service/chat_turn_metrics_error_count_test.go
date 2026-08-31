package service

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// R4-Q10：session_metrics.error_count 记账回归测试。失败状态的 turn 必须向
// session metrics delta 累加 ErrorCount=1；ok/cancelled/timeout_degraded 不计。
func TestRecordTurnUsage_ErrorStatusAccumulatesErrorCount(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"error", 1},
		{"failed", 1},
		{"orphaned", 1},
		{"ok", 0},
		{"cancelled", 0},
		{"timeout_degraded", 0},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			stub := &stubSessionTurnRecorder{}
			m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
			m.RecordTurnUsage(context.Background(), TurnUsageParams{
				SessionID: "s1",
				RunID:     "r1",
				Status:    tc.status,
			})
			got := 0
			for _, d := range stub.deltas {
				got += d.ErrorCount
			}
			if got != tc.want {
				t.Errorf("status %q: accumulated ErrorCount = %d, want %d", tc.status, got, tc.want)
			}
		})
	}
}

// 空 SessionID 不产生 delta（防御：脏数据不得入聚合）。
func TestRecordTurnUsage_ErrorCountSkipsEmptySession(t *testing.T) {
	stub := &stubSessionTurnRecorder{}
	m := newChatTurnMetrics(stub, nil, nil, loggateway.NewNoop())
	m.RecordTurnUsage(context.Background(), TurnUsageParams{SessionID: "  ", Status: "error"})
	if len(stub.deltas) != 0 {
		t.Errorf("expected no delta for empty session, got %d", len(stub.deltas))
	}
}
