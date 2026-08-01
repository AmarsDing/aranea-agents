package team

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// 终态收敛与步骤落库测试（F-B）：finalizeTeamRun 事件、node_end → steps_json、
// graph_executions 终态收敛。从 team_graph_run_coordinator_test.go 拆出以控制
// 单文件行数（AS-COG-01）。

func TestTeamGraphRunCoordinator_finalizeTeamRun(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	v2Bus := event.NewV2Bus()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, v2Bus, nil, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}

	outCh, unsub := v2Bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	coord.finalizeTeamRun(ctx, coord.session("exec-1"), false, "")

	select {
	case e := <-outCh:
		switch ev := e.(type) {
		case *biz.TeamStageCompletedEvent:
			if ev.TeamStage.Status != biz.TeamStageStatusCompleted {
				t.Fatalf("status=%s want=%s", ev.TeamStage.Status, biz.TeamStageStatusCompleted)
			}
		case *biz.SystemNoticeEvent:
			// notice may arrive before or after TeamStageCompleted depending on fan-out order
			if ev.NoticeType != "team_stage_completed" && ev.NoticeType != "team_run_failed" {
				t.Fatalf("unexpected notice type %q", ev.NoticeType)
			}
		default:
			t.Fatalf("expected TeamStageCompletedEvent or SystemNoticeEvent, got %T", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for finalize event")
	}
	got, err := repo.GetTeamRunByID(ctx, "run-1")
	if err != nil || got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run=%+v err=%v", got, err)
	}
}

// F-B：graph watch 收到 node_end 时必须把节点步骤写入 graph_executions.steps_json
// （team 路径无 consumeRuntimeEvents 消费者），且只写本 execution 的步骤。
func TestTeamGraphRunCoordinator_nodeEndRecordsGraphStep(t *testing.T) {
	backend := newCoordTestBackend()
	defJSON := `{"version":2,"mode":"sequential","members":[{"agent_id":"a1","name":"Agent1","enabled":true,"sort_order":1}]}`
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning, DefinitionSnapshotJSON: defJSON},
	}}
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "member-1", Type: "agent", AgentName: "a1"}}}, nil, nil, nil)
	ctx := context.Background()
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	sess := coord.session("exec-1")
	if sess == nil || sess.obsStore == nil {
		t.Fatal("session obsStore not built")
	}
	nodeEnd := biz.NewSystemNoticeEvent("sess-1", "node_end", "", map[string]any{
		"activity_kind":  string(biz.ActivityKindGraphStage),
		"activity_event": string(biz.ActivityEventCompleted),
		"execution_id":   "exec-1",
		"node_id":        "member-1",
		"step_number":    1,
	})
	coord.handleGraphWatchNotice(ctx, sess, nodeEnd, graphWatchStepsOnly)
	exec, err := backend.uc.GetExecution(ctx, "exec-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Steps) != 1 || exec.Steps[0].NodeID != "member-1" || exec.Steps[0].Status != "completed" || exec.Steps[0].StepIndex != 1 {
		t.Fatalf("steps=%+v", exec.Steps)
	}
	// 其他 execution 的通知不得串写。
	other := biz.NewSystemNoticeEvent("sess-1", "node_end", "", map[string]any{
		"activity_kind":  string(biz.ActivityKindGraphStage),
		"activity_event": string(biz.ActivityEventCompleted),
		"execution_id":   "exec-other",
		"node_id":        "member-1",
		"step_number":    2,
	})
	coord.handleGraphWatchNotice(ctx, sess, other, graphWatchStepsOnly)
	exec, _ = backend.uc.GetExecution(ctx, "exec-1")
	if len(exec.Steps) != 1 {
		t.Fatalf("cross-exec contamination: steps=%+v", exec.Steps)
	}
}

// F-B：team run 终态时 graph_executions 必须收敛（status + finished_at），
// 成功/失败两条路径都要覆盖。
func TestTeamGraphRunCoordinator_finalizeTeamRun_convergesGraphExecution(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning},
	}}
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "member-1", Type: "agent", AgentName: "a1"}}}, nil, nil, nil)
	ctx := context.Background()
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	coord.finalizeTeamRun(ctx, coord.session("exec-1"), false, "")
	exec, err := backend.uc.GetExecution(ctx, "exec-1")
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != string(biz.GraphExecCompleted) || exec.FinishedAt == nil {
		t.Fatalf("exec=%+v", exec)
	}

	// 失败路径：failed + error_message。
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-2", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	// run-1 已被上一个 finalize 置为 success；重置为 running 供失败分支。
	repo.runs["run-1"] = biz.TeamRunRecord{ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning}
	coord.finalizeTeamRun(ctx, coord.session("exec-2"), true, "node exploded")
	exec2, err := backend.uc.GetExecution(ctx, "exec-2")
	if err != nil {
		t.Fatal(err)
	}
	if exec2.Status != string(biz.GraphExecFailed) || exec2.ErrorMessage != "node exploded" {
		t.Fatalf("exec2=%+v", exec2)
	}
}
