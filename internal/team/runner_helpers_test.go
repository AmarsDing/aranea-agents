package team

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

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

// TestResolveUpstreamDeliverableSeed 锚定 2026-08-08 问题3c：DAG 下游团队
// turn 启动时解析上游交付物种子；无依赖/无 DagNodeID/未装配/解析失败均
// 降级为 nil（种子错误不得拖垮 turn）。
func TestResolveUpstreamDeliverableSeed(t *testing.T) {
	seed := map[string]any{"research": map[string]any{"winner": "bm25"}}

	t.Run("wired DAG team with deps returns seed", func(t *testing.T) {
		r := &Runner{lg: loggateway.NewNoop()}
		r.SetUpstreamDeliverableSeed(func(_ context.Context, team biz.Team) (map[string]any, error) {
			if team.ID != "t2" {
				t.Fatalf("seed fn got team %q want t2", team.ID)
			}
			return seed, nil
		})
		got := r.resolveUpstreamDeliverableSeed(context.Background(), biz.Team{ID: "t2", DagNodeID: "st_1", DependsOn: []string{"st_0"}})
		if len(got) != 1 {
			t.Fatalf("seed=%v want 1 topic", got)
		}
		if got["research"].(map[string]any)["winner"] != "bm25" {
			t.Fatalf("seed[research] mismatch: %v", got["research"])
		}
	})

	t.Run("nil resolver degrades to nil", func(t *testing.T) {
		r := &Runner{lg: loggateway.NewNoop()}
		if got := r.resolveUpstreamDeliverableSeed(context.Background(), biz.Team{ID: "t2", DagNodeID: "st_1", DependsOn: []string{"st_0"}}); got != nil {
			t.Fatalf("seed=%v want nil without resolver", got)
		}
	})

	t.Run("standalone team (no DagNodeID) skips resolution", func(t *testing.T) {
		r := &Runner{lg: loggateway.NewNoop()}
		called := false
		r.SetUpstreamDeliverableSeed(func(context.Context, biz.Team) (map[string]any, error) {
			called = true
			return seed, nil
		})
		if got := r.resolveUpstreamDeliverableSeed(context.Background(), biz.Team{ID: "t1"}); got != nil || called {
			t.Fatalf("standalone team must skip seed resolution, got=%v called=%v", got, called)
		}
	})

	t.Run("no DependsOn skips resolution", func(t *testing.T) {
		r := &Runner{lg: loggateway.NewNoop()}
		called := false
		r.SetUpstreamDeliverableSeed(func(context.Context, biz.Team) (map[string]any, error) {
			called = true
			return seed, nil
		})
		if got := r.resolveUpstreamDeliverableSeed(context.Background(), biz.Team{ID: "t2", DagNodeID: "st_1"}); got != nil || called {
			t.Fatalf("dep-less team must skip seed resolution, got=%v called=%v", got, called)
		}
	})

	t.Run("resolver error degrades to nil", func(t *testing.T) {
		r := &Runner{lg: loggateway.NewNoop()}
		r.SetUpstreamDeliverableSeed(func(context.Context, biz.Team) (map[string]any, error) {
			return nil, context.DeadlineExceeded
		})
		if got := r.resolveUpstreamDeliverableSeed(context.Background(), biz.Team{ID: "t2", DagNodeID: "st_1", DependsOn: []string{"st_0"}}); got != nil {
			t.Fatalf("resolver error must degrade to nil, got %v", got)
		}
	})
}

// TestPersistStep_NoMemberCompletedProjection 锚定 2026-07-28 成员终态单写者
// 重设计：persistStep 只落库成员 step，不得发布 member-session completed 事件。
// 「成员产出最终文本」是消息生命周期而非工作结果——成员终态由 service 终态
// outcome pass（outcome 哨兵权威带）唯一裁决，runner 不再投影成功。
func TestPersistStep_NoMemberCompletedProjection(t *testing.T) {
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

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 2, 0, "", time.Time{}, "")

	repo := runner.runWriter.(*stepBusRunWriter)
	if len(repo.steps) != 1 {
		t.Fatalf("steps=%d", len(repo.steps))
	}
	if repo.steps[0].ToolCallCount != 2 {
		t.Fatalf("tool_call_count=%d", repo.steps[0].ToolCallCount)
	}
	// 成员终态事件只能来自 service outcome pass：runner 侧任何
	// MemberSessionUpdatedEvent（completed）都是消息生命周期的越权投影。
	for {
		select {
		case e := <-ch:
			if ev, ok := e.(*biz.MemberSessionUpdatedEvent); ok {
				t.Fatalf("persistStep must not publish member-session completed, got status=%s agent=%s",
					ev.MemberSession.Status, ev.MemberSession.AgentKey)
			}
		default:
			return
		}
	}
}

