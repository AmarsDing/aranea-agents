// runner_test.go —— W2 ⑥ 八步序钉阵（无 PG 内存替身阵营；SQL CAS 真值钉在
// knowledge_source_sync_repo_test.go 挂起批）。每钉一案：断言 RunOutcome 状态机
// + 仓口副作用 + run 审计行 + 帧发射，四位一体。
package sourcesync

import (
	"context"
	"errors"
	"testing"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

// TestRunner_HappyPath 快乐路径：1 新 + 1 改 + 1 不变 + 1 Confirmed 自删。
func TestRunner_HappyPath(t *testing.T) {
	f := newRunnerFixture(t, nil)
	seenBase := fixedClock.Add(-2 * time.Hour)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "f1", Fingerprint: "v2", DocumentID: "doc:old1", ContentHash: "h1", OwnLifecycle: true, Status: bizsync.ItemStatusActive, LastSeenRunID: "ss:tok:prev", LastSeenAt: seenBase})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "f2", Fingerprint: "v1", DocumentID: "doc:f2", ContentHash: "h2", OwnLifecycle: true, Status: bizsync.ItemStatusActive, LastSeenRunID: "ss:tok:prev", LastSeenAt: seenBase})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "f3", Fingerprint: "v1", DocumentID: "doc:f3", ContentHash: "h3", OwnLifecycle: true, Status: bizsync.ItemStatusActive, LastSeenRunID: "ss:tok:prev", LastSeenAt: seenBase})
	f.walker.snaps = []Snapshot{
		{ExternalID: "f0", Fingerprint: "v1", DisplayPath: "/d/f0.md"}, // 新建
		{ExternalID: "f2", Fingerprint: "v2", DisplayPath: "/d/f2.md"}, // 指纹漂移=更新
		{ExternalID: "f3", Fingerprint: "v1", DisplayPath: "/d/f3.md"}, // 不变
	}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"f1": bizsync.VerdictConfirmed}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("SyncOnce err = %v", err)
	}
	if out.Skipped {
		t.Fatalf("unexpected skip: %s", out.SkipReason)
	}
	if out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("status = %s, want success", out.Status)
	}
	want := RunStats{PrevActive: 3, Seen: 3, Created: 1, Updated: 1, Unchanged: 1, Deleted: 1, BytesPulled: 32}
	if out.Stats != want {
		t.Fatalf("stats = %+v, want %+v", out.Stats, want)
	}
	if !out.Finalized || out.ClaimLost {
		t.Fatalf("finalize flags: finalized=%v claimLost=%v", out.Finalized, out.ClaimLost)
	}
	// 连接器终态：idle、游标推进、token 清位、next=now+interval、失败计数清零
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateIdle || conn.RunToken != "" {
		t.Fatalf("conn state = %s token=%q", conn.SyncState, conn.RunToken)
	}
	if conn.Cursor != "cur:n+1" {
		t.Fatalf("cursor = %q, want advanced", conn.Cursor)
	}
	if !conn.NextRunAt.Equal(fixedClock.Add(900 * time.Second)) {
		t.Fatalf("next_run_at = %v", conn.NextRunAt)
	}
	if conn.ConsecutiveFailures != 0 || !conn.LastSuccessAt.Equal(fixedClock) {
		t.Fatalf("success watermark wrong: failures=%d lastSuccess=%v", conn.ConsecutiveFailures, conn.LastSuccessAt)
	}
	// item 终态：f1 已删（applier 级联+MarkItemDeleted）；f2 hash 推进；f0 建档
	it1, _ := f.items.get(f.conn.ID, "f1")
	if it1.Status != bizsync.ItemStatusDeleted || it1.DeletedAt.IsZero() {
		t.Fatalf("f1 should be deleted-marked: %+v", it1)
	}
	if len(f.appl.deleteCalls) != 1 || f.appl.deleteCalls[0].ExternalID != "f1" {
		t.Fatalf("applier delete calls = %+v", f.appl.deleteCalls)
	}
	it0, _ := f.items.get(f.conn.ID, "f0")
	if it0.DocumentID != "doc:f0" || it0.ContentHash != "hash:v1" || !it0.OwnLifecycle {
		t.Fatalf("f0 binding = %+v", it0)
	}
	it2, _ := f.items.get(f.conn.ID, "f2")
	if it2.ContentHash != "hash:v2" || it2.Fingerprint != "v2" {
		t.Fatalf("f2 checkpoint = %+v", it2)
	}
	it3, _ := f.items.get(f.conn.ID, "f3")
	if it3.ContentHash != "h3" || it3.MissCount != 0 || it3.Status != bizsync.ItemStatusActive {
		t.Fatalf("f3 untouched semantics = %+v", it3)
	}
	// run 审计行
	if f.runs.count() != 1 {
		t.Fatalf("runs count = %d", f.runs.count())
	}
	run := f.runs.last()
	if run.Status != bizsync.RunStatusSuccess || run.Created != 1 || run.Deleted != 1 || run.ErrorCode != "" {
		t.Fatalf("run row = %+v", run)
	}
	// 帧：started 1 + finished 1
	if len(f.frames.started) != 1 || len(f.frames.finished) != 1 {
		t.Fatalf("frames = %d/%d", len(f.frames.started), len(f.frames.finished))
	}
}

