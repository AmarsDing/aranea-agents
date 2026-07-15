package biz

import "testing"

func TestDeliveryEventID_Stable(t *testing.T) {
	ev := NewTaskCompletedEvent(Task{ID: "t1", SessionID: "sess-1", Seq: 7})
	got := DeliveryEventID(ev, 7)
	want := "v2:sess-1:7:task.completed:t1"
	if got != want {
		t.Fatalf("DeliveryEventID = %q, want %q", got, want)
	}
	if EventSeq(ev) != 7 {
		t.Fatalf("EventSeq = %d, want 7", EventSeq(ev))
	}
}

func TestSetEventSeq_SystemNotice(t *testing.T) {
	ev := NewSystemNoticeEvent("sess-1", "orchestration_completed", "", nil)
	if EventSeq(ev) != 0 {
		t.Fatalf("initial seq = %d", EventSeq(ev))
	}
	SetEventSeq(ev, 42)
	if EventSeq(ev) != 42 {
		t.Fatalf("after SetEventSeq = %d, want 42", EventSeq(ev))
	}
	if !IsCriticalDeliveryEvent(ev) {
		t.Fatal("orchestration_completed should be critical")
	}
}
