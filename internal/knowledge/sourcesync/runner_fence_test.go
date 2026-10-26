// runner_fence_test.go —— 评审 W2 修（wf_1940525b-dd1，13 确认）收复钉：
// fencing 三闸（emit 顶/pre-⑥-3/删除循环随检）、deleted 复活重绑、字节预算
// 哨兵、空 external id、级联失败保留、注册缺失分支、游标连续性、审计失败
// 续收轮、仓口故障六分支、RunOutcome.Error 贯通。风格同 runner_test.go。
package sourcesync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

// fenceSetup fencing 钉公共装配：心跳 1ms + 失地同步点。
func fenceSetup(f *runnerFixture) {
	f.runner.hb = time.Millisecond
	f.conns.lostSeen = make(chan struct{})
}

// TestFence_ClaimLostAbortsWalk 闸①：walk 中途失地 → emit 顶闸即刻中止，
// 已出席项保留、游标不动、零圈定零删除、审计照落、Finalize 必失地。
func TestFence_ClaimLostAbortsWalk(t *testing.T) {
	f := newRunnerFixture(t, nil)
	fenceSetup(f)
	f.walker.snaps = []Snapshot{
		{ExternalID: "n0", Fingerprint: "fp:n0"},
		{ExternalID: "n1", Fingerprint: "fp:n1"},
		{ExternalID: "n2", Fingerprint: "fp:n2"},
	}
	f.walker.beforeEmit = func(idx int) {
		if idx == 1 {
			f.conns.steal(f.conn.ID) // 新属主夺权
			<-f.conns.lostSeen       // 等心跳判死（确定性同步，无竞态窗）
		}
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if !errors.Is(out.Error, errClaimLost) {
		t.Fatalf("out.Error = %v, want errClaimLost", out.Error)
	}
	if !out.ClaimLost || out.Finalized {
		t.Fatalf("flags: claimLost=%v finalized=%v", out.ClaimLost, out.Finalized)
	}
	if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkFailed {
		t.Fatalf("status/code = %s/%s", out.Status, out.ErrorCode)
	}
	if out.Stats.Created != 1 || out.Stats.Seen != 1 {
		t.Fatalf("n0 应已建绑（出席+工作完成才失地）: %+v", out.Stats)
	}
	if f.walker.fetchCalls["n1"] != 0 || f.walker.fetchCalls["n2"] != 0 {
		t.Fatalf("失地后应即刻止步，fetch = %+v", f.walker.fetchCalls)
	}
	if f.runs.count() != 1 {
		t.Fatalf("审计行照落铁律: count=%d", f.runs.count())
	}
	got := f.conns.get(f.conn.ID)
	if got.RunToken != "ss:tok:stolen" || got.SyncState != bizsync.SyncStateRunning {
		t.Fatalf("新属主写面不得被旧主覆写: %+v", got)
	}
	if got.Cursor != f.conn.Cursor || got.ConsecutiveFailures != f.conn.ConsecutiveFailures {
		t.Fatalf("游标/退避面不得动: %+v", got)
	}
}

// TestFence_ClaimLostSkipsMarking 闸②：walk 完满后失地 → ⑥-3 圈定永不执行
// （switch/pre-⑥-3 闸殊途同归，钉观测面：缺席项保持 active 不被错圈）。
func TestFence_ClaimLostSkipsMarking(t *testing.T) {
	f := newRunnerFixture(t, nil)
	fenceSetup(f)
	seenBase := fixedClock.Add(-2 * time.Hour)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "keep", Fingerprint: "fp:keep",
		DocumentID: "doc:keep", ContentHash: "h:keep", Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "old", Fingerprint: "fp:old",
		DocumentID: "doc:old", ContentHash: "h:old", OwnLifecycle: true, Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.walker.snaps = []Snapshot{{ExternalID: "keep", Fingerprint: "fp:keep"}}
	f.walker.onWalkEnd = func() {
		f.conns.steal(f.conn.ID)
		<-f.conns.lostSeen
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if !errors.Is(out.Error, errClaimLost) || !out.ClaimLost {
		t.Fatalf("flags: err=%v claimLost=%v", out.Error, out.ClaimLost)
	}
	old, _ := f.items.get(f.conn.ID, "old")
	if old.Status != bizsync.ItemStatusActive {
		t.Fatalf("圈定不可发生于失地后: old=%+v", old)
	}
	if out.Stats.MissUnknown != 0 || out.Stats.Deleted != 0 {
		t.Fatalf("删除面零消费: %+v", out.Stats)
	}
	if got := f.conns.get(f.conn.ID); got.Cursor != f.conn.Cursor {
		t.Fatalf("游标不得推进: %q", got.Cursor)
	}
}

// TestFence_ClaimLostAbortsDeletion 闸③：删除循环内失地 → ConfirmDeleted
// 结果不再消费（零级联零标记），候选项保留 missing 让新主自决。
func TestFence_ClaimLostAbortsDeletion(t *testing.T) {
	f := newRunnerFixture(t, nil)
	fenceSetup(f)
	seenBase := fixedClock.Add(-2 * time.Hour)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "keep", Fingerprint: "fp:keep",
		DocumentID: "doc:keep", ContentHash: "h:keep", Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "old", Fingerprint: "fp:old",
		DocumentID: "doc:old", ContentHash: "h:old", OwnLifecycle: true, Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.walker.snaps = []Snapshot{{ExternalID: "keep", Fingerprint: "fp:keep"}}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"old": bizsync.VerdictConfirmed}
	f.walker.onConfirm = func(string) {
		f.conns.steal(f.conn.ID)
		<-f.conns.lostSeen
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if !errors.Is(out.Error, errClaimLost) || !out.ClaimLost {
		t.Fatalf("flags: err=%v claimLost=%v", out.Error, out.ClaimLost)
	}
	if len(f.appl.deleteCalls) != 0 {
		t.Fatalf("失地后级联删除不可发: %+v", f.appl.deleteCalls)
	}
	old, _ := f.items.get(f.conn.ID, "old")
	if old.Status != bizsync.ItemStatusMissing {
		t.Fatalf("⑥-3 圈定已发生（失地前），old 应留 missing 侍新主: %+v", old)
	}
	if out.Stats.Deleted != 0 {
		t.Fatalf("Deleted = %d, want 0", out.Stats.Deleted)
	}
}

// TestResurrect_Rebinds 复活条款：deleted 项重新出席即重取重绑——指纹相同
// 也不能省（指针挂的是旧 doc，或已级联删除）。
func TestResurrect_Rebinds(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "z", Fingerprint: "fp:old",
		DocumentID: "doc:STALE", ContentHash: "h:STALE", OwnLifecycle: true,
		Status: bizsync.ItemStatusDeleted, DeletedAt: seenBaseT(), LastSeenRunID: "ss:run:prev"})
	f.walker.snaps = []Snapshot{{ExternalID: "z", Fingerprint: "fp:old"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Status != bizsync.RunStatusSuccess || out.Stats.Updated != 1 {
		t.Fatalf("复活应计 updated=1 且 success: %s %+v", out.Status, out.Stats)
	}
	if f.walker.fetchCalls["z"] != 1 {
		t.Fatalf("deleted 出席必须重取: fetch=%d", f.walker.fetchCalls["z"])
	}
	z, _ := f.items.get(f.conn.ID, "z")
	if z.Status != bizsync.ItemStatusActive || z.DocumentID != "doc:z" || z.ContentHash != "hash:fp:old" || !z.DeletedAt.IsZero() {
		t.Fatalf("复活重绑终态: %+v", z)
	}
}

func seenBaseT() time.Time { return fixedClock.Add(-2 * time.Hour) }

// TestBudget_BytesSentinel 字节预算哨兵：16+16 撞 20 上限 → 第三条中止，
// 游标不动、partial 收轮、已提交两条有效。
func TestBudget_BytesSentinel(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.MaxFetchBytesPerRun = 20 })
	f.walker.snaps = []Snapshot{
		{ExternalID: "n1", Fingerprint: "fp:n1"},
		{ExternalID: "n2", Fingerprint: "fp:n2"},
		{ExternalID: "n3", Fingerprint: "fp:n3"},
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Status != bizsync.RunStatusPartial || out.ErrorCode != bizsync.ErrCodeNone {
		t.Fatalf("partial 收轮面: %s/%s", out.Status, out.ErrorCode)
	}
	if out.Stats.Created != 2 || out.Stats.BytesPulled != 32 {
		t.Fatalf("预算前两件有效: %+v", out.Stats)
	}
	if _, ok := f.items.get(f.conn.ID, "n3"); ok {
		t.Fatalf("n3 不可出席（预算哨兵先于出席）")
	}
	if got := f.conns.get(f.conn.ID); got.Cursor != f.conn.Cursor {
		t.Fatalf("预算轮游标不动: %q", got.Cursor)
	}
}

// TestWalk_EmptyExternalIDAborts 空 external id 契约违背 → walk 立即 abort。
func TestWalk_EmptyExternalIDAborts(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.walker.snaps = []Snapshot{{ExternalID: "", Fingerprint: "fp:x"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkFailed {
		t.Fatalf("abort 面: %s/%s", out.Status, out.ErrorCode)
	}
	if out.Error == nil || !strings.Contains(out.Error.Error(), "empty external id") {
		t.Fatalf("cause 贯通: %v", out.Error)
	}
	if got := f.conns.get(f.conn.ID); got.ConsecutiveFailures != f.conn.ConsecutiveFailures+1 {
		t.Fatalf("退避递增: %+v", got)
	}
}

// TestCascade_DeleteFailureKeepsMissing 级联删除失败：item 保留 missing
// 下轮再议，miss_unknown+1，不降级轮次评级（success 照收）。
func TestCascade_DeleteFailureKeepsMissing(t *testing.T) {
	f := newRunnerFixture(t, nil)
	seenBase := fixedClock.Add(-2 * time.Hour)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "keep", Fingerprint: "fp:keep",
		DocumentID: "doc:keep", ContentHash: "h:keep", Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "old", Fingerprint: "fp:old",
		DocumentID: "doc:old", ContentHash: "h:old", OwnLifecycle: true, Status: bizsync.ItemStatusActive,
		LastSeenRunID: "ss:run:prev", LastSeenAt: seenBase})
	f.walker.snaps = []Snapshot{{ExternalID: "keep", Fingerprint: "fp:keep"}}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"old": bizsync.VerdictConfirmed}
	f.appl.deleteErr = errFakeInfra

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("级联失败不降级轮次: %s", out.Status)
	}
	if out.Stats.MissUnknown != 1 || out.Stats.Deleted != 0 {
		t.Fatalf("miss_unknown 记账: %+v", out.Stats)
	}
	old, _ := f.items.get(f.conn.ID, "old")
	if old.Status != bizsync.ItemStatusMissing {
		t.Fatalf("保留 missing 下轮再议: %+v", old)
	}
}