// TestRunner_WalkEmptyAbort ⑥-4 通用闸：空 walk ∧ 上轮 active>0 → failed +
// walk_empty_abort + 零删除零圈定 + 退避推进。
func TestRunner_WalkEmptyAbort(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.ConsecutiveFailures = 1 })
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "x1", Status: bizsync.ItemStatusActive, DocumentID: "d1", ContentHash: "h", Fingerprint: "v1"})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "x2", Status: bizsync.ItemStatusActive, DocumentID: "d2", ContentHash: "h", Fingerprint: "v1"})
	f.walker.snaps = nil
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"x1": bizsync.VerdictConfirmed}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeWalkEmptyAbort {
		t.Fatalf("outcome = %s/%s", out.Status, out.ErrorCode)
	}
	if out.Stats.Deleted != 0 || len(f.appl.deleteCalls) != 0 {
		t.Fatalf("deletion phase must be skipped")
	}
	it1, _ := f.items.get(f.conn.ID, "x1")
	if it1.Status != bizsync.ItemStatusActive {
		t.Fatalf("x1 must stay active (no absent mark): %+v", it1.Status)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateError || conn.ConsecutiveFailures != 2 {
		t.Fatalf("conn = %s failures=%d", conn.SyncState, conn.ConsecutiveFailures)
	}
	if !conn.NextRunAt.Equal(fixedClock.Add(4 * time.Minute)) { // Backoff(2)=4m
		t.Fatalf("backoff next = %v", conn.NextRunAt)
	}
	if conn.Cursor != "" {
		t.Fatalf("cursor must stay: %q", conn.Cursor)
	}
}

// TestRunner_AuthGateFromWalk ⑥-4：walker 自体 AuthError → auth_paused +
// 零删除 + 游标不动 + 不动 next_run_at + 不入退避。
func TestRunner_AuthGateFromWalk(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.ConsecutiveFailures = 3 })
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "x1", Status: bizsync.ItemStatusActive, DocumentID: "d1", ContentHash: "h", Fingerprint: "v1"})
	f.walker.walkErr = NewAuthError("", errors.New("HTTP 401 unauthorized"))
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"x1": bizsync.VerdictConfirmed}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusFailed || out.ErrorCode != bizsync.ErrCodeAuthFailed {
		t.Fatalf("outcome = %s/%s", out.Status, out.ErrorCode)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateAuthPaused {
		t.Fatalf("state = %s, want auth_paused", conn.SyncState)
	}
	if conn.ConsecutiveFailures != 3 {
		t.Fatalf("auth must not enter backoff: failures=%d", conn.ConsecutiveFailures)
	}
	if !conn.NextRunAt.Equal(f.conn.NextRunAt) {
		t.Fatalf("auth must not touch next_run_at: %v", conn.NextRunAt)
	}
	if conn.Cursor != "" {
		t.Fatalf("cursor must stay: %q", conn.Cursor)
	}
	it1, _ := f.items.get(f.conn.ID, "x1")
	if it1.Status != bizsync.ItemStatusActive {
		t.Fatalf("deletion skipped: x1 status = %s", it1.Status)
	}
	run := f.runs.last()
	if run.Status != bizsync.RunStatusFailed || run.ErrorCode != bizsync.ErrCodeAuthFailed {
		t.Fatalf("run row = %+v", run)
	}
}

