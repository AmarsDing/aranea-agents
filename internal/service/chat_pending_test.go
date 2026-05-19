package service

import "testing"

func TestPendingMessageQueue_EnqueueDequeueFIFO(t *testing.T) {
	q := NewPendingMessageQueue()
	id1 := q.Enqueue("sess-1", "first")
	id2 := q.Enqueue("sess-1", "second")
	if id1 == "" || id2 == "" {
		t.Fatal("expected enqueue ids")
	}
	head, ok := q.Dequeue("sess-1")
	if !ok || head.Content != "first" || head.ID != id1 {
		t.Fatalf("unexpected head: %+v ok=%v", head, ok)
	}
	head, ok = q.Dequeue("sess-1")
	if !ok || head.Content != "second" {
		t.Fatalf("unexpected second: %+v ok=%v", head, ok)
	}
	if _, ok = q.Dequeue("sess-1"); ok {
		t.Fatal("expected empty queue")
	}
}

func TestPendingMessageQueue_RemoveAndUpdate(t *testing.T) {
	q := NewPendingMessageQueue()
	id := q.Enqueue("sess-2", "hello")
	if !q.Update("sess-2", id, "updated") {
		t.Fatal("update failed")
	}
	list := q.List("sess-2")
	if len(list) != 1 || list[0].Content != "updated" {
		t.Fatalf("list: %+v", list)
	}
	if !q.Remove("sess-2", id) {
		t.Fatal("remove failed")
	}
	if len(q.List("sess-2")) != 0 {
		t.Fatal("expected empty after remove")
	}
}

func TestPendingMessageQueue_MaxPerSession(t *testing.T) {
	q := NewPendingMessageQueue()
	for i := 0; i < maxPendingPerSession; i++ {
		if q.Enqueue("sess-3", "msg") == "" {
			t.Fatalf("enqueue %d failed", i)
		}
	}
	if q.Enqueue("sess-3", "overflow") != "" {
		t.Fatal("expected enqueue cap")
	}
}
