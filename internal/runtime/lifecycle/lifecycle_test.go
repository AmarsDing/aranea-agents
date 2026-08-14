package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ManagedMap 测试

func TestManagedMap_StoreAndLoad(t *testing.T) {
	m := NewManagedMap[string, int](0)
	defer m.Close()

	m.Store("a", 1)
	v, ok := m.Load("a")
	if !ok || v != 1 {
		t.Fatalf("Load = (%v, %v), want (1, true)", v, ok)
	}

	_, ok = m.Load("notexist")
	if ok {
		t.Fatal("Load notexist should return false")
	}
}

func TestManagedMap_LoadOrStore(t *testing.T) {
	m := NewManagedMap[string, int](0)
	defer m.Close()

	// 首次存储
	v, loaded := m.LoadOrStore("a", 1)
	if loaded {
		t.Fatal("first LoadOrStore should return loaded=false")
	}
	if v != 1 {
		t.Fatalf("v = %v, want 1", v)
	}

	// 再次存储，应返回现有值
	v, loaded = m.LoadOrStore("a", 2)
	if !loaded {
		t.Fatal("second LoadOrStore should return loaded=true")
	}
	if v != 1 {
		t.Fatalf("v = %v, want 1", v)
	}
}

func TestManagedMap_UpdateOrStore(t *testing.T) {
	m := NewManagedMap[string, int](0)
	defer m.Close()

	// 首次：无现有值
	m.UpdateOrStore("counter", func(existing int, ok bool) int {
		if ok {
			t.Fatal("first UpdateOrStore should have ok=false")
		}
		return 1
	})

	v, ok := m.Load("counter")
	if !ok || v != 1 {
		t.Fatalf("after first UpdateOrStore, Load = (%v, %v), want (1, true)", v, ok)
	}

	// 再次：基于现有值更新
	m.UpdateOrStore("counter", func(existing int, ok bool) int {
		if !ok || existing != 1 {
			t.Fatalf("second UpdateOrStore should have ok=true, existing=1, got (%v, %v)", existing, ok)
		}
		return existing + 10
	})

	v, ok = m.Load("counter")
	if !ok || v != 11 {
		t.Fatalf("after second UpdateOrStore, Load = (%v, %v), want (11, true)", v, ok)
	}
}

func TestManagedMap_ConcurrentUpdateOrStore(t *testing.T) {
	// 验证 UpdateOrStore 的原子性：1000 个 goroutine 并发递增，结果应为 1000
	m := NewManagedMap[string, int](0)
	defer m.Close()

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.UpdateOrStore("counter", func(existing int, ok bool) int {
				return existing + 1
			})
		}()
	}
	wg.Wait()

	v, ok := m.Load("counter")
	if !ok {
		t.Fatal("counter should exist")
	}
	if v != 1000 {
		t.Fatalf("counter = %v, want 1000 (TOCTOU race detected)", v)
	}
}