// TestRunner_AuthGateFromFetch ⑥-4：Fetch 内 403 → emit 原样返回 → walk 中止
// → 同一 auth 闸；已出席的 item 标记保留（出席是事实）。
func TestRunner_AuthGateFromFetch(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.walker.snaps = []Snapshot{
		{ExternalID: "ok1", Fingerprint: "v1"},
		{ExternalID: "denied1", Fingerprint: "v1"},
		{ExternalID: "never", Fingerprint: "v1"},
	}
	f.walker.fetchErrs = map[string]error{"denied1": NewAuthError("", errors.New("HTTP 403 forbidden"))}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.ErrorCode != bizsync.ErrCodeAuthFailed {
		t.Fatalf("code = %s", out.ErrorCode)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateAuthPaused {
		t.Fatalf("state = %s", conn.SyncState)
	}
	itOK, ok := f.items.get(f.conn.ID, "ok1")
	if !ok || itOK.Status != bizsync.ItemStatusActive {
		t.Fatalf("ok1 attendance must persist: %+v", itOK)
	}
	if _, ok := f.items.get(f.conn.ID, "never"); ok {
		t.Fatalf("never must not be marked (walk aborted before it)")
	}
	if f.walker.fetchCalls["never"] != 0 {
		t.Fatalf("fetch must stop at auth failure")
	}
}

// TestRunner_AuthCodeGranularity credential 家族码保粒度晋级。
func TestRunner_AuthCodeGranularity(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.walker.walkErr = NewAuthError(bizsync.ErrCodeCredentialMissing, errors.New("no credential row"))

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.ErrorCode != bizsync.ErrCodeCredentialMissing {
		t.Fatalf("code = %s, want credential_missing", out.ErrorCode)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.SyncState != bizsync.SyncStateAuthPaused {
		t.Fatalf("state = %s", conn.SyncState)
	}
}

// TestRunner_UnknownStaysMissing ⑥-5：Unknown/StillExists 一律保留 missing；
// miss_unknown 合并计数；零删除。
func TestRunner_UnknownStaysMissing(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "gone1", Status: bizsync.ItemStatusActive, DocumentID: "d1", ContentHash: "h", Fingerprint: "v1", OwnLifecycle: true})
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "gone2", Status: bizsync.ItemStatusActive, DocumentID: "d2", ContentHash: "h", Fingerprint: "v1", OwnLifecycle: true})
	f.walker.snaps = []Snapshot{{ExternalID: "alive", Fingerprint: "v1"}}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{
		"gone1": bizsync.VerdictUnknown,
		"gone2": bizsync.VerdictStillExists,
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("status = %s (miss_unknown 不降级轮评级)", out.Status)
	}
	if out.Stats.MissUnknown != 2 || out.Stats.Deleted != 0 {
		t.Fatalf("stats = %+v", out.Stats)
	}
	for _, id := range []string{"gone1", "gone2"} {
		it, _ := f.items.get(f.conn.ID, id)
		if it.Status != bizsync.ItemStatusMissing || it.MissCount != 1 {
			t.Fatalf("%s must stay missing: %+v", id, it)
		}
	}
	if len(f.appl.deleteCalls) != 0 {
		t.Fatalf("no cascade on non-confirmed")
	}
}

// TestRunner_DeleteBudgetTruncation ⑥-6：Confirmed 超 max_deletes → 截断，
// 余数留 missing 下轮再议。
func TestRunner_DeleteBudgetTruncation(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.MaxDeletesPerRun = 2 })
	for _, id := range []string{"g1", "g2", "g3", "g4"} {
		f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: id, Status: bizsync.ItemStatusActive, DocumentID: "d:" + id, ContentHash: "h", Fingerprint: "v1", OwnLifecycle: true})
	}
	f.walker.snaps = []Snapshot{{ExternalID: "alive", Fingerprint: "v1"}}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{
		"g1": bizsync.VerdictConfirmed, "g2": bizsync.VerdictConfirmed,
		"g3": bizsync.VerdictConfirmed, "g4": bizsync.VerdictConfirmed,
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Stats.Deleted != 2 {
		t.Fatalf("deleted = %d, want budget-capped 2", out.Stats.Deleted)
	}
	left, _ := f.items.ListItemsByStatus(context.Background(), f.conn.ID, bizsync.ItemStatusMissing, 0)
	if len(left) != 2 {
		t.Fatalf("budget overflow must stay missing: %d left", len(left))
	}
	if len(f.appl.deleteCalls) != 2 {
		t.Fatalf("cascade calls = %d", len(f.appl.deleteCalls))
	}
}

