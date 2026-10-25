package service

import (
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
)

// completed 回调 + 交付物门通过 + 验证门拒绝 → 团队 failed：不记录完成、
// 级联调度一次、发布 TeamStageFailedEvent。
func TestHandleTeamTurnResult_VerificationGateRejected_MarkedFailed(t *testing.T) {
	team := biz.Team{
		ID: "team-gate-rej", DisplayName: "安装团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{
		hasRealDeliverable: true,
		gateReject:         true,
		gateReasons:        []string{"skill 'X' 安装校验失败: enabled=false"},
	}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.gateCalls != 1 {
		t.Errorf("ExecuteVerificationGates called %d times, want 1", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 (gate rejected)", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 1 {
		t.Errorf("ScheduleDependentTeams called %d times, want 1 (cascade fail downstream)", controller.scheduleDependentsCalls)
	}
	var failedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			failedFound = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published when the verification gate rejects")
		}
	}
	if !failedFound {
		t.Errorf("no TeamStageFailedEvent published, want one for the gate-rejected team")
	}
}

// completed 回调 + 交付物门通过 + 验证门执行错误（infra）→ fail-closed
// 翻转 failed（与 Fix-1 门同姿态：校验不可用不等于通过）。
func TestHandleTeamTurnResult_VerificationGateError_MarkedFailed(t *testing.T) {
	team := biz.Team{
		ID: "team-gate-err", DisplayName: "安装团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{
		hasRealDeliverable: true,
		gateErr:            errors.New("tool_assertion gate: no tool invoker configured"),
	}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.gateCalls != 1 {
		t.Errorf("ExecuteVerificationGates called %d times, want 1", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 (gate error, fail-closed)", controller.recordCompletionCalls)
	}
	var failedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			failedFound = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published on gate infra error")
		}
	}
	if !failedFound {
		t.Errorf("no TeamStageFailedEvent published, want one for the gate-error team")
	}
}

// newStandaloneStarter 构造 standalone 终态 pass 测试用的 TeamStarter：
// seq 承载成员事件与 TeamStage 事件（publishV2Event 优先 seq），
// teamStageR 预置 Running TeamStage（V=1）供 resolveTeamStageUpdate 推导 V=2。
func newStandaloneStarter(
	team biz.Team,
	controller *stubSpiritTeamController,
	writer *stubTeamWriter,
	sessions []biz.Session,
	seq *capturingSeq,
) *TeamStarter {
	return newStandaloneStarterFromRepo(team, controller, writer, &f10SessionRepo{sessions: sessions}, seq)
}

// newStandaloneStarterFromRepo 与 newStandaloneStarter 同构，但共享调用方
// 持有的 f10SessionRepo——测试可断言 session 层副作用（S-2 interrupted 转换）。
func newStandaloneStarterFromRepo(
	team biz.Team,
	controller *stubSpiritTeamController,
	writer *stubTeamWriter,
	repo *f10SessionRepo,
	seq *capturingSeq,
) *TeamStarter {
	tsID := string(agent.NewTeamStageActivityID(team.ID, ""))
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    writer,
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	return &TeamStarter{
		sessions: biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamStageR: &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
			tsID: {ID: tsID, TeamID: team.ID, SessionID: team.SpiritSessionID,
				Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, Version: 1},
		}},
		tsSM:     biz.NewTeamStageStateMachine(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{}},
		trSM:     biz.NewTeamRunV2StateMachine(),
	}
}

