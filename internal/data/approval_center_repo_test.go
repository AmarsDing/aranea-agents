package data

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	bridge "aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// newApprovalMatrixTestDataPG 建 ent schema 后执行迁移 20261323 的真实 SQL
// 文件（raw-SQL 表，非 ent schema——直接跑迁移文件保证测试与生产 DDL 一致）。
func newApprovalMatrixTestDataPG(t *testing.T) *Data {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ddl, err := os.ReadFile("sql/migrations/20261323_approval_risk_matrix.sql")
	if err != nil {
		t.Fatalf("read migration 20261323: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), string(ddl)); err != nil {
		t.Fatalf("apply migration 20261323: %v", err)
	}
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	return d
}

func TestApprovalRiskMatrixRepo_ReplaceAndList(t *testing.T) {
	d := newApprovalMatrixTestDataPG(t)
	r := NewApprovalRiskMatrixRepo(d)
	ctx := context.Background()

	// 空表读出为空（非 nil 错误）。
	rows, err := r.ListRiskMatrix(ctx)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty, got %v", rows)
	}

	// 全量写入：updatedBy/updated_at 由仓储统一落列。
	in := []approval.RiskMatrixRow{
		{MatcherKey: "shell.exec", Risk: approval.RiskHigh, DefaultMode: approval.ModeAsk},
		{MatcherKey: "http.fetch", Risk: approval.RiskMedium, DefaultMode: approval.ModeTrusted, AutoApproveTimes: 5},
		{MatcherKey: "fs.read", Risk: approval.RiskLow, DefaultMode: approval.ModeAuto},
	}
	if err := r.ReplaceRiskMatrix(ctx, in, "42"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	rows, err = r.ListRiskMatrix(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// 按 matcher_key 字典序稳定输出。
	if rows[0].MatcherKey != "fs.read" || rows[1].MatcherKey != "http.fetch" || rows[2].MatcherKey != "shell.exec" {
		t.Fatalf("order wrong: %v", rows)
	}
	for _, row := range rows {
		if row.UpdatedBy != "42" || row.UpdatedAt.IsZero() {
			t.Fatalf("updated_by/at not stamped: %+v", row)
		}
	}
	if rows[1].AutoApproveTimes != 5 || rows[1].DefaultMode != approval.ModeTrusted {
		t.Fatalf("row fields lost: %+v", rows[1])
	}

	// 再次全量替换：旧行消失（DELETE+INSERT 语义）。
	if err := r.ReplaceRiskMatrix(ctx, []approval.RiskMatrixRow{
		{MatcherKey: "only.left", Risk: approval.RiskLow, DefaultMode: approval.ModeNever},
	}, "7"); err != nil {
		t.Fatal(err)
	}
	rows, err = r.ListRiskMatrix(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].MatcherKey != "only.left" || rows[0].UpdatedBy != "7" {
		t.Fatalf("replace-all semantics broken: %+v", rows)
	}
}

// TestCodingTaskRepo_ListPendingReviewAndReviewCAS E 阶段 W1b：审批中心任务
// 验收源 + 决议 CAS（验收 3：并发决议恰一个成功，其余 409）。
func TestCodingTaskRepo_ListPendingReviewAndReviewCAS(t *testing.T) {
	d := newCodingReposTestDataPG(t)
	r := NewCodingTaskRepo(d)
	ctx := context.Background()

	mk := func(workspace, prompt string, status bridge.TaskStatus) *bridge.CodingTask {
		task := &bridge.CodingTask{
			Workspace: workspace, SessionID: "s", AgentID: "a", ProjectID: "p",
			Prompt: prompt, Status: bridge.StatusDispatched,
		}
		if err := r.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
		if status != bridge.StatusDispatched {
			if err := r.UpdateStatus(ctx, task.ID, bridge.StatusDispatched, status, bridge.TaskPatch{}); err != nil {
				t.Fatal(err)
			}
		}
		return task
	}
	doneWS1 := mk("ws-1", "完成任务一", bridge.StatusDone)
	_ = mk("ws-2", "完成任务二", bridge.StatusFailed)
	running := mk("ws-1", "仍在执行", bridge.StatusRunning)

	// 只列终态且未验收的；按租户过滤。
	list, total, err := r.ListPendingReview(ctx, "ws-1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != doneWS1.ID {
		t.Fatalf("ws-1 pending = %v/%d, want only %s", idsOfTasks(list), total, doneWS1.ID)
	}
	list, total, err = r.ListPendingReview(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("all pending = %v/%d, want 2", idsOfTasks(list), total)
	}

	// CAS：首次决议成功，重复决议 409，不存在 404。
	if err := r.UpdateReview(ctx, doneWS1.ID, "approved", "lgtm", "2026-11-09T12:00:00Z"); err != nil {
		t.Fatalf("first review: %v", err)
	}
	err = r.UpdateReview(ctx, doneWS1.ID, "rejected", "changed my mind", "2026-11-09T12:01:00Z")
	if err == nil || apiErrCode(t, err) != apierror.CodeConflict {
		t.Fatalf("second review must conflict, got %v", err)
	}
	err = r.UpdateReview(ctx, "missing", "approved", "", "2026-11-09T12:00:00Z")
	if err == nil || apiErrCode(t, err) != apierror.CodeNotFound {
		t.Fatalf("missing task must be NotFound, got %v", err)
	}

	// 已验收任务从待办消失；运行中任务本就不在。
	list, total, err = r.ListPendingReview(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("after review pending = %v/%d, want 1", idsOfTasks(list), total)
	}
	got, err := r.Get(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReviewStatus != "" {
		t.Fatalf("running task review_status must stay empty, got %q", got.ReviewStatus)
	}
	reviewed, err := r.Get(ctx, doneWS1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.ReviewStatus != "approved" || reviewed.ReviewComment != "lgtm" || reviewed.ReviewedAt == "" {
		t.Fatalf("review fields not persisted: %+v", reviewed)
	}
}

// TestReviewTaskRepo_DecideCAS E 阶段 W1b：ent 仓储决议 CAS（并发决议恰一个
// 成功——第二个以过期 from 进入者拿 ErrConflict）+ creator_id 往返。
func TestReviewTaskRepo_DecideCAS(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	r := NewReviewTaskRepo(d)

	now := time.Now().UTC()
	task, err := r.Create(review.Task{
		ID: uuid.NewString(), ContentItemID: "c1", CreatorID: "42",
		DraftTitle: "t", DraftBody: "b", Status: review.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatorID != "42" {
		t.Fatalf("creator_id not persisted: %+v", got)
	}

	decided, err := r.Decide(task.ID, review.StatusPending, review.StatusApproved, "u7", "lgtm", now)
	if err != nil {
		t.Fatalf("first decide: %v", err)
	}
	if decided.Status != review.StatusApproved || decided.ReviewerID != "u7" || decided.DecidedAt == nil {
		t.Fatalf("decide fields: %+v", decided)
	}
	// 过期 from（并发方视图）→ ErrConflict。
	if _, err := r.Decide(task.ID, review.StatusPending, review.StatusRejected, "u8", "no", now); !errors.Is(err, review.ErrConflict) {
		t.Fatalf("stale decide must yield ErrConflict, got %v", err)
	}
	// 不存在 → ErrNotFound。
	if _, err := r.Decide("missing", review.StatusPending, review.StatusApproved, "u8", "no", now); !errors.Is(err, review.ErrNotFound) {
		t.Fatalf("missing decide must yield ErrNotFound, got %v", err)
	}
}

func idsOfTasks(tasks []*bridge.CodingTask) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}
