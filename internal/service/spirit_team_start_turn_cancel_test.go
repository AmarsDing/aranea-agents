package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

// ── D1-F1：StartTeamTurn 取消分流（调度/dispatch 入口）─────────────────────
// 87-orchestration-chain-hardening：runner 被用户取消（context.Canceled / ctx
// 已取消）时，StartTeamTurn 必须让团队落 cancelled 而非 failed——Cancelled
// Teams>0 → synthesis skipped（中断不出报告）依赖该终态；recordTeamCompletion
// 不得计数。

// newStartTurnTestStarter 构造可运行 StartTeamTurn 的 TeamStarter：session 库
// 走 f10SessionRepo（Get/Update 真实内存语义），teams 走 stub reader/writer
// 记录转移序列。
func newStartTurnTestStarter(sess biz.Session, team biz.Team, runner biz.TeamRunnerWirePort, controller *stubSpiritTeamController, writer *stubTeamWriter) *TeamStarter {
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    writer,
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	return &TeamStarter{
		sessions: biz.NewSessionUsecase(&f10SessionRepo{sessions: []biz.Session{sess}}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller, TeamsNative: runner},
		lg:       loggateway.NewNoop(),
	}
}

func TestStartTeamTurn_CancelledErrReportsCancelled(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-cancelled-1", DisplayName: "被取消团队", AutoCreated: true, Status: biz.TeamStatusRunning}
	sess := biz.Session{
		ID: "team-sess-c1", TeamID: team.ID, ParentSessionID: "spirit-c1",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := &capturingTeamRunner{runErr: context.Canceled}
	s := newStartTurnTestStarter(sess, team, runner, controller, writer)

	err := s.StartTeamTurn(context.Background(), sess.ID, "执行团队任务")
	if err == nil {
		t.Fatal("cancelled runner error must propagate from StartTeamTurn")
	}

	var hasCancelled, hasFailed bool
	for _, tr := range writer.snapshotTransitions() {
		if tr.id != team.ID {
			continue
		}
		switch tr.newStatus {
		case biz.TeamStatusCancelled:
			hasCancelled = true
		case biz.TeamStatusFailed:
			hasFailed = true
		}
	}
	if !hasCancelled {
		t.Errorf("teams transition (team → cancelled) missing for user-cancelled run, got %#v", writer.snapshotTransitions())
	}
	if hasFailed {
		t.Errorf("user-cancelled run must NOT transition team to failed, got %#v", writer.snapshotTransitions())
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("recordTeamCompletion calls = %d, want 0 (cancelled is not a completion)", controller.recordCompletionCalls)
	}
}

func TestStartTeamTurn_GenericErrStillFailed(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-failed-1", DisplayName: "失败团队", AutoCreated: true, Status: biz.TeamStatusRunning}
	sess := biz.Session{
		ID: "team-sess-f1", TeamID: team.ID, ParentSessionID: "spirit-f1",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := &capturingTeamRunner{runErr: errors.New("llm provider down")}
	s := newStartTurnTestStarter(sess, team, runner, controller, writer)

	err := s.StartTeamTurn(context.Background(), sess.ID, "执行团队任务")
	if err == nil {
		t.Fatal("runner error must propagate from StartTeamTurn")
	}

	var hasCancelled, hasFailed bool
	for _, tr := range writer.snapshotTransitions() {
		if tr.id != team.ID {
			continue
		}
		switch tr.newStatus {
		case biz.TeamStatusCancelled:
			hasCancelled = true
		case biz.TeamStatusFailed:
			hasFailed = true
		}
	}
	if hasCancelled {
		t.Errorf("generic error must NOT transition team to cancelled, got %#v", writer.snapshotTransitions())
	}
	if !hasFailed {
		t.Errorf("teams transition (team → failed) missing for generic runner error, got %#v", writer.snapshotTransitions())
	}
}
