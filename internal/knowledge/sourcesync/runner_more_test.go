// runner_more_test.go —— W2 钉阵第二部：认领/收轮 CAS、僵尸回收、锁面
// fail-closed、跳空闸、walk 失败路径、submit-failed 合并桶。
package sourcesync

import (
	"context"
	"errors"
	"testing"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

// TestRunner_SkipMatrix 跳空闸四案：not_found / disabled / locked / claim_lost
// ——skipped 轮次一律不写 runs 审计行（runs 只记真实轮）。
func TestRunner_SkipMatrix(t *testing.T) {
	ctx := context.Background()

	t.Run("not_found", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		out, err := f.runner.SyncOnce(ctx, "src:conn:ghost")
		if err != nil || !out.Skipped || out.SkipReason != "not_found" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
		if f.runs.count() != 0 {
			t.Fatalf("skipped run must not audit")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		f := newRunnerFixture(t, func(c *bizsync.Connector) { c.Enabled = false })
		out, err := f.runner.SyncOnce(ctx, f.conn.ID)
		if err != nil || !out.Skipped || out.SkipReason != "disabled" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})

	t.Run("locked", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.locker.ok = false
		out, err := f.runner.SyncOnce(ctx, f.conn.ID)
		if err != nil || !out.Skipped || out.SkipReason != "locked" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})

	t.Run("claim_lost_running_fresh", func(t *testing.T) {
		// 活跃 running（心跳未过期）→ CAS 拒认 → skipped；僵尸窗不窃取
		f := newRunnerFixture(t, func(c *bizsync.Connector) {
			c.SyncState = bizsync.SyncStateRunning
			c.RunToken = "ss:tok:other"
			c.UpdatedAt = fixedClock.Add(-time.Minute) // 30min TTL 内=活跃
		})
		out, err := f.runner.SyncOnce(ctx, f.conn.ID)
		if err != nil || !out.Skipped || out.SkipReason != "claim_lost" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
		conn := f.conns.get(f.conn.ID)
		if conn.RunToken != "ss:tok:other" {
			t.Fatalf("live claim must not be stolen")
		}
	})

	t.Run("claim_lost_auth_paused", func(t *testing.T) {
		// auth_paused 一律拒（凭据唤醒唯一出路）
		f := newRunnerFixture(t, func(c *bizsync.Connector) {
			c.SyncState = bizsync.SyncStateAuthPaused
		})
		out, err := f.runner.SyncOnce(ctx, f.conn.ID)
		if err != nil || !out.Skipped || out.SkipReason != "claim_lost" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})
}

// TestRunner_LockerFailClosed ⑤：锁面故障宁停不跑（外部 API 配额副作用）。
func TestRunner_LockerFailClosed(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.locker.err = errors.New("pg advisory lock backend down")
	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err == nil {
		t.Fatalf("fail-closed must surface locker error")
	}
	if !out.Skipped && out.RunToken != "" {
		t.Fatalf("no claim may happen on locker failure: %+v", out)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("conn untouched: %s", conn.SyncState)
	}
}

// TestRunner_ZombieStealViaClaim ⑤ 崩溃回收主路径：running ∧ updated_at 过期
// → TryClaimRun 内嵌窃取，当轮照常执行。
func TestRunner_ZombieStealViaClaim(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) {
		c.SyncState = bizsync.SyncStateRunning
		c.RunToken = "ss:tok:zombie"
		c.UpdatedAt = fixedClock.Add(-2 * time.Hour) // 远超 30min TTL
	})
	f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Skipped {
		t.Fatalf("zombie window must be stealable: %s", out.SkipReason)
	}
	if out.Status != bizsync.RunStatusSuccess || out.Stats.Created != 1 {
		t.Fatalf("out = %+v", out)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.RunToken != "" || conn.SyncState != bizsync.SyncStateIdle {
		t.Fatalf("stolen run finalized: %s token=%q", conn.SyncState, conn.RunToken)
	}
}

