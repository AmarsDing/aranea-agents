package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- D7（P0-6 阶段 2）：事务内取号 + 认领化清扫 PG 语义钉（deferred 战役） ---
//
// 裸跑无 build tag（D6 先例）；PG 55432 缺席时与存量 8 项 PG 红同族
// （G-37 归口，容器修复后裸跑补绿）。

// secondEventOutboxRepo 从同一 *sql.DB 构造第二个 repo 实例——双副本连接面
// 等价物：计数器行锁串行化由 PG 服务端保证，同池并发事务即两个副本。
func secondEventOutboxRepo(db *sql.DB) biz.EventDeliveryOutboxRepo {
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return NewEventDeliveryOutboxRepo(d, loggateway.NewNoop())
}

func d7AllocBuild(sessionID string) func(seq int64) (biz.EventDeliveryOutboxRow, error) {
	return func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		return biz.EventDeliveryOutboxRow{
			SessionID: sessionID,
			Seq:       seq,
			EventID:   fmt.Sprintf("v2:%s:%d:task.completed:t", sessionID, seq),
			Kind:      "task.completed",
			EntityID:  "t",
			Payload:   []byte(fmt.Sprintf(`{"seq":%d}`, seq)),
			CreatedAt: time.Now().UTC(),
		}, nil
	}
}

// TestEventOutboxAlloc_ConcurrentReplicasUniqueGapless 是 D7-1 核心钉：两个
// repo（=两副本）对同一会话并发取号 40 次——计数器行锁串行化 ⇒ 发出的
// seq 恰好是 1..40 无重无洞（失败回滚不消耗号、成功提交号号相连），行数
// 与号数一致。可见序==取号序由此结构性成立：任一快照见到 N+1 必先见到 N
// （持锁事务先提交 N，N+1 的 UPSERT 才放行）。
func TestEventOutboxAlloc_ConcurrentReplicasUniqueGapless(t *testing.T) {
	ctx := context.Background()
	repo1, db := newEventOutboxTestRepo(t)
	repo2 := secondEventOutboxRepo(db)
	alloc1 := repo1.(biz.EventOutboxSeqAllocatingInserter)
	alloc2 := repo2.(biz.EventOutboxSeqAllocatingInserter)

	const workers = 4
	const perWorker = 10
	var mu sync.Mutex
	seqs := make([]int64, 0, workers*perWorker)
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			alloc := alloc1
			if w%2 == 1 {
				alloc = alloc2
			}
			build := d7AllocBuild("sess-d7")
			for i := 0; i < perWorker; i++ {
				var got int64
				err := alloc.InsertAllocatingSeq(ctx, "sess-d7", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
					got = seq
					return build(seq)
				})
				if err != nil {
					errCh <- fmt.Errorf("worker %d iter %d: %w", w, i, err)
					return
				}
				mu.Lock()
				seqs = append(seqs, got)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("alloc: %v", err)
	}

	if len(seqs) != workers*perWorker {
		t.Fatalf("allocated seqs = %d, want %d", len(seqs), workers*perWorker)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for i, s := range seqs {
		if s != int64(i+1) {
			t.Fatalf("seqs 非 1..N 连续（重号或洞）：idx %d = %d，全量 %v", i, s, seqs)
		}
	}
	rows, err := repo1.ListPending(ctx, time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != workers*perWorker {
		t.Fatalf("durable rows = %d, want %d", len(rows), workers*perWorker)
	}
}

// TestEventOutboxAlloc_SeedsAboveLegacyWatermark：off→pg 翻档时既有会话的
// 存量 Go 计数器行（seq 1..5）不被撞号——首分配种子 MAX(seq)+2 ⇒ 6。
func TestEventOutboxAlloc_SeedsAboveLegacyWatermark(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)
	for i := int64(1); i <= 5; i++ {
		row := biz.EventDeliveryOutboxRow{
			ID:        fmt.Sprintf("legacy-%d", i),
			SessionID: "sess-legacy",
			Seq:       i,
			EventID:   fmt.Sprintf("v2:sess-legacy:%d:task.completed:t", i),
			Kind:      "task.completed",
			Payload:   []byte(`{"legacy":true}`),
			CreatedAt: time.Now().UTC(),
		}
		if err := repo.Insert(ctx, row); err != nil {
			t.Fatalf("seed legacy row %d: %v", i, err)
		}
	}

	alloc := repo.(biz.EventOutboxSeqAllocatingInserter)
	var got int64
	if err := alloc.InsertAllocatingSeq(ctx, "sess-legacy", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		got = seq
		return d7AllocBuild("sess-legacy")(seq)
	}); err != nil {
		t.Fatalf("first pg-mode alloc: %v", err)
	}
	if got != 6 {
		t.Fatalf("首分配 seq = %d, want 6（存量水位 5 之上）", got)
	}
	rows, err := repo.ListPending(ctx, time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 6 {
		t.Fatalf("durable rows = %d, want 6", len(rows))
	}
}

