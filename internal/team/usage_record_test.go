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

func newUsageTestRunner(usage biz.TeamUsageQuerier, sessions biz.SessionTurnManager) *Runner {
	return &Runner{
		usage:     usage,
		runWriter: &stepBusRunWriter{},
		td:        rt.TurnDeps{Sessions: sessions},
	}
}

// P2-1 双计根治：anchor-fallback 的 member 行携带 run 全量（与 team_turn 行
// 同量），落库但必须跳过 session 累加（team_turn 行是唯一 session 累加源）；
// 真实 per-member 行（flag=false，如失败路径部分 step）保留累加。
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

		r.recordMemberUsage(context.Background(), run, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", true)

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

		r.recordMemberUsage(context.Background(), run, "team-1", ag, asst, "deepseek", "deepseek-chat", "default", "step-1", 100, "streaming", false)

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
		r.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "deepseek", "deepseek-chat", "default", 0, 40, "streaming", time.Time{}, false)

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
		r.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 0, 0, "", time.Time{}, false)

		repo := r.runWriter.(*stepBusRunWriter)
		if len(repo.steps) != 1 {
			t.Fatalf("steps=%d", len(repo.steps))
		}
		if repo.steps[0].CostMicroUSD != 0 {
			t.Fatalf("CostMicroUSD=%d want 0", repo.steps[0].CostMicroUSD)
		}
	})
}
