package team

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// ---------------------------------------------------------------------------
// G3（ADR-G，2026-08-14）：runner 侧质量门集成
//
// 位置：finalizeTeamRun 中二元门（HasRealDeliverable）之后、success 转换之前。
//   - verdict=pass → 照常 success
//   - verdict=revise/fail + 修订预算内（team+session 计数 <2）→ followup 入队
//     （反馈消息）+ run 标 failed（FSM running→failed 合法）
//   - 预算耗尽 / judge infra error / 未装配 enqueuer → fail-open 放行（warn），
//     不得把今天二元门会放行的交付物卡死（防回归）
// ---------------------------------------------------------------------------

func TestFinalizeTeamRun_QualityGatePass_Success(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{Verdict: biz.TeamQualityPass}, nil
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want %q", got.Status, biz.TeamRunStatusSuccess)
	}
}

func TestFinalizeTeamRun_QualityRevise_EnqueuesFollowupAndFails(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{
			Verdict:  biz.TeamQualityRevise,
			Feedback: "内容过于简略",
			RuleHits: []string{"J2"},
		}, nil
	})
	var gotSession, gotContent string
	enqueueCalls := 0
	runner.SetRevisionEnqueuer(func(_ context.Context, sessionID, content string) error {
		enqueueCalls++
		gotSession, gotContent = sessionID, content
		return nil
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusFailed {
		t.Fatalf("revision round must fail the run, status=%q", got.Status)
	}
	if !strings.Contains(repo.runs[run.ID].ErrorMessage, "质量门") {
		t.Fatalf("ErrorMessage should carry the gate rejection, got %q", repo.runs[run.ID].ErrorMessage)
	}
	if enqueueCalls != 1 {
		t.Fatalf("revision followup enqueueCalls=%d want 1", enqueueCalls)
	}
	if gotSession != sess.ID {
		t.Fatalf("followup must target the team session %q, got %q", sess.ID, gotSession)
	}
	if !strings.Contains(gotContent, "内容过于简略") {
		t.Fatalf("followup must carry the judge feedback, got %q", gotContent)
	}
	if c := runner.qualityRevisionCount("team-1", sess.ID); c != 1 {
		t.Fatalf("revision counter=%d want 1", c)
	}
}

func TestFinalizeTeamRun_QualityRevise_BudgetExhausted_FailOpenPass(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{Verdict: biz.TeamQualityRevise, Feedback: "仍不达标"}, nil
	})
	enqueueCalls := 0
	runner.SetRevisionEnqueuer(func(_ context.Context, _, _ string) error { enqueueCalls++; return nil })
	runner.seedQualityRevisionCount("team-1", "sess-1", maxQualityRevisions)

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("exhausted budget must fail-open pass, status=%q", got.Status)
	}
	if enqueueCalls != 0 {
		t.Fatalf("no further followup after budget exhausted, got %d", enqueueCalls)
	}
	if c := runner.qualityRevisionCount("team-1", "sess-1"); c != 0 {
		t.Fatalf("counter must reset after fail-open pass, got %d", c)
	}
}

func TestFinalizeTeamRun_QualityJudgeError_FailOpenPass(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{}, errors.New("state unreadable")
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("judge infra error must fail-open pass, status=%q", got.Status)
	}
}

func TestFinalizeTeamRun_QualityRevise_NoEnqueuer_FailOpenPass(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{Verdict: biz.TeamQualityRevise, Feedback: "不达标"}, nil
	})
	// 未装配 enqueuer（legacy/测试路径）：无修订通道时不得卡死。

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("missing enqueuer must fail-open pass, status=%q", got.Status)
	}
}

// 质量门仅在二元门通过后到达：二元门否决时质量门不得被调用。
func TestFinalizeTeamRun_QualityGateSkippedWhenBinaryVetoes(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return false, nil })
	qualityCalled := false
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		qualityCalled = true
		return biz.QualityGateResult{Verdict: biz.TeamQualityPass}, nil
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)
	if qualityCalled {
		t.Fatal("quality gate must not run after the binary gate vetoes (no deliverable)")
	}
}

// R1-a（2026-09-01 eval0831-s06-fix1 实证）：质量门 revise 拦截的 run 仍消耗了
// 真实 token——team_turn usage 行必须在质量门之前落库。finish-path 成员行带
// attribution 标记（跳过 session 累计），team_turn 行是 run 消耗的唯一 session
// 累计源；此前拦截路径直接 return，该 run 消耗从所有会话级统计中消失。
func TestFinalizeTeamRun_QualityRevise_StillRecordsTeamTurnUsage(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	usage := &fakeTeamUsage{}
	runner.usage = usage
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })
	runner.SetQualityGate(func(_ context.Context, _ biz.Team) (biz.QualityGateResult, error) {
		return biz.QualityGateResult{Verdict: biz.TeamQualityRevise, Feedback: "不达标", RuleHits: []string{"J3"}}, nil
	})
	runner.SetRevisionEnqueuer(func(_ context.Context, _, _ string) error { return nil })

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 300, 80, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusFailed {
		t.Fatalf("revision round must fail the run, status=%q", got.Status)
	}
	teamTurns := 0
	for _, e := range usage.events {
		if e.UsageKind != biz.UsageKindTeamTurn {
			continue
		}
		teamTurns++
		if e.InputTokens != 300 || e.OutputTokens != 80 {
			t.Fatalf("team_turn tokens=%d/%d want 300/80", e.InputTokens, e.OutputTokens)
		}
	}
	if teamTurns != 1 {
		t.Fatalf("team_turn usage rows=%d want 1 (quality-gate-revised runs consumed real tokens and must be accounted)", teamTurns)
	}
}