// TestUnregistered_WalkerAndApplier 注册缺失两分支（kind 无 walker/无 applier）。
func TestUnregistered_WalkerAndApplier(t *testing.T) {
	t.Run("walker", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.runner = NewRunner(Deps{
			Connectors: f.conns, Items: f.items, Runs: f.runs,
			Locker:   f.locker,
			Walkers:  map[bizsync.Kind]Walker{},
			Appliers: map[bizsync.Kind]Applier{f.walker.Kind(): f.appl},
			Now:      func() time.Time { return fixedClock },
			Frames:   f.frames,
		})
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkFailed {
			t.Fatalf("%s/%s", out.Status, out.ErrorCode)
		}
		if out.Error == nil || !strings.Contains(out.Error.Error(), "walker") {
			t.Fatalf("cause: %v", out.Error)
		}
		if f.runs.count() != 1 {
			t.Fatalf("审计照落: %d", f.runs.count())
		}
	})
	t.Run("applier", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.runner = NewRunner(Deps{
			Connectors: f.conns, Items: f.items, Runs: f.runs,
			Locker:   f.locker,
			Walkers:  map[bizsync.Kind]Walker{f.walker.Kind(): f.walker},
			Appliers: map[bizsync.Kind]Applier{},
			Now:      func() time.Time { return fixedClock },
			Frames:   f.frames,
		})
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.Error == nil || !strings.Contains(out.Error.Error(), "applier") {
			t.Fatalf("cause: %v", out.Error)
		}
		if got := f.conns.get(f.conn.ID); got.SyncState != bizsync.SyncStateError {
			t.Fatalf("error 态落库: %+v", got)
		}
	})
}

