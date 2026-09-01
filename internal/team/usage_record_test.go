package team

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
)

// fakeTeamUsage implements biz.TeamUsageQuerier for usage-record tests.
type fakeTeamUsage struct {
	events    []biz.TokenUsageEvent
	quoteCost int64
}

func (f *fakeTeamUsage) RecordTokenUsageEvent(_ context.Context, e biz.TokenUsageEvent) (biz.TokenUsageEvent, error) {
	if e.CallCount <= 0 {
		e.CallCount = 1
	}
	f.events = append(f.events, e)
	return e, nil
}

func (f *fakeTeamUsage) RecordAuxLLMUsage(context.Context, biz.AuxLLMUsageInput) error { return nil }

func (f *fakeTeamUsage) QuoteTokenUsageCostMicroUSD(_ context.Context, _, _ string, _, _, _ int) int64 {
	return f.quoteCost
}

// fakeMetricSessions embeds the composite interface (nil for unused methods)
// and captures AccumulateMetricsDelta calls.
type fakeMetricSessions struct {
	biz.SessionTurnManager
	deltas []session.SessionMetricsDelta
}

func (f *fakeMetricSessions) AccumulateMetricsDelta(d session.SessionMetricsDelta) {
	f.deltas = append(f.deltas, d)
}

func (f *fakeMetricSessions) FlushSessionMetrics(_ string) {}

func newUsageTestRunner(usage biz.TeamUsageQuerier, sessions biz.SessionTurnManager) *Runner {
	return &Runner{
		usage:     usage,
		runWriter: &stepBusRunWriter{},
		td:        rt.TurnDeps{Sessions: sessions},
	}
}