// TestRunner_ReclaimZombies 崩溃回收观测面：过期 running→idle + 轻罚 +1；
// 活跃 running 不动。
func TestRunner_ReclaimZombies(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.conns.seed(bizsync.Connector{
		ID: "src:conn:z1", Kind: bizsync.KindSharedDir, Name: "z1", WorkspaceID: "ws1", CollectionID: "c",
		Enabled: true, SyncState: bizsync.SyncStateRunning, RunToken: "ss:tok:z1",
		UpdatedAt: fixedClock.Add(-2 * time.Hour), ConsecutiveFailures: 4,
	})
	f.conns.seed(bizsync.Connector{
		ID: "src:conn:z2", Kind: bizsync.KindSharedDir, Name: "z2", WorkspaceID: "ws1", CollectionID: "c2",
		Enabled: true, SyncState: bizsync.SyncStateRunning, RunToken: "ss:tok:z2",
		UpdatedAt: fixedClock.Add(-time.Minute), // 活跃=不收
	})

	n, err := f.runner.ReclaimZombies(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("reclaimed = %d err = %v, want 1", n, err)
	}
	z1 := f.conns.get("src:conn:z1")
	if z1.SyncState != bizsync.SyncStateIdle || z1.RunToken != "" || z1.ConsecutiveFailures != 5 {
		t.Fatalf("z1 = %+v", z1)
	}
	z2 := f.conns.get("src:conn:z2")
	if z2.SyncState != bizsync.SyncStateRunning || z2.RunToken != "ss:tok:z2" {
		t.Fatalf("z2 untouched: %+v", z2)
	}
}

// TestRunner_WalkFailureKeepsCursorAndBackoff walker 一般性 abort：零删除零
// 圈定 + 游标回滚 + 退避推进 + 状态留 error。
func TestRunner_WalkFailureKeepsCursorAndBackoff(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) {
		c.ConsecutiveFailures = 2
		c.Cursor = "cur:c0"
	})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "kept", Status: bizsync.ItemStatusActive, DocumentID: "d1", ContentHash: "h", Fingerprint: "v1"})
	f.walker.walkErr = errors.New("upstream 502 bad gateway")

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkFailed {
		t.Fatalf("out = %s/%s", out.Status, out.ErrorCode)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.Cursor != "cur:c0" {
		t.Fatalf("cursor rollback: %q", conn.Cursor)
	}
	if conn.ConsecutiveFailures != 3 || !conn.NextRunAt.Equal(fixedClock.Add(8*time.Minute)) {
		t.Fatalf("backoff: failures=%d next=%v", conn.ConsecutiveFailures, conn.NextRunAt)
	}
	if !conn.LastSuccessAt.IsZero() {
		t.Fatalf("failed run must not push success watermark")
	}
	it, _ := f.items.get(f.conn.ID, "kept")
	if it.Status != bizsync.ItemStatusActive {
		t.Fatalf("no absent mark on walk abort: %s", it.Status)
	}
	run := f.runs.last()
	if run.Status != bizsync.RunStatusFailed || run.ErrorCode != bizsync.ErrCodeWalkFailed || run.ErrorMessage == "" {
		t.Fatalf("run row = %+v", run)
	}
}

