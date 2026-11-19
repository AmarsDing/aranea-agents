package review

import (
	"errors"
	"testing"
	"time"
)

func TestApproveGateAndRejectNote(t *testing.T) {
	uc := NewUsecase(NewMemoryRepo())
	task, err := uc.Enqueue(Task{ContentItemID: "c1", DraftTitle: "绝对好货", DraftBody: "买它"})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.MachineFlags) == 0 {
		t.Fatal("expected soft flags")
	}
	if err := MustBeApproved(task); err == nil {
		t.Fatal("pending must block publish")
	}
	if _, err := uc.Reject(task.ID, "u1", ""); err == nil {
		t.Fatal("reject without note")
	}
	ok, err := uc.Approve(task.ID, "u1", "lgtm")
	if err != nil {
		t.Fatal(err)
	}
	if err := MustBeApproved(ok); err != nil {
		t.Fatal(err)
	}
}

// TestSelfApprovalForbidden E 阶段 W1b（A-2 安全红线）：创建者与决议者
// 均非空且相同 → ErrSelfApproval；机器入队任务（CreatorID 空）不受影响。
func TestSelfApprovalForbidden(t *testing.T) {
	uc := NewUsecase(NewMemoryRepo())
	human, err := uc.Enqueue(Task{ContentItemID: "c1", DraftTitle: "t", DraftBody: "b", CreatorID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Approve(human.ID, "42", "self"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self-approval must fail with ErrSelfApproval, got %v", err)
	}
	if _, err := uc.Reject(human.ID, "42", "self-reject"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self-reject must also fail with ErrSelfApproval, got %v", err)
	}
	// 他审通过。
	if _, err := uc.Approve(human.ID, "7", "lgtm"); err != nil {
		t.Fatalf("other reviewer must pass, got %v", err)
	}
	// 机器入队（CreatorID 空）：任何 reviewer 均可决议。
	machine, err := uc.Enqueue(Task{ContentItemID: "c2", DraftTitle: "t", DraftBody: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Approve(machine.ID, "42", "ok"); err != nil {
		t.Fatalf("machine-enqueued task must not trip self-approval, got %v", err)
	}
}

// TestDecideSequentialConflictAndReviewer 决议后状态推进：二次决议走状态机
// 拦截（ErrIllegalTransition）；首次决议落 reviewer/note/decided_at。
func TestDecideSequentialConflictAndReviewer(t *testing.T) {
	uc := NewUsecase(NewMemoryRepo())
	task, err := uc.Enqueue(Task{ContentItemID: "c1", DraftTitle: "t", DraftBody: "b"})
	if err != nil {
		t.Fatal(err)
	}
	done, err := uc.Approve(task.ID, "u9", "ship it")
	if err != nil {
		t.Fatal(err)
	}
	if done.ReviewerID != "u9" || done.DecisionNote != "ship it" || done.DecidedAt == nil {
		t.Fatalf("reviewer/note/decided_at not recorded: %+v", done)
	}
	if _, err := uc.Approve(task.ID, "u10", "again"); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("second decide must fail with ErrIllegalTransition, got %v", err)
	}
}

// TestRepoDecideCAS 仓储层竞态防线：绕过用例层直接以过期 from 调 Decide →
// ErrConflict（Get→Decide 之间状态被并发推进的场景）。
func TestRepoDecideCAS(t *testing.T) {
	repo := NewMemoryRepo()
	task, err := repo.Create(Task{ID: "t1", ContentItemID: "c1", DraftTitle: "t", DraftBody: "b", Status: StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Decide(task.ID, StatusApproved, StatusRejected, "u1", "x", time.Now().UTC()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale from-state must yield ErrConflict, got %v", err)
	}
	if _, err := repo.Decide("missing", StatusPending, StatusApproved, "u1", "x", time.Now().UTC()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing task must fail with ErrNotFound, got %v", err)
	}
	if _, err := repo.Decide(task.ID, StatusPending, StatusApproved, "u1", "x", time.Now().UTC()); err != nil {
		t.Fatalf("valid decide must pass, got %v", err)
	}
}