// standalone completed：teams 转 completed + TeamStageCompletedEvent + 成员
// completed（outcome 哨兵带）；编排专属职责全部不触发。
func TestHandleTeamTurnResult_Standalone_Completed_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-ok", DisplayName: "独立团队", SpiritSessionID: "sess-sa-ok",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-ok", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	s.HandleTeamTurnResult(context.Background(), "sess-sa-ok", team.ID, biz.TeamStatusCompleted, "", "")

	trs := writer.snapshotTransitions()
	if len(trs) != 1 || trs[0].newStatus != biz.TeamStatusCompleted || trs[0].id != team.ID {
		t.Fatalf("teams transitions = %+v, want exactly one → completed for %s", trs, team.ID)
	}
	if controller.cancelTimeoutCalls != 1 {
		t.Errorf("CancelTimeoutTimer called %d times, want 1", controller.cancelTimeoutCalls)
	}

	if controller.deliverableGateCalls != 0 {
		t.Errorf("HasRealDeliverable called %d times, want 0 for standalone", controller.deliverableGateCalls)
	}
	if controller.gateCalls != 0 {
		t.Errorf("ExecuteVerificationGates called %d times, want 0 for standalone", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 for standalone", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 0 {
		t.Errorf("ScheduleDependentTeams called %d times, want 0 for standalone", controller.scheduleDependentsCalls)
	}
	// TeamStage 终态事件。
	var tsCompleted bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageCompletedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsCompleted = true
			if tsEv.TeamStage.Version != 2 {
				t.Errorf("TeamStage terminal Version = %d, want 2 (resolveTeamStageUpdate from V=1)", tsEv.TeamStage.Version)
			}
		}
	}
	if !tsCompleted {
		t.Errorf("no TeamStageCompletedEvent published for standalone team")
	}

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusCompleted {
		t.Errorf("agent-a status = %s, want completed", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	if msA.SpiritSessionID != "sess-sa-ok" {
		t.Errorf("agent-a SpiritSessionID = %q, want fallback aggregate root sess-sa-ok", msA.SpiritSessionID)
	}
}

// standalone failed：teams 转 failed + TeamStageFailedEvent + 成员 failed
// （outcome 哨兵带，无需证据——团队 failed 直接传播）。
func TestHandleTeamTurnResult_Standalone_Failed_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-fail", DisplayName: "独立团队", SpiritSessionID: "sess-sa-fail",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-fail", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	s.HandleTeamTurnResult(context.Background(), "sess-sa-fail", team.ID, biz.TeamStatusFailed, "boom", "")

	trs := writer.snapshotTransitions()
	if len(trs) != 1 || trs[0].newStatus != biz.TeamStatusFailed {
		t.Fatalf("teams transitions = %+v, want exactly one → failed", trs)
	}
	var tsFailed bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsFailed = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published for a failed standalone team")
		}
	}
	if !tsFailed {
		t.Errorf("no TeamStageFailedEvent published for failed standalone team")
	}
	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusFailed {
		t.Errorf("agent-a status = %s, want failed (team failure propagates)", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}

	if controller.memberEvidenceCalls != 0 {
		t.Errorf("MemberExecutionEvidence called %d times, want 0 (team failed needs no evidence)", controller.memberEvidenceCalls)
	}
}

// standalone 空 spiritSessionID 回退（2026-07-29 S-1）：CancelTeam 等无
// session 上下文的入口传空 spiritSessionID（team.SpiritSessionID=="",
// CreateTeam 不落该字段）时，service 必须内部回退查 team session 作聚合根
// （F-3 语义兜底）；此前 publishV2TeamRunCompletion 的空守卫直接 return，
// TeamStage/TeamRun/MemberSession 永不达终态（成员永 running）。
func TestHandleTeamTurnResult_Standalone_EmptySpiritID_FallbackToTeamSession(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-e", DisplayName: "独立团队", SpiritSessionID: "",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-e", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-e-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCompleted, "", "")

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a (empty spiritSessionID must fall back to team session)")
	}
	if msA.Status != biz.MemberSessionStatusCompleted {
		t.Errorf("agent-a status = %s, want completed", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	if msA.SpiritSessionID != "sess-sa-e" {
		t.Errorf("agent-a SpiritSessionID = %q, want fallback team session sess-sa-e", msA.SpiritSessionID)
	}
	// TeamStage 终态事件同样携带回退后的聚合根。
	var tsSessionID string
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageCompletedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsSessionID = tsEv.TeamStage.SessionID
		}
	}
	if tsSessionID != "sess-sa-e" {
		t.Errorf("TeamStage SessionID = %q, want fallback team session sess-sa-e", tsSessionID)
	}
}

