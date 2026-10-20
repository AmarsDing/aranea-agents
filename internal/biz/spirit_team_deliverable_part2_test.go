package biz

import (
	"aranea-agents/internal/biz/session"
	"context"
	"testing"
	"time"
)

// 成员会话与团队会话均无该成员 step → ok=false（调用方走下一级回退）。
func TestMemberExecutionWindow_NoSteps_NotFound(t *testing.T) {
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	u := newDeliverableUsecaseWithSteps(newDeliverableTeamRepo(), sessions, steps)
	sessions.sessionsByTeam["t1"] = Session{ID: "ms-1", ParentSessionID: "sess-team", MemberAgentKey: "eng"}
	steps.stepsBySession["sess-team"] = []Step{
		{ID: "o1", Kind: StepKindThinking, Status: StepStatusCompleted, AuthorAgentKey: "other", StartedAt: time.Now().UTC()},
	}

	if _, _, ok := u.MemberExecutionWindow(context.Background(), "ms-1"); ok {
		t.Fatal("no member steps anywhere must yield ok=false")
	}
}

// 未装配 stepReader → ok=false（不臆测）。
func TestMemberExecutionWindow_NilStepReader(t *testing.T) {
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(newDeliverableTeamRepo(), sessions)
	sessions.sessionsByTeam["t1"] = Session{ID: "ms-1", Status: string(session.SessionStatusCompleted)}

	if _, _, ok := u.MemberExecutionWindow(context.Background(), "ms-1"); ok {
		t.Fatal("nil stepReader must yield ok=false")
	}
}
