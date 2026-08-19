package team

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// 2026-07-28 修复3：runner 侧真实产出闸门
//
// DAG 团队 LLM turn 完成后，runner 在 finalizeTeamRun 中把 v1 team_runs 标为
// success（FSM 终态，不可逆）。若团队从未调用 set_deliverable 提交真实交付物，
// service 层闸门会把 team 翻转为 failed，但 run 记录已固化 success ——
// 「无交付物的成功」。runner 必须在 success 转换前拦截：闸门否决 →
// finishRunErr（running→failed，FSM 合法），与 service 闸门互为双保险。
// ---------------------------------------------------------------------------

// gateRunRepo is an in-memory TeamRun reader/writer for gate tests. Unused
// interface methods are satisfied via embedded nil interfaces (never called
// on the finalize path under test).
type gateRunRepo struct {
	biz.TeamRunReader
	biz.TeamRunWriter
	runs map[string]biz.TeamRunRecord
}

func (r *gateRunRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	run, ok := r.runs[id]
	if !ok {
		return biz.TeamRunRecord{}, fmt.Errorf("run %s not found", id)
	}
	return run, nil
}

func (r *gateRunRepo) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}

func (r *gateRunRepo) UpdateTeamRun(_ context.Context, run biz.TeamRunRecord) error {
	r.runs[run.ID] = run
	return nil
}

func (r *gateRunRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }

// gateRunTransitioner applies FSM-validated transitions against gateRunRepo,
// mirroring biz.TeamUsecase.TransitionRunStatus semantics.
type gateRunTransitioner struct{ repo *gateRunRepo }

func (t *gateRunTransitioner) TransitionRunStatus(_ context.Context, runID, newStatus string) (biz.TeamRunRecord, error) {
	run, err := t.repo.GetTeamRunByID(context.Background(), runID)
	if err != nil {
		return biz.TeamRunRecord{}, err
	}
	sm := biz.NewTeamRunStateMachine()
	if !sm.CanTransition(biz.TeamRunState(run.Status), biz.TeamRunState(newStatus)) {
		return biz.TeamRunRecord{}, fmt.Errorf("invalid team run status transition: %s → %s", run.Status, newStatus)
	}
	run.Status = newStatus
	t.repo.runs[runID] = run
	return run, nil
}

func newGateTestRunner(repo *gateRunRepo, bus *event.V2Bus) *Runner {
	return &Runner{
		runReader:       repo,
		runWriter:       repo,
		runTransitioner: &gateRunTransitioner{repo: repo},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{EventBus: bus},
		},
		lg: loggateway.NewNoop(),
	}
}

func gateTestFixture() (biz.Session, biz.TeamRunRecord, anchorResolution, biz.ChatMessage) {
	sess := biz.Session{ID: "sess-1"}
	run := biz.TeamRunRecord{
		ID:        "run-1",
		TeamID:    "team-1",
		SessionID: "sess-1",
		Status:    biz.TeamRunStatusRunning,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	ar := anchorResolution{
		member: MemberDef{Role: "worker"},
		agent:  biz.Agent{ID: "a1", AgentKey: "worker-a"},
		prov:   "prov",
		mod:    "mod",
	}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK}
	return sess, run, ar, asst
}

// drainCompletedStageEvents reports whether a TeamStageCompletedEvent was
// published to the bus (non-blocking drain).
func drainCompletedStageEvents(ch <-chan biz.Event) bool {
	for {
		select {
		case e := <-ch:
			if _, ok := e.(*biz.TeamStageCompletedEvent); ok {
				return true
			}
		default:
			return false
		}
	}
}

// 闸门否决（无真实交付物）→ run 必须标 failed，且不得发布 TeamStageCompletedEvent。
func TestFinalizeTeamRun_DagTeamWithoutDeliverable_MarkedFailed(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	ch, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return false, nil })

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusFailed {
		t.Fatalf("returned run status=%q want %q", got.Status, biz.TeamRunStatusFailed)
	}
	persisted := repo.runs[run.ID]
	if persisted.Status != biz.TeamRunStatusFailed {
		t.Fatalf("persisted run status=%q want %q", persisted.Status, biz.TeamRunStatusFailed)
	}
	if persisted.ErrorMessage == "" {
		t.Fatal("persisted run ErrorMessage is empty, want gate rejection reason")
	}
	if drainCompletedStageEvents(ch) {
		t.Fatal("TeamStageCompletedEvent must not be published for a gate-rejected run")
	}
}

// 闸门通过（有真实交付物）→ run 正常标 success。
func TestFinalizeTeamRun_DagTeamWithDeliverable_Success(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) { return true, nil })

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want %q", got.Status, biz.TeamRunStatusSuccess)
	}
}

// 非 DAG 团队（DagNodeID 为空）→ 闸门跳过，即使闸门否决也标 success。
func TestFinalizeTeamRun_NonDagTeam_GateSkipped(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	gateCalled := false
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) {
		gateCalled = true
		return false, nil
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1"} // no DagNodeID

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if gateCalled {
		t.Fatal("deliverable gate must not be consulted for non-DAG teams")
	}
	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want %q", got.Status, biz.TeamRunStatusSuccess)
	}
}

// 闸门 infra 错误 → 按无交付物处理（与 service 闸门语义一致），run 标 failed。
func TestFinalizeTeamRun_GateError_MarkedFailed(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus)
	runner.SetDeliverableGate(func(_ context.Context, _ biz.Team) (bool, error) {
		return false, errors.New("state unreadable")
	})

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusFailed {
		t.Fatalf("run status=%q want %q", got.Status, biz.TeamRunStatusFailed)
	}
}

// 未装配闸门（nil）→ 保持既有行为标 success（向后兼容：非 spirit 编排部署）。
func TestFinalizeTeamRun_NilGate_Success(t *testing.T) {
	repo := &gateRunRepo{runs: map[string]biz.TeamRunRecord{}}
	bus := event.NewV2Bus()
	_, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := newGateTestRunner(repo, bus) // no SetDeliverableGate

	sess, run, ar, asst := gateTestFixture()
	repo.runs[run.ID] = run
	teamRow := biz.Team{ID: "team-1", DagNodeID: "st_1"}

	got := runner.finalizeTeamRun(context.Background(), sess, run, teamRow, ar, asst, 0, 0, 0, "default", "", "", time.Now(), nil)

	if got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run status=%q want %q", got.Status, biz.TeamRunStatusSuccess)
	}
}