// P2-1 双计根治：anchor-fallback 的 member 行携带 run 全量（与 team_turn 行
// 同量），落库但必须跳过 session 累加（team_turn 行是唯一 session 累加源）；
// 无 attribution 标记的行（如失败路径部分 step）保留累加。
func TestRecordMemberUsage_RunLevelAttributionSkipsSessionAccum(t *testing.T) {
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a"}
	asst := biz.ChatMessage{
		Role: "assistant", Status: biz.TeamMemberStepStatusOK,
		TokenIn: 280, TokenOut: 55, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	t.Run("run-level fallback row recorded but not accumulated", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)

		r.recordMemberUsage(context.Background(), run, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", biz.UsageAttributionRunLevelAnchorFallback)

		if len(usage.events) != 1 {
			t.Fatalf("events=%d want 1", len(usage.events))
		}
		ev := usage.events[0]
		if !strings.Contains(ev.MetadataJSON, biz.UsageAttributionRunLevelAnchorFallback) {
			t.Fatalf("metadata missing attribution marker: %s", ev.MetadataJSON)
		}
		if len(sessions.deltas) != 0 {
			t.Fatalf("run-level fallback row must not accumulate session metrics, got %d deltas", len(sessions.deltas))
		}
	})

	t.Run("genuine per-member row still accumulates", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)

		r.recordMemberUsage(context.Background(), run, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", "")

		if len(usage.events) != 1 {
			t.Fatalf("events=%d want 1", len(usage.events))
		}
		if strings.Contains(usage.events[0].MetadataJSON, biz.MetadataKeyUsageAttribution) {
			t.Fatalf("per-member row must not carry attribution marker: %s", usage.events[0].MetadataJSON)
		}
		if len(sessions.deltas) != 1 {
			t.Fatalf("per-member row must accumulate session metrics, got %d deltas", len(sessions.deltas))
		}
		d := sessions.deltas[0]
		if d.InputTokens != 280 || d.OutputTokens != 55 || d.ModelCallCount != 1 {
			t.Fatalf("delta = %+v, want tokens 280/55 call 1", d)
		}
	})

	t.Run("genuine row mirrors to spirit session (T3 fix)", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)
		spiritRun := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-team-1", SpiritSessionID: "sess-spirit-1"}

		r.recordMemberUsage(context.Background(), spiritRun, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", "")

		if len(sessions.deltas) != 2 {
			t.Fatalf("expected 2 deltas (team + spirit), got %d", len(sessions.deltas))
		}
		teamDelta := sessions.deltas[0]
		spiritDelta := sessions.deltas[1]
		if teamDelta.SessionID != "sess-team-1" {
			t.Fatalf("delta[0].SessionID = %q, want team session", teamDelta.SessionID)
		}
		if spiritDelta.SessionID != "sess-spirit-1" {
			t.Fatalf("delta[1].SessionID = %q, want spirit session", spiritDelta.SessionID)
		}
		if spiritDelta.InputTokens != 280 || spiritDelta.OutputTokens != 55 {
			t.Fatalf("spirit delta tokens = %d/%d, want 280/55", spiritDelta.InputTokens, spiritDelta.OutputTokens)
		}
	})

	t.Run("spirit mirror skipped when SpiritSessionID empty", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)
		plainRun := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1", SpiritSessionID: ""}

		r.recordMemberUsage(context.Background(), plainRun, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", "")

		if len(sessions.deltas) != 1 {
			t.Fatalf("expected 1 delta (no spirit mirror), got %d", len(sessions.deltas))
		}
	})

	t.Run("spirit mirror skipped when SpiritSessionID equals SessionID", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)
		sameRun := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1", SpiritSessionID: "sess-1"}

		r.recordMemberUsage(context.Background(), sameRun, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", "")

		if len(sessions.deltas) != 1 {
			t.Fatalf("expected 1 delta (same session, no mirror), got %d", len(sessions.deltas))
		}
	})

	t.Run("attributed row skips both team and spirit accumulation", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)
		spiritRun := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-team-1", SpiritSessionID: "sess-spirit-1"}

		r.recordMemberUsage(context.Background(), spiritRun, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", biz.UsageAttributionMemberLevelStream)

		if len(sessions.deltas) != 0 {
			t.Fatalf("attributed row must not accumulate to any session, got %d deltas", len(sessions.deltas))
		}
	})

	t.Run("member_level_stream row recorded but not accumulated", func(t *testing.T) {
		usage := &fakeTeamUsage{}
		sessions := &fakeMetricSessions{}
		r := newUsageTestRunner(usage, sessions)

		r.recordMemberUsage(context.Background(), run, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", biz.UsageAttributionMemberLevelStream)

		if len(usage.events) != 1 {
			t.Fatalf("events=%d want 1", len(usage.events))
		}
		if !strings.Contains(usage.events[0].MetadataJSON, biz.UsageAttributionMemberLevelStream) {
			t.Fatalf("metadata missing member_level_stream marker: %s", usage.events[0].MetadataJSON)
		}
		if len(sessions.deltas) != 0 {
			t.Fatalf("member_level_stream row must not accumulate session metrics, got %d deltas", len(sessions.deltas))
		}
	})
}

// P2-1 TeamRunStep.CostMicroUSD 回填：persistStep 落库前经 Quote 报价回填；
// usage 未装配时保持 0（既有行为）。
func TestPersistStep_BackfillsCostMicroUSD(t *testing.T) {
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{
		Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK,
		TokenIn: 100, TokenOut: 20, CreatedAt: "2026-01-01T00:00:00Z",
	}

	t.Run("quote result lands on step", func(t *testing.T) {
		usage := &fakeTeamUsage{quoteCost: 4200}
		r := newUsageTestRunner(usage, nil)
		r.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "deepseek", "deepseek-chat", "default", 0, 40, "streaming", time.Time{}, "")

		repo := r.runWriter.(*stepBusRunWriter)
		if len(repo.steps) != 1 {
			t.Fatalf("steps=%d", len(repo.steps))
		}
		if repo.steps[0].CostMicroUSD != 4200 {
			t.Fatalf("CostMicroUSD=%d want 4200", repo.steps[0].CostMicroUSD)
		}
	})

	t.Run("nil usage keeps zero cost", func(t *testing.T) {
		r := newUsageTestRunner(nil, nil)
		r.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 0, 0, "", time.Time{}, "")

		repo := r.runWriter.(*stepBusRunWriter)
		if len(repo.steps) != 1 {
			t.Fatalf("steps=%d", len(repo.steps))
		}
		if repo.steps[0].CostMicroUSD != 0 {
			t.Fatalf("CostMicroUSD=%d want 0", repo.steps[0].CostMicroUSD)
		}
	})
}
