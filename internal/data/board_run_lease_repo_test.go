package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// ensureBoardRunLeaseTable 在隔离 schema 内建表——逐句镜像迁移
// 20261295_board_run_leases.sql（raw-SQL 表不经 ent Schema.Create；迁移文件
// 改动必须同步本镜像，列序钉死测试依赖不漂移）。
func ensureBoardRunLeaseTable(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS board_run_leases (
  board_id   TEXT PRIMARY KEY,
  session_id TEXT NOT NULL DEFAULT '',
  owner_id   TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_board_run_leases_session
  ON board_run_leases(session_id, expires_at);
`)
	if err != nil {
		t.Fatalf("create board_run_leases mirror DDL: %v", err)
	}
}

const (
	leaseOwnerA = "host-a:1:111"
	leaseOwnerB = "host-b:2:222"
)

// TestBoardRunLeaseRepo_ClaimAdoptOnlyExpired 认领语义钉死：
// 活跃行他人二次认领=false（且同 owner 收养——含同进程重复 startBoardDAG）；仅
// 过期行可被收养（接管崩溃实例）。
func TestBoardRunLeaseRepo_ClaimAdoptOnlyExpired(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureBoardRunLeaseTable(t, d)
	repo := NewBoardRunLeaseRepo(d)
	ctx := context.Background()
	ttl := 30 * time.Second
	lease := biz.BoardRunLease{BoardID: "board-1", SessionID: "sess-1"}

	claimed, err := repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerA, ttl)
	if err != nil || !claimed {
		t.Fatalf("first claim = (%v,%v), want (true,nil)", claimed, err)
	}
	// 活跃期他人认领必拒。
	claimed, err = repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerB, ttl)
	if err != nil || claimed {
		t.Fatalf("active-row second claim = (%v,%v), want (false,nil)", claimed, err)
	}
	// 活跃期同 owner 认领也必拒（进程内重复 startBoardDAG 幂等底线）。
	claimed, err = repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerA, ttl)
	if err != nil || claimed {
		t.Fatalf("active-row same-owner reclaim = (%v,%v), want (false,nil)", claimed, err)
	}
	// 过期后可收养：直接落一条已过期行。
	db := d.RWDB().WriteDB(ctx)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, d.Dialect().RenumberPlaceholders(
		`UPDATE board_run_leases SET expires_at = ? WHERE board_id = ?`), past, "board-1"); err != nil {
		t.Fatalf("expire row: %v", err)
	}
	claimed, err = repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerB, ttl)
	if err != nil || !claimed {
		t.Fatalf("expired-row adoption = (%v,%v), want (true,nil)", claimed, err)
	}
	// 收养后 owner 已换位：A 续约必败。
	renewed, err := repo.RenewBoardRunLease(ctx, "board-1", leaseOwnerA, ttl)
	if err != nil || renewed {
		t.Fatalf("owner-A renew after adoption = (%v,%v), want (false,nil)", renewed, err)
	}
}

// TestBoardRunLeaseRepo_RenewReleaseOwnerGuard renew/release owner 守卫 +
// 过期不续约 + release 幂等。
func TestBoardRunLeaseRepo_RenewReleaseOwnerGuard(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureBoardRunLeaseTable(t, d)
	repo := NewBoardRunLeaseRepo(d)
	ctx := context.Background()
	ttl := 30 * time.Second
	lease := biz.BoardRunLease{BoardID: "board-2", SessionID: "sess-2"}

	if claimed, err := repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerA, ttl); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	renewed, err := repo.RenewBoardRunLease(ctx, "board-2", leaseOwnerA, ttl)
	if err != nil || !renewed {
		t.Fatalf("owner renew = (%v,%v), want (true,nil)", renewed, err)
	}
	renewed, err = repo.RenewBoardRunLease(ctx, "board-2", leaseOwnerB, ttl)
	if err != nil || renewed {
		t.Fatalf("foreign renew = (%v,%v), want (false,nil)", renewed, err)
	}
	// 他人 release 必不删行（幂等 nil error）。
	if err := repo.ReleaseBoardRunLease(ctx, "board-2", leaseOwnerB); err != nil {
		t.Fatalf("foreign release must be nil-error idempotent: %v", err)
	}
	active, err := repo.HasActiveBoardRunForSession(ctx, "sess-2", time.Now())
	if err != nil || !active {
		t.Fatalf("row must survive foreign release: (%v,%v)", active, err)
	}
	// owner release 后行消失 + 幂等。
	if err := repo.ReleaseBoardRunLease(ctx, "board-2", leaseOwnerA); err != nil {
		t.Fatalf("owner release: %v", err)
	}
	if err := repo.ReleaseBoardRunLease(ctx, "board-2", leaseOwnerA); err != nil {
		t.Fatalf("release idempotent: %v", err)
	}
	active, err = repo.HasActiveBoardRunForSession(ctx, "sess-2", time.Now())
	if err != nil || active {
		t.Fatalf("released row must be gone: (%v,%v)", active, err)
	}
}

// TestBoardRunLeaseRepo_HasActiveExpiryFilter HasActiveForSession 只看未过期行，
// 空 session 恒 false（跨副本门控误报防线）。
func TestBoardRunLeaseRepo_HasActiveExpiryFilter(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureBoardRunLeaseTable(t, d)
	repo := NewBoardRunLeaseRepo(d)
	ctx := context.Background()
	ttl := 30 * time.Second

	if claimed, err := repo.TryClaimBoardRunLease(ctx, biz.BoardRunLease{BoardID: "board-3", SessionID: "sess-3"}, leaseOwnerA, ttl); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if active, err := repo.HasActiveBoardRunForSession(ctx, "", time.Now()); err != nil || active {
		t.Fatalf("empty session must be false: (%v,%v)", active, err)
	}
	if active, err := repo.HasActiveBoardRunForSession(ctx, "sess-3", time.Now()); err != nil || !active {
		t.Fatalf("active session: (%v,%v)", active, err)
	}
	if active, err := repo.HasActiveBoardRunForSession(ctx, "sess-other", time.Now()); err != nil || active {
		t.Fatalf("other session: (%v,%v)", active, err)
	}
	// 过期行不算活跃。
	db := d.RWDB().WriteDB(ctx)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, d.Dialect().RenumberPlaceholders(
		`UPDATE board_run_leases SET expires_at = ? WHERE board_id = ?`), past, "board-3"); err != nil {
		t.Fatalf("expire row: %v", err)
	}
	if active, err := repo.HasActiveBoardRunForSession(ctx, "sess-3", time.Now()); err != nil || active {
		t.Fatalf("expired row must not be active: (%v,%v)", active, err)
	}
}

// TestBoardRunLeaseRepo_NilDataSafe nil Data 兜底：写均 Internal 错、读 Internal 错。
func TestBoardRunLeaseRepo_NilDataSafe(t *testing.T) {
	t.Parallel()
	repo := NewBoardRunLeaseRepo(nil)
	ctx := context.Background()
	if _, err := repo.TryClaimBoardRunLease(ctx, biz.BoardRunLease{BoardID: "b"}, "o", time.Second); err == nil {
		t.Fatal("nil data TryClaim must report unavailable")
	}
	if _, err := repo.HasActiveBoardRunForSession(ctx, "s", time.Now()); err == nil {
		t.Fatal("nil data HasActive must report unavailable")
	}
	// Renew/Release 同样 fail-closed 报 unavailable（对抗评审 #8 补钉）。
	if _, err := repo.RenewBoardRunLease(ctx, "b", "o", time.Second); err == nil {
		t.Fatal("nil data Renew must report unavailable")
	}
	if err := repo.ReleaseBoardRunLease(ctx, "b", "o"); err == nil {
		t.Fatal("nil data Release must report unavailable")
	}
}

// TestBoardRunLeaseRepo_RenewExpiredRowDenied 对抗评审 #8：过期行 owner 本人
// 续约也必须 renewed=false（expires_at>now 守卫不可绕过）——否则崩溃实例房
// 里「仍在跑的旧进程」永远续约成功、收养语义纯摆设。
func TestBoardRunLeaseRepo_RenewExpiredRowDenied(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureBoardRunLeaseTable(t, d)
	repo := NewBoardRunLeaseRepo(d)
	ctx := context.Background()
	ttl := 30 * time.Second
	lease := biz.BoardRunLease{BoardID: "board-exp", SessionID: "sess-exp"}

	if claimed, err := repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerA, ttl); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	db := d.RWDB().WriteDB(ctx)
	past := formatBoardRunLeaseTime(time.Now().Add(-time.Minute))
	if _, err := db.ExecContext(ctx, d.Dialect().RenumberPlaceholders(
		`UPDATE board_run_leases SET expires_at = ? WHERE board_id = ?`), past, "board-exp"); err != nil {
		t.Fatalf("expire row: %v", err)
	}
	if renewed, err := repo.RenewBoardRunLease(ctx, "board-exp", leaseOwnerA, ttl); err != nil || renewed {
		t.Fatalf("expired-row owner renew = (%v,%v), want (false,nil)", renewed, err)
	}
}

// TestBoardRunLeaseTimeLayout_Property 对抗评审 #0/#11 对抗锚：定宽布局保证
// TEXT 列字典序 == 时序。RFC3339Nano 变宽小数在整秒（零小数）瞬时字典序
// 倒置（"…:05Z" > "…:05.9Z" 但时序更早）→ 活跃租约被误判过期收养/
// HasActive 漏判活跃。本用例在旧布局下确定性失败。
func TestBoardRunLeaseTimeLayout_Property(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_800_000_000, 0).UTC() // 整秒
	instants := []time.Time{
		base.Add(-time.Second),
		base.Add(-time.Nanosecond),
		base, // 关键：整秒零小数瞬时
		base.Add(time.Nanosecond),
		base.Add(500 * time.Millisecond),
		base.Add(999999999 * time.Nanosecond),
		base.Add(time.Second),
	}
	var last string
	for i, inst := range instants {
		s := formatBoardRunLeaseTime(inst)
		if i > 0 && !(last < s) {
			t.Fatalf("lexicographic order broken at %d: %q !< %q", i, last, s)
		}
		// 定宽钉子：纳秒小数位恒 9 位（19 位 datetime + "." + 9 + "Z"）。
		if len(s) != 19+1+9+1 {
			t.Fatalf("fixed-width layout violated: %q len=%d", s, len(s))
		}
		last = s
	}
	// 解析回读双向无损（TEXT 往返语义不漂移）。
	inst := base.Add(123456789 * time.Nanosecond)
	if got := formatBoardRunLeaseTime(mustParseLeaseTime(t, formatBoardRunLeaseTime(inst))); got != formatBoardRunLeaseTime(inst) {
		t.Fatalf("roundtrip = %q, want %q", got, formatBoardRunLeaseTime(inst))
	}
}

func mustParseLeaseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(boardRunLeaseTimeLayout, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return parsed.UTC()
}

// TestBoardRunLeaseRepo_WholeSecondActiveRowNotAdopted 对抗评审 #0/#11 数据库
// 端到端：过期时刻恰落整秒的活跃行（定宽格式写盘）不可被收养、HasActive
// 必须可见——旧变宽格式在此形态下字典序倒置。
func TestBoardRunLeaseRepo_WholeSecondActiveRowNotAdopted(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureBoardRunLeaseTable(t, d)
	repo := NewBoardRunLeaseRepo(d)
	ctx := context.Background()
	ttl := 30 * time.Second
	lease := biz.BoardRunLease{BoardID: "board-ws", SessionID: "sess-ws"}

	if claimed, err := repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerA, ttl); err != nil || !claimed {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	// 过期时刻改写为「未来第一个整秒」——模拟心跳把 expires_at 推上整秒
	// 边界的形态（旧变宽格式下行内为 "…:00Z" 无小数，字典序倒置触发点）。
	db := d.RWDB().WriteDB(ctx)
	wholeSecFuture := time.Now().UTC().Add(time.Second).Truncate(time.Second)
	if !wholeSecFuture.After(time.Now()) {
		wholeSecFuture = wholeSecFuture.Add(time.Second)
	}
	if _, err := db.ExecContext(ctx, d.Dialect().RenumberPlaceholders(
		`UPDATE board_run_leases SET expires_at = ? WHERE board_id = ?`), formatBoardRunLeaseTime(wholeSecFuture), "board-ws"); err != nil {
		t.Fatalf("rewrite expires_at: %v", err)
	}
	// 他人收养必拒（活跃行）。
	if claimed, err := repo.TryClaimBoardRunLease(ctx, lease, leaseOwnerB, ttl); err != nil || claimed {
		t.Fatalf("whole-second active row adoption = (%v,%v), want (false,nil)", claimed, err)
	}
	// HasActive 必须可见该活跃行。
	if active, err := repo.HasActiveBoardRunForSession(ctx, "sess-ws", time.Now()); err != nil || !active {
		t.Fatalf("whole-second active row invisible: (%v,%v), want (true,nil)", active, err)
	}
}
