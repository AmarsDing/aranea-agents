package biz

import "testing"

func TestDeliveryEventID_Stable(t *testing.T) {
	ev := NewTaskCompletedEvent(Task{ID: "t1", SessionID: "sess-1", Seq: 7})
	got := DeliveryEventID(ev, 7)
	want := "v2:sess-1:7:task.completed:t1"
	if got != want {
		t.Fatalf("DeliveryEventID = %q, want %q", got, want)
	}
}

func TestDeliverySeqCarrier_EntityEvents(t *testing.T) {
	// H1 (2026-10-18): critical entity events carry a Sequencer-assigned
	// delivery seq via the embedded carrier; the domain Task.Seq is untouched.
	ev := NewTaskCompletedEvent(Task{ID: "t1", SessionID: "sess-1", Seq: 7})
	de, ok := interface{}(ev).(DeliverySeqEvent)
	if !ok {
		t.Fatal("TaskCompletedEvent must implement DeliverySeqEvent")
	}
	if got := de.DeliverySeqValue(); got != 0 {
		t.Fatalf("initial delivery seq = %d, want 0", got)
	}
	de.SetDeliverySeqValue(42)
	if got := de.DeliverySeqValue(); got != 42 {
		t.Fatalf("after SetDeliverySeqValue = %d, want 42", got)
	}
	if ev.Task.Seq != 7 {
		t.Fatalf("domain Task.Seq mutated to %d, want 7", ev.Task.Seq)
	}
}

func TestDeliverySeq_SystemNotice(t *testing.T) {
	ev := NewSystemNoticeEvent("sess-1", "orchestration_completed", "", nil)
	de, ok := interface{}(ev).(DeliverySeqEvent)
	if !ok {
		t.Fatal("SystemNoticeEvent must implement DeliverySeqEvent")
	}
	if got := de.DeliverySeqValue(); got != 0 {
		t.Fatalf("initial seq = %d, want 0", got)
	}
	de.SetDeliverySeqValue(42)
	if ev.Seq != 42 || de.DeliverySeqValue() != 42 {
		t.Fatalf("after SetDeliverySeqValue: Seq=%d DeliverySeqValue=%d, want 42", ev.Seq, de.DeliverySeqValue())
	}
	if !IsCriticalDeliveryEvent(ev) {
		t.Fatal("orchestration_completed should be critical")
	}
}