// TestEventOutboxAlloc_BuildErrorRollsBack：build 失败整事务回滚——计数器
// 不消耗（下一次成功分配仍得 1）、无落库行。
func TestEventOutboxAlloc_BuildErrorRollsBack(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)
	alloc := repo.(biz.EventOutboxSeqAllocatingInserter)

	buildErr := errors.New("marshal boom")
	if err := alloc.InsertAllocatingSeq(ctx, "sess-x", func(int64) (biz.EventDeliveryOutboxRow, error) {
		return biz.EventDeliveryOutboxRow{}, buildErr
	}); !errors.Is(err, buildErr) {
		t.Fatalf("build 失败应原样上抛，got %v", err)
	}

	var got int64
	if err := alloc.InsertAllocatingSeq(ctx, "sess-x", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		got = seq
		return d7AllocBuild("sess-x")(seq)
	}); err != nil {
		t.Fatalf("second alloc: %v", err)
	}
	if got != 1 {
		t.Fatalf("回滚后首分配 seq = %d, want 1（计数器未被失败事务消耗）", got)
	}
	rows, err := repo.ListPending(ctx, time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("durable rows = %d, want 1", len(rows))
	}
}

// TestEventOutboxAlloc_RejectsOffAddressRow：build 产出行偏离分配寻址
// （SessionID/Seq 不符）⇒ 拒绝并回滚（防调用方把行建到别的寻址空间造成
// 计数器与行分叉）。
func TestEventOutboxAlloc_RejectsOffAddressRow(t *testing.T) {
	ctx := context.Background()
	repo, _ := newEventOutboxTestRepo(t)
	alloc := repo.(biz.EventOutboxSeqAllocatingInserter)

	err := alloc.InsertAllocatingSeq(ctx, "sess-y", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		row, berr := d7AllocBuild("sess-y")(seq)
		if berr != nil {
			return row, berr
		}
		row.SessionID = "sess-OTHER" // 偏离分配寻址
		return row, nil
	})
	if err == nil {
		t.Fatal("偏离寻址的行必须被拒绝")
	}

	var got int64
	if err := alloc.InsertAllocatingSeq(ctx, "sess-y", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		got = seq
		return d7AllocBuild("sess-y")(seq)
	}); err != nil {
		t.Fatalf("clean alloc: %v", err)
	}
	if got != 1 {
		t.Fatalf("拒绝回滚后首分配 seq = %d, want 1", got)
	}
}

