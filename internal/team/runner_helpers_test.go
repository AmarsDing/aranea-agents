package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

type stepBusRunWriter struct {
	steps []biz.TeamRunStep
}

func (r *stepBusRunWriter) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	r.steps = append(r.steps, step)
	return step, nil
}

func (r *stepBusRunWriter) CreateTeamRun(_ context.Context, run biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return run, nil
}
func (r *stepBusRunWriter) UpdateTeamRun(_ context.Context, _ biz.TeamRunRecord) error { return nil }
func (r *stepBusRunWriter) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *stepBusRunWriter) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (r *stepBusRunWriter) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
func (r *stepBusRunWriter) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error {
	return nil
}

// TestTeamTurnBaseRunOptions_InjectsTeamIDRuntimeState verifies the C5 root
// injection: the base run options of a team turn carry team_id in RuntimeState
// so the graph runtime can propagate it to member invocations.
func TestTeamTurnBaseRunOptions_InjectsTeamIDRuntimeState(t *testing.T) {
	opts := teamTurnBaseRunOptions("team-123", "do something")
	var ro trpcagent.RunOptions
	for _, opt := range opts {
		opt(&ro)
	}
	got, ok := ro.RuntimeState["team_id"].(string)
	if !ok || got != "team-123" {
		t.Fatalf("RuntimeState[team_id]=%v want %q", ro.RuntimeState["team_id"], "team-123")
	}
}

func TestPersistStep_EmitsFinished(t *testing.T) {
	v2Bus := event.NewV2Bus()
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := &Runner{
		runWriter: &stepBusRunWriter{},
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{EventBus: v2Bus},
		},
	}
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK, CreatedAt: "2026-01-01T00:00:00Z"}

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 2)

	repo := runner.runWriter.(*stepBusRunWriter)
	if len(repo.steps) != 1 {
		t.Fatalf("steps=%d", len(repo.steps))
	}
	if repo.steps[0].ToolCallCount != 2 {
		t.Fatalf("tool_call_count=%d", repo.steps[0].ToolCallCount)
	}
	// persistStep now only emits "completed" (finished) event.
	// The "created" (started) event is emitted by PublishTeamStepStarted
	// at node_start time, or by ensureGraphRunStepsFallback for the
	// fallback path.
	var finished bool
	select {
	case e := <-ch:
		// May receive MemberSessionUpdatedEvent and/or system.notice; accept either.
		switch ev := e.(type) {
		case *biz.MemberSessionUpdatedEvent:
			if ev.MemberSession.AgentName != "Worker A" {
				t.Fatalf("agent_name=%q want %q", ev.MemberSession.AgentName, "Worker A")
			}
			if ev.MemberSession.Status != biz.MemberSessionStatusCompleted {
				t.Fatalf("status=%q want completed", ev.MemberSession.Status)
			}
			finished = true
		case *biz.SystemNoticeEvent:
			if ev.NoticeType != "completed" {
				t.Fatalf("notice=%q want completed", ev.NoticeType)
			}
			csid, _ := ev.Meta["child_session_id"].(string)
			if csid != "sess-1" {
				t.Fatalf("child_session_id=%q want %q", csid, "sess-1")
			}
			finished = true
		default:
			t.Fatalf("expected MemberSessionUpdatedEvent or SystemNoticeEvent, got %T", e)
		}
	default:
	}
	if !finished {
		t.Fatal("expected completed event, got none")
	}
}
