package data_test

import (
	"context"
	"testing"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func setupComputerUseAuditRepo(t *testing.T) *data.ComputerUseAuditRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	d := &data.Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return data.NewComputerUseAuditRepo(d, loggateway.NewNoop())
}

func TestComputerUseAuditRepo_RecordAndList(t *testing.T) {
	repo := setupComputerUseAuditRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	steps := []bizcu.AuditEntry{
		{
			SessionID: "sess-1", AgentKey: "agent-a", Index: 1,
			Target: "保存菜单", Path: bizcu.PathA11y, Action: bizcu.ActionInvoke,
			Params: map[string]any{"ref": "g1.e0"}, Result: bizcu.StepOK,
			DurationMs: 42, ConfirmedBy: "user-1", Danger: true, CreatedAt: now,
		},
		{
			SessionID: "sess-1", AgentKey: "agent-a", Index: 2,
			Target: "输入文本", Path: bizcu.PathVision, Action: bizcu.ActionTypeText,
			Params: map[string]any{"text": "hello"}, Result: bizcu.StepFailed,
			Error: "元素不可用", DurationMs: 1300, CreatedAt: now.Add(time.Second),
		},
		// 另一会话：不应混入。
		{SessionID: "sess-2", AgentKey: "agent-b", Index: 1, Target: "x", Action: bizcu.ActionClick, Result: bizcu.StepOK, CreatedAt: now},
	}
	for _, s := range steps {
		if err := repo.RecordStep(ctx, s); err != nil {
			t.Fatalf("RecordStep: %v", err)
		}
	}

	got, err := repo.ListSteps(ctx, "sess-1")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	// 按 step_index 升序。
	if got[0].Index != 1 || got[1].Index != 2 {
		t.Fatalf("排序错误: %d, %d", got[0].Index, got[1].Index)
	}
	first := got[0]
	if first.ID == 0 {
		t.Fatalf("落库后 ID 应由 DB 生成")
	}
	if first.Target != "保存菜单" || first.Path != bizcu.PathA11y || first.Action != bizcu.ActionInvoke {
		t.Fatalf("字段回读错误: %+v", first)
	}
	if first.Params["ref"] != "g1.e0" {
		t.Fatalf("params JSON 回读错误: %+v", first.Params)
	}
	if !first.Danger || first.ConfirmedBy != "user-1" || first.DurationMs != 42 {
		t.Fatalf("danger/confirmed_by/duration 回读错误: %+v", first)
	}
	if got[1].Error != "元素不可用" || got[1].Result != bizcu.StepFailed {
		t.Fatalf("error/result 回读错误: %+v", got[1])
	}
}

func TestComputerUseAuditRepo_ListEmpty(t *testing.T) {
	repo := setupComputerUseAuditRepo(t)
	got, err := repo.ListSteps(context.Background(), "no-such-session")
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空会话应返回空列表, got %d", len(got))
	}
}

func TestComputerUseAuditRepo_NilParams(t *testing.T) {
	repo := setupComputerUseAuditRepo(t)
	ctx := context.Background()
	s := bizcu.AuditEntry{
		SessionID: "sess-nil", AgentKey: "a", Index: 1,
		Target: "t", Action: bizcu.ActionKey, Result: bizcu.StepDryRun,
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.RecordStep(ctx, s); err != nil {
		t.Fatalf("RecordStep nil params: %v", err)
	}
	got, err := repo.ListSteps(ctx, "sess-nil")
	if err != nil || len(got) != 1 {
		t.Fatalf("ListSteps: %v len=%d", err, len(got))
	}
	if got[0].Result != bizcu.StepDryRun {
		t.Fatalf("result=%q", got[0].Result)
	}
}
