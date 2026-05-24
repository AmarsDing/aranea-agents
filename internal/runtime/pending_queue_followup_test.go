package runtime

import "testing"

func TestPendingMessageQueue_EnqueueFollowupMerges(t *testing.T) {
	q := NewPendingMessageQueue()
	id1 := q.Enqueue("sess-f", "first")
	if id1 == "" {
		t.Fatal("expected first id")
	}
	id2 := q.EnqueueFollowup("sess-f", "second", "\n")
	if id2 != id1 {
		t.Fatalf("followup should merge into same entry: id1=%s id2=%s", id1, id2)
	}
	list := q.List("sess-f")
	if len(list) != 1 || list[0].Content != "first\nsecond" {
		t.Fatalf("merged content: %+v", list)
	}
	id3 := q.EnqueueFollowup("sess-f", "third", "\n")
	if id3 != id1 {
		t.Fatalf("third merge should reuse id: %s", id3)
	}
	list = q.List("sess-f")
	if list[0].Content != "first\nsecond\nthird" {
		t.Fatalf("content after third merge: %q", list[0].Content)
	}
	head, ok := q.Dequeue("sess-f")
	if !ok || head.Content != "first\nsecond\nthird" {
		t.Fatalf("dequeue: %+v ok=%v", head, ok)
	}
}
