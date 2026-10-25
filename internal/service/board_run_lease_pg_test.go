package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeBoardLeaseRepo 进程内假仓（biz.BoardRunLeaseRepo 最小脚本化实现）。
type fakeBoardLeaseRepo struct {
	mu          sync.Mutex
	rows        map[string]fakeLeaseRow
	renewCalls  int
	renewErr    error // 常驻错误（模拟 DB 挂）
	renewBlock  bool  // Renew 阻塞到 ctx 死（对抗评审 #3：DB 驱动卡死场景）
	renewStops  atomic_flag
	hasActiveEr bool  // HasActive 返回错误（fail-closed 断言用）
	claimDeny   bool
}

type fakeLeaseRow struct {
	owner   string
	session string
	expires time.Time
}

type atomic_flag struct{ b bool }

func (r *fakeBoardLeaseRepo) TryClaimBoardRunLease(_ context.Context, lease biz.BoardRunLease, ownerID string, ttl time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimDeny {
		return false, errors.New("db down")
	}
	if row, ok := r.rows[lease.BoardID]; ok && time.Now().Before(row.expires) {
		return false, nil
	}
	if r.rows == nil {
		r.rows = map[string]fakeLeaseRow{}
	}
	r.rows[lease.BoardID] = fakeLeaseRow{owner: ownerID, session: lease.SessionID, expires: time.Now().Add(ttl)}
	return true, nil
}

func (r *fakeBoardLeaseRepo) RenewBoardRunLease(ctx context.Context, boardID, ownerID string, ttl time.Duration) (bool, error) {
	r.mu.Lock()
	r.renewCalls++
	block := r.renewBlock
	err0 := r.renewErr
	r.mu.Unlock()
	if block {
		// 模拟 DB 驱动卡死：只有 ctx 到期/取消才返回（#3 用例断言端口方
		// 的带边界子 ctx 能把它打断）。
		<-ctx.Done()
		return false, ctx.Err()
	}
	if err0 != nil {
		return false, err0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.renewStops.b {
		return false, nil // 模拟行被领养/过期
	}
	row, ok := r.rows[boardID]
	if !ok || row.owner != ownerID || time.Now().After(row.expires) {
		return false, nil
	}
	row.expires = time.Now().Add(ttl)
	r.rows[boardID] = row
	return true, nil
}

func (r *fakeBoardLeaseRepo) ReleaseBoardRunLease(_ context.Context, boardID, ownerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.rows[boardID]; ok && row.owner == ownerID {
		delete(r.rows, boardID)
	}
	return nil
}

func (r *fakeBoardLeaseRepo) HasActiveBoardRunForSession(_ context.Context, sessionID string, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasActiveEr {
		return false, errors.New("db down")
	}
	if sessionID == "" {
		return false, nil
	}
	for _, row := range r.rows {
		if row.session == sessionID && now.Before(row.expires) {
			return true, nil
		}
	}
	return false, nil
}

// TestPGBoardRunLease_ClaimStartsHeartbeatAndReleaseStops 认领成功起心跳、
// release 停心跳+删行；重复认领（活跃他人行）不起心跳。
func TestPGBoardRunLease_ClaimStartsHeartbeatAndReleaseStops(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{rows: map[string]fakeLeaseRow{}}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: 50 * time.Millisecond, Logger: loggateway.NewNoop()})
	if l == nil {
		t.Fatal("repo-equipped must construct")
	}
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"}) {
		t.Fatal("first claim must succeed")
	}
	if got := l.ActiveHeartbeats(); got != 1 {
		t.Fatalf("heartbeat count = %d, want 1", got)
	}
	// 同板活跃行二次认领必拒（port 语义等价 LoadOrStore）。
	if l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"}) {
		t.Fatal("active-row reclaim must be denied")
	}
	l.Release("b1")
	if got := l.ActiveHeartbeats(); got != 0 {
		t.Fatalf("heartbeat count after release = %d, want 0", got)
	}
	if l.HasActiveForSession("s1") {
		t.Fatal("released row must not be active")
	}
}

// TestPGBoardRunLease_ClaimFailClosed DB 错误 fail-closed：claimed=false。
func TestPGBoardRunLease_ClaimFailClosed(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{claimDeny: true}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: time.Minute, Logger: loggateway.NewNoop()})
	if l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1"}) {
		t.Fatal("repo error must fail-closed (claimed=false)")
	}
}

// TestPGBoardRunLease_LostLeaseCancelsRun 心跳续约 renewed=false → cancel 本地
// 僵尸 run（单写者兜底）。
func TestPGBoardRunLease_LostLeaseCancelsRun(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{rows: map[string]fakeLeaseRow{}}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: 40 * time.Millisecond, Logger: loggateway.NewNoop()})
	cancelled := make(chan struct{})
	cancel := func() { close(cancelled) }
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1", Cancel: cancel}) {
		t.Fatal("claim must succeed")
	}
	repo.mu.Lock()
	repo.renewStops.b = true // 下个 tick 起 renewed=false（模拟行被领养）
	repo.mu.Unlock()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("lost lease must cancel the local run")
	}
}

// TestPGBoardRunLease_RepeatedDBErrorsCancelRun 心跳连续 DB 错误达上限 →
// cancel（DB 抖动期间防僵尸续跑）。
func TestPGBoardRunLease_RepeatedDBErrorsCancelRun(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{rows: map[string]fakeLeaseRow{}}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: 40 * time.Millisecond, Logger: loggateway.NewNoop()})
	cancelled := make(chan struct{})
	cancel := func() { close(cancelled) }
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1", Cancel: cancel}) {
		t.Fatal("claim must succeed")
	}
	repo.mu.Lock()
	repo.renewErr = errors.New("db down")
	repo.mu.Unlock()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("3 consecutive renew failures must cancel the local run")
	}
}