// TestEventOutboxClaim_PartitionsLeaseReclaim 是 D7-2 核心钉：两 worker 认领
// 互斥分片（并集=全部、交集=空）；新鲜认领不被回收、租约过期可回收；
// 已发布行/太年轻行/空 workerID 三排除。
func TestEventOutboxClaim_PartitionsLeaseReclaim(t *testing.T) {
	ctx := context.Background()
	repo1, db := newEventOutboxTestRepo(t)
	repo2 := secondEventOutboxRepo(db)
	claimer1 := repo1.(biz.EventOutboxClaimer)
	claimer2 := repo2.(biz.EventOutboxClaimer)

	old := time.Now().UTC().Add(-time.Hour)
	seed := func(id string, seq int64, created time.Time) {
		row := biz.EventDeliveryOutboxRow{
			ID:        id,
			SessionID: "sess-c",
			Seq:       seq,
			EventID:   fmt.Sprintf("v2:sess-c:%d:task.completed:t", seq),
			Kind:      "task.completed",
			Payload:   []byte(fmt.Sprintf(`{"seq":%d}`, seq)),
			CreatedAt: created,
		}
		if err := repo1.Insert(ctx, row); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	// created_at 逐一错开保证 ORDER BY 确定性（同值并列序不定）。
	for i := int64(1); i <= 5; i++ {
		seed(fmt.Sprintf("row-%d", i), i, old.Add(time.Duration(i)*time.Second))
	}
	cutoff := time.Now().UTC().Add(-time.Minute)
	freshDeadline := time.Now().UTC().Add(-time.Minute) // 认领不足 1min = 未过期

	// workerA 认领 3（最老三行）；workerB 认领其余 2——互斥分片。
	// （UPDATE...RETURNING 行序不作契约定死，断言用有序 ID 集合。）
	a1, err := claimer1.ClaimPendingOutboxDeliveries(ctx, "worker-a", cutoff, freshDeadline, 3)
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	if got := claimIDsSorted(a1); !equalIDs(got, []string{"row-1", "row-2", "row-3"}) {
		t.Fatalf("workerA 认领 = %v, want [row-1 row-2 row-3]", got)
	}
	a2, err := claimer2.ClaimPendingOutboxDeliveries(ctx, "worker-b", cutoff, freshDeadline, 10)
	if err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if got := claimIDsSorted(a2); !equalIDs(got, []string{"row-4", "row-5"}) {
		t.Fatalf("workerB 认领 = %v, want [row-4 row-5]（A 已认领行互斥）", got)
	}

	// 新鲜认领不被回收（租约死线 1min，认领刚发生）。
	a3, err := claimer1.ClaimPendingOutboxDeliveries(ctx, "worker-a", cutoff, freshDeadline, 10)
	if err != nil {
		t.Fatalf("re-claim fresh: %v", err)
	}
	if len(a3) != 0 {
		t.Fatalf("新鲜认领不应被回收，got %v", claimIDs(a3))
	}

	// 已发布行永不出现在认领集。
	if err := repo1.MarkPublished(ctx, "row-4", time.Now().UTC()); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	// 租约过期（死线放到未来 = 全部认领视为过期）⇒ 可回收；row-4 已发布除外。
	a4, err := claimer2.ClaimPendingOutboxDeliveries(ctx, "worker-b", cutoff, time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("reclaim expired: %v", err)
	}
	if len(a4) != 4 {
		t.Fatalf("过期回收 = %v, want 4 行（row-4 已发布除外）", claimIDs(a4))
	}
	for _, row := range a4 {
		if row.ID == "row-4" {
			t.Fatalf("已发布行被认领：%v", claimIDs(a4))
		}
	}

	// 空 workerID = 误配 fail-closed。
	a5, err := claimer1.ClaimPendingOutboxDeliveries(ctx, "  ", cutoff, freshDeadline, 10)
	if err != nil || a5 != nil {
		t.Fatalf("空 workerID = (%v, %v), want (nil, nil)", a5, err)
	}

	// cutoff 尊重：刚创建的行（grace 窗口内）不被认领。
	seed("row-6", 6, time.Now().UTC())
	a6, err := claimer1.ClaimPendingOutboxDeliveries(ctx, "worker-a", cutoff, time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("claim after young row: %v", err)
	}
	for _, row := range a6 {
		if row.ID == "row-6" {
			t.Fatalf("grace 窗口内的年轻行被认领：%v", claimIDs(a6))
		}
	}
}

func claimIDs(rows []biz.EventDeliveryOutboxRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func claimIDsSorted(rows []biz.EventDeliveryOutboxRow) []string {
	ids := claimIDs(rows)
	sort.Strings(ids)
	return ids
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