// TestPersistStep_NodeStartOverride 锚定 2026-08-08 问题4b：graph watch 路径
// 传入 node_start 追踪的真实开始时间时，step 必须落真实窗口——StartedAt 取
// 追踪值、DurationMS 取墙钟耗时，而非 StartedAt≈FinishedAt≈now 的 0ms 假窗口。
func TestPersistStep_NodeStartOverride(t *testing.T) {
	runner := &Runner{runWriter: &stepBusRunWriter{}}
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK, CreatedAt: agent.RFC3339Now()}
	startedAt := time.Now().Add(-3 * time.Second)

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 0, 0, "", startedAt, "")

	repo := runner.runWriter.(*stepBusRunWriter)
	if len(repo.steps) != 1 {
		t.Fatalf("steps=%d", len(repo.steps))
	}
	step := repo.steps[0]
	if step.StartedAt != startedAt.UTC().Format(time.RFC3339) {
		t.Fatalf("StartedAt=%s want tracked %s", step.StartedAt, startedAt.UTC().Format(time.RFC3339))
	}
	if step.DurationMS < 2500 {
		t.Fatalf("DurationMS=%d want >=2500 (3s wall clock minus slack)", step.DurationMS)
	}
	if step.FinishedAt != asst.CreatedAt {
		t.Fatalf("FinishedAt=%s want end evidence %s", step.FinishedAt, asst.CreatedAt)
	}
}

// TestPersistStep_ZeroStartKeepsLegacyFallback 锚定回退路径：无 node_start
// 追踪（standalone / watch 迟到 / anchor fallback）时保持既有行为——
// StartedAt=FinishedAt=asst.CreatedAt、DurationMS=asst.LatencyMS。
func TestPersistStep_ZeroStartKeepsLegacyFallback(t *testing.T) {
	runner := &Runner{runWriter: &stepBusRunWriter{}}
	run := biz.TeamRunRecord{ID: "run-1", SessionID: "sess-1"}
	ag := biz.Agent{ID: "a1", AgentKey: "worker-a", DisplayName: "Worker A"}
	m := MemberDef{Role: "worker"}
	asst := biz.ChatMessage{Role: "assistant", ContentMarkdown: "done", Status: biz.TeamMemberStepStatusOK, CreatedAt: "2026-01-01T00:00:00Z", LatencyMS: 1234}

	runner.persistStep(context.Background(), run, "team-1", 0, m, ag, "hello", asst, "", "", "default", 0, 0, "", time.Time{}, "")

	repo := runner.runWriter.(*stepBusRunWriter)
	if len(repo.steps) != 1 {
		t.Fatalf("steps=%d", len(repo.steps))
	}
	step := repo.steps[0]
	if step.StartedAt != asst.CreatedAt || step.FinishedAt != asst.CreatedAt {
		t.Fatalf("legacy window broken: started=%s finished=%s want %s", step.StartedAt, step.FinishedAt, asst.CreatedAt)
	}
	if step.DurationMS != 1234 {
		t.Fatalf("DurationMS=%d want latency fallback 1234", step.DurationMS)
	}
}

// TestPublishTeamStepActivity_MemberSessionIDMatchesV2Formula guards the
// duplicate-row fix: the runner's MemberSession must derive its ID and
// TeamRunID from the SAME v2 deterministic formulas the spirit_team service
// uses, so both writers converge on one member_sessions_v2 row per member
// per run (upsert by ID). Previously the runner used
// NewSessionActivityID(teamID, agentKey) + v1 run.ID while the service used
// "aranea.member_session.v2:"+teamRunV2ID+":"+agentKey → two rows per member.
func TestPublishTeamStepActivity_MemberSessionIDMatchesV2Formula(t *testing.T) {
	v2Bus := event.NewV2Bus()
	ch, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	runner := &Runner{
		td: rt.TurnDeps{
			Pipeline: rt.EventPipeline{EventBus: v2Bus},
		},
	}
	run := biz.TeamRunRecord{ID: "v1-run-random-uuid", SessionID: "sess-1", SpiritSessionID: "spirit-1"}
	teamID, agentKey := "team-1", "worker-a"

	runner.publishTeamStepActivity(context.Background(), run, teamID, agentKey, "Worker A",
		biz.ActivityEventCreated, biz.ActivityStatusRunning, "member_started", nil)

	teamStageID := string(agent.NewTeamStageActivityID(teamID, ""))
	wantRunID := agent.NewTeamRunV2ID(teamStageID)
	wantMsID := string(agent.NewMemberSessionActivityID(wantRunID, agentKey))

	deadline := 0
	for deadline < 2 {
		select {
		case e := <-ch:
			if ev, ok := e.(*biz.MemberSessionCreatedEvent); ok {
				if ev.MemberSession.ID != wantMsID {
					t.Fatalf("MemberSession.ID=%q want %q (v2 unified formula)", ev.MemberSession.ID, wantMsID)
				}
				if ev.MemberSession.TeamRunID != wantRunID {
					t.Fatalf("MemberSession.TeamRunID=%q want v2 run %q (must not be v1 run.ID %q)",
						ev.MemberSession.TeamRunID, wantRunID, run.ID)
				}
				if ev.MemberSession.TeamStageID != teamStageID {
					t.Fatalf("MemberSession.TeamStageID=%q want %q", ev.MemberSession.TeamStageID, teamStageID)
				}
				return
			}
			deadline++
		default:
			t.Fatal("expected MemberSessionCreatedEvent, got none")
		}
	}
	t.Fatal("expected MemberSessionCreatedEvent, got other events only")
}