// TestPGBoardRunLease_HasActiveFailClosed HasActive 出错 fail-closed=true
// （波次中点门控宁误拦不放行）。
func TestPGBoardRunLease_HasActiveFailClosed(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{hasActiveEr: true}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: time.Minute, Logger: loggateway.NewNoop()})
	if !l.HasActiveForSession("s1") {
		t.Fatal("repo error must fail-closed (active=true)")
	}
}

// TestResolveBoardRunLeaseMode_Matrix flag 档位解析：非法值 fail-safe=off。
func TestResolveBoardRunLeaseMode_Matrix(t *testing.T) {
	t.Parallel()
	getenv := func(v string) func(string) string {
		return func(string) string { return v }
	}
	cases := []struct {
		in   string
		want string
	}{
		{"", BoardRunLeaseModeOff},
		{"pg", BoardRunLeaseModePG},
		{" PG ", BoardRunLeaseModePG},
		{"mem", BoardRunLeaseModeMem},
		{"off", BoardRunLeaseModeOff},
		{"garbage", BoardRunLeaseModeOff},
	}
	for _, tc := range cases {
		if got := ResolveBoardRunLeaseMode(getenv(tc.in)); got != tc.want {
			t.Fatalf("ResolveBoardRunLeaseMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPGBoardRunLease_NilRepoSafe nil repo → 构造 nil（装配层按 flag=off）。
func TestPGBoardRunLease_NilRepoSafe(t *testing.T) {
	t.Parallel()
	if l := NewPGBoardRunLease(nil, PGBoardRunLeaseOptions{}); l != nil {
		t.Fatal("nil repo must construct nil")
	}
}

// TestPGBoardRunLease_StalledRenewBoundedAndSelfClean 对抗评审 #3/#1 合并用例：
// DB 驱动卡死（Renew 挂起）时——
//  1. 单次续约必须带有边界子 ctx（min(10s, ttl/2)，下限 1s）：连续
//     boardRunLeaseMaxRenewFail 次超时后 cancel 僵尸 run，而非无限挂起；
//  2. 杀僵尸路径退出后心跳句柄必须自清（ActiveHeartbeats 归零，不依赖
//     Release 调用）。
func TestPGBoardRunLease_StalledRenewBoundedAndSelfClean(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{rows: map[string]fakeLeaseRow{}, renewBlock: true}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: 40 * time.Millisecond, Logger: loggateway.NewNoop()})
	cancelled := make(chan struct{})
	cancel := func() { close(cancelled) }
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1", Cancel: cancel}) {
		t.Fatal("claim must succeed")
	}
	// 每次续约至少 1s（边界下限）×3 连败 ≈ 3s；给足 15s 防 CI 抖动。
	select {
	case <-cancelled:
	case <-time.After(15 * time.Second):
		t.Fatal("3 consecutive stalled renews must cancel the local zombie run")
	}
	repo.mu.Lock()
	n := repo.renewCalls
	repo.mu.Unlock()
	if n < boardRunLeaseMaxRenewFail {
		t.Fatalf("renewCalls=%d, want >= %d before kill", n, boardRunLeaseMaxRenewFail)
	}
	// #1：心跳句柄自清（旧实现杀僵尸裸退 → hb 泄漏恒 1）。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if l.ActiveHeartbeats() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("heartbeat handle leaked after zombie kill: %d, want 0", l.ActiveHeartbeats())
}

// TestPGBoardRunLease_ZombieKillSelfClean 对抗评审 #1（快速路径）：renewed=false
// 杀僵尸后无需 Release 心跳句柄也归零；且换代商誉——杀完立刻过期收养再起
// 心跳，期间旧 goroutine 的自清不得误删新句柄（最终仍归零）。
func TestPGBoardRunLease_ZombieKillSelfClean(t *testing.T) {
	t.Parallel()
	repo := &fakeBoardLeaseRepo{rows: map[string]fakeLeaseRow{}}
	l := NewPGBoardRunLease(repo, PGBoardRunLeaseOptions{Owner: "me:1:1", TTL: 40 * time.Millisecond, Logger: loggateway.NewNoop()})
	cancelled := make(chan struct{})
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1", Cancel: func() { close(cancelled) }}) {
		t.Fatal("first claim must succeed")
	}
	repo.mu.Lock()
	repo.renewStops.b = true
	repo.mu.Unlock()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("lost lease must cancel the local run")
	}
	// 过期收养再起二代心跳（行已过 TTL+宽限）。
	repo.mu.Lock()
	row := repo.rows["b1"]
	row.expires = time.Now().Add(-time.Second)
	repo.rows["b1"] = row
	repo.renewStops.b = false
	repo.mu.Unlock()
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"}) {
		t.Fatal("expired-row adoption must succeed")
	}
	if got := l.ActiveHeartbeats(); got != 1 {
		t.Fatalf("gen-2 heartbeat count = %d, want 1", got)
	}
	// gen-1 退出时的身份守卫自清不得误删 gen-2 句柄；给旧 goroutine 退出窗口。
	time.Sleep(100 * time.Millisecond)
	if got := l.ActiveHeartbeats(); got != 1 {
		t.Fatalf("gen-1 self-clean must not delete gen-2 handle: %d, want 1", got)
	}
	l.Release("b1")
	if got := l.ActiveHeartbeats(); got != 0 {
		t.Fatalf("release must stop gen-2 heartbeat: %d, want 0", got)
	}
}
