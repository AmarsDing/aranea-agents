package biz

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

// ── D3-FIX-1（87-orchestration-chain-hardening）：poller 复活扫描 ────────────
// interrupted（持久失败类中断原因）且全部团队已终态的精灵会话收到完成兜底
// 通知；用户/澄清类中断原因不复活。dedup/CAS/budget 在 service 侧守门，本
// 扫描只是只读兜底。

func newPollerReviveFixture(t *testing.T, sessions map[string]Session, teams map[string]Team) (*SpiritTeamUsecase, *recordingAllDoneNotifier) {
	t.Helper()
	teamRepo := newMemSpiritTeamRepo()
	for id, team := range teams {
		teamRepo.items[id] = team
	}
	sessionRepo := newMemSpiritSessionRepo()
	for id, sess := range sessions {
		sessionRepo.items[id] = sess
	}
	teamUsecase := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	uc := NewSpiritTeamUsecase(teamUsecase, sessionUC, &keyedSpiritAgentResolver{id: "agent_spirit", key: SpiritAgentKey}, loggateway.NewNoop())
	n := &recordingAllDoneNotifier{}
	uc.SetAllTeamsCompletedNotifier(n)
	return uc, n
}

// interrupted（error）+ 全团队终态 → 复活通知 1 次。
func TestPollTeamCompletions_RevivesInterruptedOnError(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-int": {ID: "sp-int", AgentID: "agent_spirit", Status: string(session.SessionStatusInterrupted), StatusReason: string(session.StatusReasonError)},
		},
		map[string]Team{
			"t-done": {ID: "t-done", SpiritSessionID: "sp-int", Status: TeamStatusCompleted},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 1 || n.ids[0] != "sp-int" {
		t.Fatalf("notified %v, want [sp-int] (interrupted/error is revivable)", n.ids)
	}
}

// interrupted（user_cancelled 用户主动取消）→ 不复活。
func TestPollTeamCompletions_SkipsUserCancelledInterrupted(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-int": {ID: "sp-int", AgentID: "agent_spirit", Status: string(session.SessionStatusInterrupted), StatusReason: string(session.StatusReasonUserCancelled)},
		},
		map[string]Team{
			"t-done": {ID: "t-done", SpiritSessionID: "sp-int", Status: TeamStatusCompleted},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 0 {
		t.Fatalf("notified %v, want none (user_cancelled 是用户意图，不得复活)", n.ids)
	}
}

// interrupted（unexpected_shutdown 进程重启收割）→ 复活通知 1 次。
func TestPollTeamCompletions_RevivesUnexpectedShutdown(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-int": {ID: "sp-int", AgentID: "agent_spirit", Status: string(session.SessionStatusInterrupted), StatusReason: string(session.StatusReasonUnexpectedShutdown)},
		},
		map[string]Team{
			"t-done": {ID: "t-done", SpiritSessionID: "sp-int", Status: TeamStatusCompleted},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 1 || n.ids[0] != "sp-int" {
		t.Fatalf("notified %v, want [sp-int] (unexpected_shutdown is revivable)", n.ids)
	}
}

// interrupted（error）但团队未全部终态 → 通知 0 次（不越权推进）。
func TestPollTeamCompletions_InterruptedButTeamsRunning_Skips(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-int": {ID: "sp-int", AgentID: "agent_spirit", Status: string(session.SessionStatusInterrupted), StatusReason: string(session.StatusReasonError)},
		},
		map[string]Team{
			"t-running": {ID: "t-running", SpiritSessionID: "sp-int", Status: TeamStatusRunning},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 0 {
		t.Fatalf("notified %v, want none (teams not all terminal)", n.ids)
	}
}

// running 扫描回归：running + 全团队终态 → 通知 1 次。
func TestPollTeamCompletions_RunningScanRegression(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-run": {ID: "sp-run", AgentID: "agent_spirit", Status: string(session.SessionStatusRunning)},
		},
		map[string]Team{
			"t-done": {ID: "t-done", SpiritSessionID: "sp-run", Status: TeamStatusCompleted},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 1 || n.ids[0] != "sp-run" {
		t.Fatalf("notified %v, want [sp-run] (running 扫描语义不变)", n.ids)
	}
}

// interrupted（context_overflow）→ 不复活（人工介入类）。
func TestPollTeamCompletions_SkipsContextOverflowInterrupted(t *testing.T) {
	uc, n := newPollerReviveFixture(t,
		map[string]Session{
			"sp-int": {ID: "sp-int", AgentID: "agent_spirit", Status: string(session.SessionStatusInterrupted), StatusReason: string(session.StatusReasonContextOverflow)},
		},
		map[string]Team{
			"t-done": {ID: "t-done", SpiritSessionID: "sp-int", Status: TeamStatusCompleted},
		},
	)
	uc.orchestration.pollTeamCompletions(context.Background())
	if len(n.ids) != 0 {
		t.Fatalf("notified %v, want none (context_overflow 不复活)", n.ids)
	}
}
