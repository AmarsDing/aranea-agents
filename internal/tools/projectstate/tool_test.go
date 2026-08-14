package projectstate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

func TestUpdateProjectStateTool_Declaration(t *testing.T) {
	tl := NewUpdateProjectStateTool()
	decl := tl.Declaration()
	if decl.Name != "update_project_state" {
		t.Fatalf("name=%q want update_project_state", decl.Name)
	}
	if decl.InputSchema == nil || decl.InputSchema.Type != "object" {
		t.Fatalf("input schema missing or not object: %#v", decl.InputSchema)
	}
	if decl.OutputSchema == nil {
		t.Fatal("output schema should be non-nil")
	}
}

func TestUpdateProjectStateTool_Call_RequiresAtLeastOneField(t *testing.T) {
	tl := NewUpdateProjectStateTool()
	if _, err := tl.Call(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("empty update must be rejected")
	}
}

func TestUpdateProjectStateTool_Call_RollsChangeAndStateDeltaRoundTrip(t *testing.T) {
	inv := agent.NewInvocation()
	inv.RunOptions.RuntimeState = map[string]any{}
	ctx := agent.NewInvocationContext(context.Background(), inv)

	tl := NewUpdateProjectStateTool()
	out, err := tl.Call(ctx, []byte(`{"change":{"actor":"researcher","summary":"完成了竞品调研"},"milestone":"调研收尾"}`))
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(updateProjectStateOutput)
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if !o.Written || len(o.Updated) != 2 {
		t.Fatalf("output=%+v, want written + 2 updated fields", o)
	}
	// read-your-writes：同节点 RuntimeState 立即可见。
	raw, found := inv.RunOptions.RuntimeState[biz.ProjectStateKey]
	if !found {
		t.Fatal("RuntimeState must carry project_state after Call")
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("RuntimeState project_state type=%T, want map", raw)
	}
	ps := biz.TeamProjectStateFromMap(m)
	if len(ps.RecentChanges) != 1 || ps.RecentChanges[0].Actor != "researcher" {
		t.Fatalf("recent=%+v", ps.RecentChanges)
	}
	if len(ps.Milestones) != 1 || ps.Milestones[0] != "调研收尾" {
		t.Fatalf("milestones=%+v", ps.Milestones)
	}

	// StateDelta 必须携完整 map（MergeReducer 顶层 key 合并 = 整值替换）。
	resultJSON, _ := json.Marshal(o)
	delta := tl.StateDelta("tc-1", nil, resultJSON)
	b, ok := delta[biz.ProjectStateKey]
	if !ok || len(b) == 0 {
		t.Fatalf("delta must carry %q, got keys %v", biz.ProjectStateKey, delta)
	}
	var roundTripped map[string]any
	if err := json.Unmarshal(b, &roundTripped); err != nil {
		t.Fatal(err)
	}
	ps2 := biz.TeamProjectStateFromMap(roundTripped)
	if len(ps2.RecentChanges) != 1 || len(ps2.Milestones) != 1 {
		t.Fatalf("round trip lost entries: %+v", ps2)
	}
	// 空 toolCallID / Written=false 必须返回 nil。
	if got := tl.StateDelta("", nil, resultJSON); got != nil {
		t.Fatalf("empty toolCallID must yield nil delta, got %v", got)
	}
}

func TestUpdateProjectStateTool_Call_ReadModifyWriteMergesExisting(t *testing.T) {
	// 已有状态（上一成员写入）必须保留——读-改-写而非覆盖。
	existing := biz.TeamProjectState{}
	existing.RollChange("leader", "拆解了任务")
	existingBytes, _ := json.Marshal(existing.ToMap())

	sess := session.NewSession("app", "user", "sess")
	sess.SetState(biz.ProjectStateKey, existingBytes)
	inv := agent.NewInvocation()
	inv.Session = sess
	ctx := agent.NewInvocationContext(context.Background(), inv)

	tl := NewUpdateProjectStateTool()
	out, err := tl.Call(ctx, []byte(`{"decision_digest":"采用 DAG 编排"}`))
	if err != nil {
		t.Fatal(err)
	}
	ps := biz.TeamProjectStateFromMap(out.(updateProjectStateOutput).Data)
	if len(ps.RecentChanges) != 1 || ps.RecentChanges[0].Summary != "拆解了任务" {
		t.Fatalf("existing recent_changes must survive, got %+v", ps.RecentChanges)
	}
	if ps.DecisionDigest != "采用 DAG 编排" {
		t.Fatalf("digest=%q", ps.DecisionDigest)
	}
}

func TestUpdateProjectStateTool_Call_CapsEnforced(t *testing.T) {
	inv := agent.NewInvocation()
	inv.RunOptions.RuntimeState = map[string]any{}
	ctx := agent.NewInvocationContext(context.Background(), inv)
	tl := NewUpdateProjectStateTool()

	long := strings.Repeat("长", 200)
	for i := 0; i < 12; i++ {
		if _, err := tl.Call(ctx, []byte(`{"change":{"summary":"`+long+`"}}`)); err != nil {
			t.Fatal(err)
		}
	}
	raw := inv.RunOptions.RuntimeState[biz.ProjectStateKey].(map[string]any)
	ps := biz.TeamProjectStateFromMap(raw)
	if len(ps.RecentChanges) != biz.ProjectStateMaxRecent {
		t.Fatalf("recent=%d, want cap %d", len(ps.RecentChanges), biz.ProjectStateMaxRecent)
	}
	if got := len([]rune(ps.RecentChanges[0].Summary)); got > 120 {
		t.Fatalf("summary runes=%d, want ≤120", got)
	}
}
