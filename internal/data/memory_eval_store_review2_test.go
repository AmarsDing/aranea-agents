package data

import (
	"sync"
	"testing"
	"time"
)

// TestEscapeEvalLike covers AMC P3 评审 C1 的 LIKE 字面化：PG LIKE 通配符是
// % 与 _（转义符默认 '\')；'[' 只在 T-SQL 方括号语法里是通配符，PG 下是
// 普通字符——必须原样保留，否则系列键前缀查询永远匹配不到行。
func TestEscapeEvalLike(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"[series-digest: 步数]", "[series-digest: 步数]"},
		{"a%b", `a\%b`},
		{"a_b", `a\_b`},
		{`a\b`, `a\\b`},
		{"50%_x", `50\%\_x`},
		{"", ""},
	}
	for _, tc := range cases {
		if got := escapeEvalLike(tc.in); got != tc.want {
			t.Errorf("escapeEvalLike(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestEvalUserWriteLock covers AMC P3 评审 C2: same-user AddMessages must
// serialize (supersede→digest→rollup share one read-modify-write window);
// different users must not block each other.
func TestEvalUserWriteLock(t *testing.T) {
	t.Parallel()
	var l evalUserWriteLock
	// 同用户互斥：并发临界区并发度恒为 1。
	var mu sync.Mutex
	inFlight, maxSeen := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := l.lock("u1")
			mu.Lock()
			inFlight++
			if inFlight > maxSeen {
				maxSeen = inFlight
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			unlock()
		}()
	}
	wg.Wait()
	if maxSeen != 1 {
		t.Fatalf("same-user critical section overlapped: max in-flight=%d", maxSeen)
	}
	// 不同用户不互斥：u1 持锁期间 u2 必须立即可得。
	u1 := l.lock("u1")
	defer u1()
	done := make(chan struct{})
	go func() {
		u2 := l.lock("u2")
		u2()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("different-user lock must not block on u1")
	}
}