// standalone cancelled（2026-07-29 S-1+S-2）：teams 已由调用方（biz.CancelTeam）
// 转换、不重复；成员 skipped（outcome 哨兵带）；running 成员 session 转
// interrupted（对齐 AutoCreated 路径）；TeamStage cancelled 事件发布。
func TestHandleTeamTurnResult_Standalone_Cancelled_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-x", DisplayName: "独立团队", SpiritSessionID: "",
		AutoCreated: false, Status: biz.TeamStatusCancelled,
	}
	repo := &f10SessionRepo{sessions: []biz.Session{
		{ID: "sess-sa-x", TeamID: team.ID, SessionType: "team", Status: string(sessstatus.SessionStatusRunning)},
		{ID: "sess-sa-x-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a", Status: string(sessstatus.SessionStatusRunning)},
	}}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarterFromRepo(team, controller, writer, repo, seq)

	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCancelled, "", "")

	if trs := writer.snapshotTransitions(); len(trs) != 0 {
		t.Errorf("teams transitions = %+v, want none for cancelled (caller already transitioned)", trs)
	}
	// TeamStage cancelled 事件（UpdatedEvent 语义，无 CancelledEvent 工厂）。
	var tsCancelled bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageUpdatedEvent); ok && tsEv.TeamStage.TeamID == team.ID &&
			tsEv.TeamStage.Status == biz.TeamStageStatusCancelled {
			tsCancelled = true
			if tsEv.TeamStage.SessionID != "sess-sa-x" {
				t.Errorf("TeamStage SessionID = %q, want fallback team session sess-sa-x", tsEv.TeamStage.SessionID)
			}
		}
	}
	if !tsCancelled {
		t.Errorf("no TeamStage cancelled event published for standalone team")
	}

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusSkipped {
		t.Errorf("agent-a status = %s, want skipped (team cancelled)", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}

	if len(repo.interruptCalls) == 0 {
		t.Errorf("no member session interrupted transition, want running sessions interrupted on cancel")
	}
}

// ── D1-F2：standalone cancelled 现态守卫（87-orchestration-chain-hardening）──
// 破除「调用方已转换」假设：runner 取消回调（StartTeamTurn 取消分流）进入
// standalone 终态 pass 时 teams 现态仍是 running，service 必须补 CAS 转移
// cancelled（FSM 已有 running→cancelled 边）；已 cancelled 的重放（如
// biz.CancelTeam 显式清理）保持零转移幂等。

func TestHandleTeamTurnResult_Standalone_Cancelled_TransitionsIfRunning(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-x2", DisplayName: "独立团队", SpiritSessionID: "",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	repo := &f10SessionRepo{sessions: []biz.Session{
		{ID: "sess-sa-x2", TeamID: team.ID, SessionType: "team", Status: string(sessstatus.SessionStatusRunning)},
		{ID: "sess-sa-x2-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a", Status: string(sessstatus.SessionStatusRunning)},
	}}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarterFromRepo(team, controller, writer, repo, seq)

	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCancelled, "context canceled", "sess-sa-x2")

	var cancelled bool
	for _, tr := range writer.snapshotTransitions() {
		if tr.id == team.ID && tr.newStatus == biz.TeamStatusCancelled && tr.expectCurrent == biz.TeamStatusRunning {
			cancelled = true
		}
	}
	if !cancelled {
		t.Errorf("standalone teams transition (running → cancelled) missing, got %#v", writer.snapshotTransitions())
	}
	if len(repo.interruptCalls) == 0 {
		t.Errorf("no member session interrupted transition, want running sessions interrupted on cancel")
	}
}

// standalone cancelled 同态重放（biz.CancelTeam 已转换的显式清理路径）：
// 零转移、成员收敛不重复（成员 session 已 interrupted 不再处理）。
func TestHandleTeamTurnResult_Standalone_Cancelled_AlreadyCancelled_Idempotent(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-x3", DisplayName: "独立团队", SpiritSessionID: "",
		AutoCreated: false, Status: biz.TeamStatusCancelled,
	}
	repo := &f10SessionRepo{sessions: []biz.Session{
		{ID: "sess-sa-x3", TeamID: team.ID, SessionType: "team", Status: string(sessstatus.SessionStatusInterrupted)},
		{ID: "sess-sa-x3-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a", Status: string(sessstatus.SessionStatusInterrupted)},
	}}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarterFromRepo(team, controller, writer, repo, seq)

	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCancelled, "context canceled", "")

	if trs := writer.snapshotTransitions(); len(trs) != 0 {
		t.Errorf("teams transitions = %+v, want none (already cancelled)", trs)
	}
	if len(repo.interruptCalls) != 0 {
		t.Errorf("interrupt calls = %v, want none (no running sessions to converge)", repo.interruptCalls)
	}
}