func TestManagedMap_TTL(t *testing.T) {
	// 使用短 TTL 验证过期机制
	m := NewManagedMap[string, int](100 * time.Millisecond)
	defer m.Close()

	m.Store("temp", 1)
	v, ok := m.Load("temp")
	if !ok || v != 1 {
		t.Fatalf("immediate Load = (%v, %v), want (1, true)", v, ok)
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	_, ok = m.Load("temp")
	if ok {
		t.Fatal("Load after TTL should return false")
	}
}

func TestManagedMap_LoadAndDelete(t *testing.T) {
	m := NewManagedMap[string, int](0)
	defer m.Close()

	m.Store("a", 1)
	v, ok := m.LoadAndDelete("a")
	if !ok || v != 1 {
		t.Fatalf("LoadAndDelete = (%v, %v), want (1, true)", v, ok)
	}

	_, ok = m.Load("a")
	if ok {
		t.Fatal("Load after LoadAndDelete should return false")
	}
}

func TestManagedMap_Range(t *testing.T) {
	m := NewManagedMap[string, int](0)
	defer m.Close()

	m.Store("a", 1)
	m.Store("b", 2)
	m.Store("c", 3)

	var keys []string
	m.Range(func(k string, v int) bool {
		keys = append(keys, k)
		return true
	})

	if len(keys) != 3 {
		t.Fatalf("Range collected %d keys, want 3", len(keys))
	}
}

// GoroutinePool 测试已随 goroutine_pool.go 一并删除（TEST_ONLY 僵尸清理）。

// LifecycleManager 测试

type mockCloser struct {
	closed int32
	err    error
}

func (m *mockCloser) Close() error {
	atomic.AddInt32(&m.closed, 1)
	return m.err
}

func TestLifecycleManager_LIFOOrder(t *testing.T) {
	mgr := NewLifecycleManager(nil)

	var closeOrder []int
	var mu sync.Mutex

	c1 := &callbackCloser{fn: func() error { mu.Lock(); closeOrder = append(closeOrder, 1); mu.Unlock(); return nil }}
	c2 := &callbackCloser{fn: func() error { mu.Lock(); closeOrder = append(closeOrder, 2); mu.Unlock(); return nil }}
	c3 := &callbackCloser{fn: func() error { mu.Lock(); closeOrder = append(closeOrder, 3); mu.Unlock(); return nil }}

	mgr.Register("c1", c1)
	mgr.Register("c2", c2)
	mgr.Register("c3", c3)

	mgr.Close()

	mu.Lock()
	defer mu.Unlock()
	// LIFO 顺序：3, 2, 1
	if len(closeOrder) != 3 || closeOrder[0] != 3 || closeOrder[1] != 2 || closeOrder[2] != 1 {
		t.Fatalf("close order = %v, want [3 2 1]", closeOrder)
	}
}

func TestLifecycleManager_CloseIdempotent(t *testing.T) {
	mgr := NewLifecycleManager(nil)
	c := &mockCloser{}
	mgr.Register("c", c)

	mgr.Close()
	mgr.Close() // 再次调用应无操作

	if c.closed != 1 {
		t.Fatalf("Close called %d times, want 1", c.closed)
	}
}

func TestLifecycleManager_CloseErrorNotFatal(t *testing.T) {
	mgr := NewLifecycleManager(nil)

	errCloser := &mockCloser{err: errors.New("close error")}
	okCloser := &mockCloser{}

	mgr.Register("err", errCloser)
	mgr.Register("ok", okCloser)

	mgr.Close()

	// 即使 errCloser 出错，okCloser 也应被关闭
	if okCloser.closed != 1 {
		t.Fatal("okCloser should be closed even if errCloser failed")
	}
}

type callbackCloser struct {
	fn func() error
}

func (c *callbackCloser) Close() error { return c.fn() }

// DeadLetterQueue 测试

func TestDeadLetterQueue_EnqueueAndList(t *testing.T) {
	q := NewDeadLetterQueue(100, nil)

	q.Enqueue(DeadLetterMessage{ID: "1", Source: "test", Error: "err1"})
	q.Enqueue(DeadLetterMessage{ID: "2", Source: "test", Error: "err2"})

	msgs := q.List(0)
	if len(msgs) != 2 {
		t.Fatalf("List = %d msgs, want 2", len(msgs))
	}
}

func TestDeadLetterQueue_MaxSize(t *testing.T) {
	q := NewDeadLetterQueue(2, nil)

	q.Enqueue(DeadLetterMessage{ID: "1"})
	q.Enqueue(DeadLetterMessage{ID: "2"})
	q.Enqueue(DeadLetterMessage{ID: "3"}) // 应丢弃最旧的 "1"

	msgs := q.List(0)
	if len(msgs) != 2 {
		t.Fatalf("List = %d msgs, want 2", len(msgs))
	}
	if msgs[0].ID != "2" || msgs[1].ID != "3" {
		t.Fatalf("messages = [%s, %s], want [2, 3]", msgs[0].ID, msgs[1].ID)
	}
}

func TestDeadLetterQueue_RetrySuccess(t *testing.T) {
	q := NewDeadLetterQueue(100, nil)
	q.Enqueue(DeadLetterMessage{ID: "1", Source: "test", MaxRetries: 3})

	var retried int32
	err := q.Retry(context.Background(), "1", func(ctx context.Context, msg *DeadLetterMessage) error {
		atomic.AddInt32(&retried, 1)
		return nil
	})

	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if atomic.LoadInt32(&retried) != 1 {
		t.Fatal("handler not called")
	}
	if q.Len() != 0 {
		t.Fatalf("Len after successful retry = %d, want 0", q.Len())
	}
}

func TestDeadLetterQueue_RetryFailure(t *testing.T) {
	q := NewDeadLetterQueue(100, nil)
	q.Enqueue(DeadLetterMessage{ID: "1", Source: "test", MaxRetries: 2})

	// 第一次重试失败
	err := q.Retry(context.Background(), "1", func(ctx context.Context, msg *DeadLetterMessage) error {
		return errors.New("still failing")
	})
	if err == nil {
		t.Fatal("Retry should return error")
	}

	msgs := q.List(0)
	if len(msgs) != 1 {
		t.Fatalf("Len after failed retry = %d, want 1", len(msgs))
	}
	if msgs[0].RetryCount != 1 {
		t.Fatalf("RetryCount = %d, want 1", msgs[0].RetryCount)
	}
}

func TestDeadLetterQueue_Discard(t *testing.T) {
	q := NewDeadLetterQueue(100, nil)
	q.Enqueue(DeadLetterMessage{ID: "1"})

	if !q.Discard("1") {
		t.Fatal("Discard should return true for existing message")
	}
	if q.Len() != 0 {
		t.Fatalf("Len after discard = %d, want 0", q.Len())
	}

	if q.Discard("notexist") {
		t.Fatal("Discard should return false for non-existing message")
	}
}

// ManagedCache 测试已随 managed_cache.go 一并删除（TEST_ONLY 僵尸清理）。