// TestCursor_ContinuityAcrossRounds 游标跨轮连续（#7：fake 原样记 prev）。
func TestCursor_ContinuityAcrossRounds(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.walker.snaps = []Snapshot{
		{ExternalID: "a", Fingerprint: "fp:a"},
		{ExternalID: "b", Fingerprint: "fp:b"},
	}
	out1, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil || out1.Status != bizsync.RunStatusSuccess {
		t.Fatalf("round1: %v %s", err, out1.Status)
	}
	out2, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil || out2.Status != bizsync.RunStatusSuccess {
		t.Fatalf("round2: %v %s", err, out2.Status)
	}
	if out2.Stats.Unchanged != 2 {
		t.Fatalf("round2 全 unchanged: %+v", out2.Stats)
	}
	if len(f.walker.seenPrevs) != 2 || f.walker.seenPrevs[0] != "" || f.walker.seenPrevs[1] != "cur:n+1" {
		t.Fatalf("prev 连续性: %+v", f.walker.seenPrevs)
	}
	if got := f.conns.get(f.conn.ID); got.Cursor != "cur:n+1" {
		t.Fatalf("cursor = %q", got.Cursor)
	}
}

// TestAudit_InsertFailureStillFinalizes 审计行插入失败仅记日志，收轮照完。
func TestAudit_InsertFailureStillFinalizes(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.runs.insertErr = errFakeInfra
	f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Status != bizsync.RunStatusSuccess || !out.Finalized {
		t.Fatalf("宁缺审计不留僵尸: %s finalized=%v", out.Status, out.Finalized)
	}
	if f.runs.count() != 0 {
		t.Fatalf("注入失败即无审计行: %d", f.runs.count())
	}
	if got := f.conns.get(f.conn.ID); got.SyncState != bizsync.SyncStateIdle || got.RunToken != "" {
		t.Fatalf("连接器照常收轮: %+v", got)
	}
}

