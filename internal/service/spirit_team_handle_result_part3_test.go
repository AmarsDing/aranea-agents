package service

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"
	"time"
)

// 问题4a 回归（2026-08-08）：终态 MemberSession 事件的 StartedAt/FinishedAt
// 必须取 MemberExecutionWindow 的 step 流聚合窗口，而非发布时刻——否则前端
// 成员耗时恒为 ≈0。无窗口证据的成员回退发布时刻（不崩溃、不零值）。
func TestPublishV2TeamRunCompletion_MemberWindowFromStepStream(t *testing.T) {
	team := biz.Team{
		ID: "team-window", DisplayName: "窗口团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCompleted, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
		{ID: "sess-b", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-b"},
	}
	wStart := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	wEnd := wStart.Add(3 * time.Minute)
	ctrl := &stubSpiritTeamController{
		hasRealDeliverable: true,
		memberWindow: map[string]stubMemberWindow{
			"sess-a": {start: wStart, end: wEnd, ok: true},
		},
	}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	before := time.Now()
	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)
	after := time.Now()

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if !msA.StartedAt.Equal(wStart) {
		t.Errorf("agent-a StartedAt = %v, want window start %v", msA.StartedAt, wStart)
	}
	if msA.FinishedAt == nil || !msA.FinishedAt.Equal(wEnd) {
		t.Errorf("agent-a FinishedAt = %v, want window end %v", msA.FinishedAt, wEnd)
	}
	msB, ok := got["agent-b"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-b")
	}
	if msB.StartedAt.Before(before.Add(-time.Second)) || msB.StartedAt.After(after.Add(time.Second)) {
		t.Errorf("agent-b StartedAt = %v, want publish-time fallback within [%v, %v]", msB.StartedAt, before, after)
	}
	if msB.FinishedAt == nil {
		t.Errorf("agent-b FinishedAt must not be nil on fallback")
	}
}
