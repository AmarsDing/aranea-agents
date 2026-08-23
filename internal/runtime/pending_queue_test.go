package runtime

import (
	"context"
	"path/filepath"
	"testing"
)

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
	for i := 0; i < MaxPendingPerSession; i++ {
		if q.Enqueue("sess-3", "msg") == "" {
			t.Fatalf("enqueue %d failed", i)
		}
	}
	if q.Enqueue("sess-3", "overflow") != "" {
		t.Fatal("expected enqueue cap")
	}
}

func TestPendingMessageQueue_WriteThroughSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	q1 := NewPendingMessageQueueWithDir(dir)
	id := q1.Enqueue("sess-dur", "persist-me")
	if id == "" {
		t.Fatal("enqueue failed")
	}
	q1.Close()

	q2 := NewPendingMessageQueueWithDir(dir)
	list := q2.List("sess-dur")
	if len(list) != 1 || list[0].Content != "persist-me" || list[0].ID != id {
		t.Fatalf("after restart expected persisted message, got %+v (file=%s)", list, filepath.Join(dir, pendingSnapshotFile))
	}

	head, ok := q2.Dequeue("sess-dur")
	if !ok || head.ID != id {
		t.Fatalf("dequeue: %+v ok=%v", head, ok)
	}
	q2.Close()

	q3 := NewPendingMessageQueueWithDir(dir)
	if len(q3.List("sess-dur")) != 0 {
		t.Fatalf("after dequeue+restart expected empty, got %+v", q3.List("sess-dur"))
	}
	q3.Close()
}

func TestPendingMessageQueue_UpdateWriteThrough(t *testing.T) {
	dir := t.TempDir()
	q1 := NewPendingMessageQueueWithDir(dir)
	id := q1.Enqueue("sess-upd", "old")
	if !q1.Update("sess-upd", id, "new") {
		t.Fatal("update failed")
	}
	if err := q1.SetPriority("sess-upd", id, 1); err != nil {
		t.Fatal(err)
	}
	q1.Close()

	q2 := NewPendingMessageQueueWithDir(dir)
	list := q2.List("sess-upd")
	if len(list) != 1 || list[0].Content != "new" || list[0].Priority != 1 {
		t.Fatalf("update/priority must persist: %+v", list)
	}
	q2.Close()
}

type memPendingStore struct {
	m map[string][]PendingMessage
}

func (s *memPendingStore) LoadAll(_ context.Context) (map[string][]PendingMessage, error) {
	out := make(map[string][]PendingMessage, len(s.m))
	for k, v := range s.m {
		out[k] = append([]PendingMessage(nil), v...)
	}
	return out, nil
}

func (s *memPendingStore) ReplaceAll(_ context.Context, queues map[string][]PendingMessage) error {
	s.m = make(map[string][]PendingMessage, len(queues))
	for k, v := range queues {
		s.m[k] = append([]PendingMessage(nil), v...)
	}
	return nil
}

func TestPendingMessageQueue_StoreRoundTrip(t *testing.T) {
	store := &memPendingStore{}
	q1 := NewPendingMessageQueue()
	q1.SetStore(store)
	id := q1.Enqueue("sess-store", "from-store")
	if id == "" {
		t.Fatal("enqueue failed")
	}
	q1.Close()

	q2 := NewPendingMessageQueue()
	q2.SetStore(store)
	list := q2.List("sess-store")
	if len(list) != 1 || list[0].Content != "from-store" || list[0].ID != id {
		t.Fatalf("store replay: %+v", list)
	}
}

func TestPendingMessageQueue_Peek(t *testing.T) {
	q := NewPendingMessageQueue()

	// Empty queue: Peek returns false.
	if _, ok := q.Peek("sess-peek"); ok {
		t.Fatal("expected Peek to return false on empty queue")
	}

	id1 := q.Enqueue("sess-peek", "first")
	id2 := q.Enqueue("sess-peek", "second")

	// Peek returns head without removing.
	head, ok := q.Peek("sess-peek")
	if !ok || head.ID != id1 || head.Content != "first" {
		t.Fatalf("unexpected peek head: %+v ok=%v", head, ok)
	}

	// Queue still has both entries.
	if len(q.List("sess-peek")) != 2 {
		t.Fatal("Peek should not remove entries")
	}

	// Peek again returns same head.
	head, ok = q.Peek("sess-peek")
	if !ok || head.ID != id1 {
		t.Fatalf("unexpected second peek head: %+v ok=%v", head, ok)
	}

	// Dequeue returns the same head we peeked.
	dequeued, ok := q.Dequeue("sess-peek")
	if !ok || dequeued.ID != id1 {
		t.Fatalf("unexpected dequeue head: %+v ok=%v", dequeued, ok)
	}

	// Peek now returns the second entry.
	head, ok = q.Peek("sess-peek")
	if !ok || head.ID != id2 || head.Content != "second" {
		t.Fatalf("unexpected peek after dequeue: %+v ok=%v", head, ok)
	}
}