// TestRunner_FetchFailurePartial fetch 单点失败：fetch_failed 桶 + 病态留痕 +
// 旧指纹保留（下轮重试）+ success/partial 收轮界线（partial⇔FetchFailed>0）。
func TestRunner_FetchFailurePartial(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "bad1", Status: bizsync.ItemStatusActive, DocumentID: "doc:bad1", ContentHash: "h1", Fingerprint: "v1", OwnLifecycle: true})
	f.walker.snaps = []Snapshot{
		{ExternalID: "ok1", Fingerprint: "v1"},
		{ExternalID: "bad1", Fingerprint: "v2"}, // 漂移但 fetch 挂
	}
	f.walker.fetchErrs = map[string]error{"bad1": errors.New("connection reset")}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusPartial {
		t.Fatalf("status = %s, want partial", out.Status)
	}
	if out.Stats.FetchFailed != 1 || out.Stats.Created != 1 {
		t.Fatalf("stats = %+v", out.Stats)
	}
	it, _ := f.items.get(f.conn.ID, "bad1")
	if it.Fingerprint != "v1" {
		t.Fatalf("failed fetch keeps old fingerprint for retry: %+v", it)
	}
	if it.ErrorMessage == "" || it.Status != bizsync.ItemStatusActive {
		t.Fatalf("item pathology must be recorded: %+v", it)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateIdle || conn.ConsecutiveFailures != 0 {
		t.Fatalf("partial keeps healthy cadence: %s/%d", conn.SyncState, conn.ConsecutiveFailures)
	}
	if !conn.LastSuccessAt.Equal(fixedClock) {
		t.Fatalf("completed walk pushes success watermark even when partial")
	}
}

// TestRunner_ApplyFailureSameBucket W2 合口：applier.Upsert 失败与 fetch 失败
// 同桶（runs 表列冻结），病态阶段前缀可辨（apply:）。
func TestRunner_ApplyFailureSameBucket(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.appl.upsertErrs = map[string]error{"gone": errors.New("vault indexing busy")}
	f.walker.snaps = []Snapshot{{ExternalID: "gone", Fingerprint: "v1"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Stats.FetchFailed != 1 || out.Stats.Created != 0 || out.Status != bizsync.RunStatusPartial {
		t.Fatalf("stats = %+v status=%s", out.Stats, out.Status)
	}
	it, _ := f.items.get(f.conn.ID, "gone")
	if it.ErrorMessage == "" || it.DocumentID != "" {
		t.Fatalf("failed apply leaves unbound item with pathology: %+v", it)
	}
}

// TestRunner_FinalizeClaimLost 收轮失地：Reclaim/软删/管理冲突致
// FinalizeRunWithClaim found=false → ClaimLost=true 但 run 审计行照落。
func TestRunner_FinalizeClaimLost(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.conns.forceFinalizeMiss = true
	f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !out.ClaimLost || out.Finalized {
		t.Fatalf("flags: claimLost=%v finalized=%v", out.ClaimLost, out.Finalized)
	}
	if f.runs.count() != 1 {
		t.Fatalf("audit row must still land: %d", f.runs.count())
	}
}

// TestRunner_NilLockerSingleProcess Locker=nil（单进程测试面）照常执行。
func TestRunner_NilLockerSingleProcess(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.runner = NewRunner(Deps{
		Connectors: f.conns, Items: f.items, Runs: f.runs,
		Walkers:  map[bizsync.Kind]Walker{f.walker.Kind(): f.walker},
		Appliers: map[bizsync.Kind]Applier{f.walker.Kind(): f.appl},
		Now:      func() time.Time { return fixedClock },
	})
	f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "v1"}}
	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil || out.Skipped || out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

// TestRunner_NeedsWorkMatrix needsWork 判定钉：DocumentID=="" 或
// ContentHash=="" 的出席项（上轮预算/失败跳过）当轮必重处理。
func TestRunner_NeedsWorkMatrix(t *testing.T) {
	f := newRunnerFixture(t, nil)
	// 上轮预算跳过的新项：指纹已记但无绑定——当轮必拉取
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "half", Status: bizsync.ItemStatusActive, Fingerprint: "v1"})
	f.walker.snaps = []Snapshot{{ExternalID: "half", Fingerprint: "v1"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Stats.Updated != 1 || f.walker.fetchCalls["half"] == 0 {
		t.Fatalf("unbound item must be fetched: stats=%+v fetchCalls=%v", out.Stats, f.walker.fetchCalls)
	}
	it, _ := f.items.get(f.conn.ID, "half")
	if it.DocumentID != "doc:half" || it.ContentHash != "hash:v1" {
		t.Fatalf("binding backfilled: %+v", it)
	}
}