// TestRunner_WorkBudgetPartial ⑥-6 工作预算：max_items 触顶 → 哨兵中止 walk、
// 游标不动、partial 收轮、零删除；未处理项留空绑定下轮续传。
func TestRunner_WorkBudgetPartial(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.MaxItemsPerRun = 1 })
	f.walker.snaps = []Snapshot{
		{ExternalID: "n1", Fingerprint: "v1"},
		{ExternalID: "n2", Fingerprint: "v1"},
		{ExternalID: "n3", Fingerprint: "v1"},
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusPartial {
		t.Fatalf("status = %s, want partial", out.Status)
	}
	if out.Stats.Created != 1 {
		t.Fatalf("created = %d, want budget 1", out.Stats.Created)
	}
	conn := f.conns.get(f.conn.ID)
	if conn.Cursor != "" {
		t.Fatalf("cursor must not advance on budget abort: %q", conn.Cursor)
	}
	if conn.SyncState != bizsync.SyncStateIdle || conn.ConsecutiveFailures != 0 {
		t.Fatalf("budget partial keeps healthy cadence: %s/%d", conn.SyncState, conn.ConsecutiveFailures)
	}
	if !conn.NextRunAt.Equal(fixedClock.Add(900 * time.Second)) {
		t.Fatalf("next = %v", conn.NextRunAt)
	}
	if _, ok := f.items.get(f.conn.ID, "n2"); ok {
		t.Fatalf("n2 must not be marked (budget abort before upsert)")
	}
	if f.walker.fetchCalls["n3"] != 0 {
		t.Fatalf("walk stopped at sentinel")
	}
}

// TestRunner_WorkBudgetPreservesFingerprint 失败/预算跳过的既有项保留旧指纹，
// 下轮指纹漂移判定自然重试（needsWork=fingerprint 漂移）。
func TestRunner_WorkBudgetPreservesFingerprint(t *testing.T) {
	f := newRunnerFixture(t, func(c *bizsync.Connector) { c.MaxItemsPerRun = 1 })
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "old1", Status: bizsync.ItemStatusActive, DocumentID: "doc:old1", ContentHash: "h1", Fingerprint: "v1", OwnLifecycle: true})
	f.walker.snaps = []Snapshot{
		{ExternalID: "first", Fingerprint: "v9"},
		{ExternalID: "old1", Fingerprint: "v2"}, // 漂移但预算已被 first 吃光
	}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Status != bizsync.RunStatusPartial {
		t.Fatalf("status = %s", out.Status)
	}
	it, _ := f.items.get(f.conn.ID, "old1")
	if it.Fingerprint != "v1" {
		t.Fatalf("budget-skipped item must keep old fingerprint: %+v", it)
	}
}

// TestRunner_AdoptSafety P3：own_lifecycle=false（adopt 接管豁免）项 Confirmed
// 时只 MarkItemDeleted，绝不动 applier.Delete（文档归原主）。
func TestRunner_AdoptSafety(t *testing.T) {
	f := newRunnerFixture(t, nil)
	f.items.seed(bizsync.Item{ConnectorID: f.conn.ID, ExternalID: "adopted", Status: bizsync.ItemStatusActive, DocumentID: "doc:shared", ContentHash: "h", Fingerprint: "v1", OwnLifecycle: false})
	f.walker.snaps = []Snapshot{{ExternalID: "alive", Fingerprint: "v1"}}
	f.walker.verdicts = map[string]bizsync.DeletionVerdict{"adopted": bizsync.VerdictConfirmed}

	out, err := f.runner.SyncOnce(context.Background(), f.conn.ID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Stats.Deleted != 1 {
		t.Fatalf("deleted = %d", out.Stats.Deleted)
	}
	if len(f.appl.deleteCalls) != 0 {
		t.Fatalf("adopt item must NOT cascade-delete document: %+v", f.appl.deleteCalls)
	}
	it, _ := f.items.get(f.conn.ID, "adopted")
	if it.Status != bizsync.ItemStatusDeleted {
		t.Fatalf("item row deleted-marked: %+v", it)
	}
}
