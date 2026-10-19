package review

import "testing"

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
