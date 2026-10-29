package service

import (
	"context"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubTeamRunReaderWithRecords 返回预置 v1 运行记录（ListRuns 语义：
// created_at DESC 最新在前由调用方自行排序）。
type stubTeamRunReaderWithRecords struct {
	stubTeamRunReader
	records []biz.TeamRunRecord
}

func (s *stubTeamRunReaderWithRecords) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRunRecord, error) {
	return s.records, nil
}

func teamRunFailedEventFromSeq(seq *capturingSeq) *biz.TeamRunFailedEvent {
	for _, ev := range seq.snapshot() {
		if fe, ok := ev.(*biz.TeamRunFailedEvent); ok {
			return fe
		}
	}
	return nil
}

// TestPublishV2TeamRunCompletion_BackfillsErrorFromV1（2026-10-29 P2）：
// finishRunErr 把真实失败原因写入 v1 team_runs.error_message，但 v2 完成
// 事件此前不填 tr.Error → team_runs_v2.error 恒空 → 综合轮简报只有
// 「任务失败（无详细错误记录）」（44 实证）。failed 终态必须经
// TeamRunV2ID 桥从 v1 回填。
func TestPublishV2TeamRunCompletion_BackfillsErrorFromV1(t *testing.T) {
	team := biz.Team{
		ID: "team-err", DisplayName: "失败团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusFailed, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	// 与 publishV2TeamRunCompletion 相同的确定性派生（ctx 无 root task → ""）。
	wantV2ID := agent.NewTeamRunV2ID(string(agent.NewTeamStageActivityID(team.ID, "")))
	realErr := `execution failed at step 0: node member-1 execution failed: received error while streaming: {"code":500,"message":"Context size has been exceeded.","type":"server_error"}`
	runReader := &stubTeamRunReaderWithRecords{records: []biz.TeamRunRecord{
		{ID: "v1-run-1", TeamID: team.ID, Status: "failed", TeamRunV2ID: wantV2ID, ErrorMessage: realErr},
	}}
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: runReader,
		Lg:        loggateway.NewNoop(),
	})
	seq := &capturingSeq{}
	s := &TeamStarter{
		sessions: biz.NewSessionUsecase(&f10SessionRepo{sessions: sessions}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{}},
		trSM:     biz.NewTeamRunV2StateMachine(),
	}

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusFailed, biz.TeamStatusFailed)

	fe := teamRunFailedEventFromSeq(seq)
	if fe == nil {
		t.Fatal("expected TeamRunFailedEvent published")
	}
	if fe.TeamRun.Error != realErr {
		t.Fatalf("v2 TeamRun.Error must be backfilled from v1, got %q", fe.TeamRun.Error)
	}
}

// TestPublishV2TeamRunCompletion_NoV1ErrorStaysEmpty 对照：v1 无错误记录时
// Error 保持空（best-effort，不编造），发布不中断。
func TestPublishV2TeamRunCompletion_NoV1ErrorStaysEmpty(t *testing.T) {
	team := biz.Team{
		ID: "team-noerr", DisplayName: "无详情团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusFailed, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
	}
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	seq := &capturingSeq{}
	s := &TeamStarter{
		sessions: biz.NewSessionUsecase(&f10SessionRepo{sessions: sessions}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{}},
		trSM:     biz.NewTeamRunV2StateMachine(),
	}

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusFailed, biz.TeamStatusFailed)

	fe := teamRunFailedEventFromSeq(seq)
	if fe == nil {
		t.Fatal("expected TeamRunFailedEvent published")
	}
	if fe.TeamRun.Error != "" {
		t.Fatalf("no v1 error must leave Error empty, got %q", fe.TeamRun.Error)
	}
}
