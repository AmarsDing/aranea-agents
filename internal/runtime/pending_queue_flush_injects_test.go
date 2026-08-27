package runtime

import "testing"

// N2 满队 inject 死锁（session-eval-20260827 S12 / C5-③）：FlushLeadingInjects
// 仅在整队全 inject 时原子清空并返回条目；含 followup 或空队列不动。
func TestPendingMessageQueue_FlushLeadingInjects(t *testing.T) {
	t.Run("pure inject queue flushed", func(t *testing.T) {
		q := NewPendingMessageQueue()
		q.EnqueueInject("sess-i", "ctx-a")
		q.EnqueueInject("sess-i", "ctx-b")
		flushed := q.FlushLeadingInjects("sess-i")
		if len(flushed) != 2 || flushed[0].Content != "ctx-a" || flushed[1].Content != "ctx-b" {
			t.Fatalf("flushed = %+v", flushed)
		}
		if got := q.List("sess-i"); len(got) != 0 {
			t.Fatalf("queue must be empty after flush, got %+v", got)
		}
	})
	t.Run("mixed queue untouched", func(t *testing.T) {
		q := NewPendingMessageQueue()
		q.EnqueueInject("sess-m", "ctx-a")
		q.Enqueue("sess-m", "real question")
		if flushed := q.FlushLeadingInjects("sess-m"); flushed != nil {
			t.Fatalf("mixed queue must not flush, got %+v", flushed)
		}
		if got := q.List("sess-m"); len(got) != 2 {
			t.Fatalf("mixed queue must stay intact, got %+v", got)
		}
	})
	t.Run("followup-only queue untouched", func(t *testing.T) {
		q := NewPendingMessageQueue()
		q.Enqueue("sess-f", "q1")
		if flushed := q.FlushLeadingInjects("sess-f"); flushed != nil {
			t.Fatalf("followup queue must not flush, got %+v", flushed)
		}
	})
	t.Run("empty queue returns nil", func(t *testing.T) {
		q := NewPendingMessageQueue()
		if flushed := q.FlushLeadingInjects("sess-none"); flushed != nil {
			t.Fatalf("empty queue must return nil, got %+v", flushed)
		}
	})
	// 死锁剧本钉住：32 条 inject 占满 → 新 followup 容量拒绝 → flush 腾出
	// 全部空位 → 合并消息入队成功。
	t.Run("full inject deadlock rescue flow", func(t *testing.T) {
		q := NewPendingMessageQueue()
		for i := 0; i < MaxPendingPerSession; i++ {
			if id := q.EnqueueInject("sess-d", "ctx"); id == "" {
				t.Fatalf("inject %d should be accepted", i)
			}
		}
		if pid := q.Enqueue("sess-d", "new question"); pid != "" {
			t.Fatal("full queue must reject plain enqueue")
		}
		flushed := q.FlushLeadingInjects("sess-d")
		if len(flushed) != MaxPendingPerSession {
			t.Fatalf("flushed = %d, want %d", len(flushed), MaxPendingPerSession)
		}
		if pid := q.Enqueue("sess-d", "merged question"); pid == "" {
			t.Fatal("enqueue after flush must succeed")
		}
	})
}