// TestRepo_FailureBranches 仓口基础设施故障六分支归仓钉（#6）。
func TestRepo_FailureBranches(t *testing.T) {
	t.Run("connector_get", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.conns.getErr = errFakeInfra
		_, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if !errors.Is(err, errFakeInfra) {
			t.Fatalf("闸 1 仓口故障直传: %v", err)
		}
	})
	t.Run("item_get_aborts_walk", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.items.getErr = errFakeInfra
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkFailed {
			t.Fatalf("%s/%s", out.Status, out.ErrorCode)
		}
		if !errors.Is(out.Error, errFakeInfra) || !out.Finalized {
			t.Fatalf("cause/finalize: %v %v", out.Error, out.Finalized)
		}
	})
	t.Run("upsert_seen_aborts_walk", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.items.upsertSeenErr = errFakeInfra
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.Stats.Created != 0 || out.ErrorCode != bizsync.ErrCodeWalkFailed {
			t.Fatalf("出席失败不计工: %+v code=%s", out.Stats, out.ErrorCode)
		}
	})
	t.Run("mark_absent_confirm_failed", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.items.markAbsentErr = errFakeInfra
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.ErrorCode != bizsync.ErrCodeConfirmFailed || !errors.Is(out.Error, errFakeInfra) {
			t.Fatalf("圈定段归仓: code=%s cause=%v", out.ErrorCode, out.Error)
		}
	})
	t.Run("list_missing_confirm_failed", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.items.listErr = errFakeInfra
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.ErrorCode != bizsync.ErrCodeConfirmFailed || !errors.Is(out.Error, errFakeInfra) {
			t.Fatalf("拾取段归仓: code=%s cause=%v", out.ErrorCode, out.Error)
		}
	})
	t.Run("finalize_infra_keeps_outcome", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.conns.finalizeErr = errFakeInfra
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.Status != bizsync.RunStatusSuccess || out.Finalized || out.ClaimLost {
			t.Fatalf("收轮仓故障仅记日志，不得误报失地: %s finalized=%v claimLost=%v",
				out.Status, out.Finalized, out.ClaimLost)
		}
		if f.runs.count() != 1 {
			t.Fatalf("审计照落: %d", f.runs.count())
		}
	})
}

// TestOutcome_ErrorPlumbing RunOutcome.Error 贯通（#4：观测面死字段回填）。
func TestOutcome_ErrorPlumbing(t *testing.T) {
	t.Run("walk_abort_cause", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.walker.walkErr = errFakeInfra
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if !errors.Is(out.Error, errFakeInfra) {
			t.Fatalf("walker abort cause: %v", out.Error)
		}
	})
	t.Run("auth_gate_cause", func(t *testing.T) {
		f := newRunnerFixture(t, nil)
		f.walker.snaps = []Snapshot{{ExternalID: "n1", Fingerprint: "fp:n1"}}
		f.walker.fetchErrs = map[string]error{
			"n1": NewAuthError(bizsync.ErrCodeCredentialMissing, errFakeInfra),
		}
		out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
		if err != nil {
			t.Fatalf("SyncOnce err = %v", err)
		}
		if out.SyncState != bizsync.SyncStateAuthPaused || out.ErrorCode != bizsync.ErrCodeCredentialMissing {
			t.Fatalf("auth 闸面: %s/%s", out.SyncState, out.ErrorCode)
		}
		if code, isAuth := AuthCodeOf(out.Error); !isAuth || code != bizsync.ErrCodeCredentialMissing {
			t.Fatalf("cause 保 auth 粒度: %v", out.Error)
		}
		if got := f.conns.get(f.conn.ID); got.ConsecutiveFailures != f.conn.ConsecutiveFailures {
			t.Fatalf("auth 不入退避: %+v", got)
		}
	})
}
