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
// ⇒ 拒绝并回滚（防调用方把行建到别的寻址空间造成计数器与行分叉）。
// D7-f（评审确认项 D7-TQ-5）：谓词三子句（SessionID 不符 / Seq 不符 /
// EventID 空）逐句表驱动钉死——删任一子句对应子测试即红。
func TestEventOutboxAlloc_RejectsOffAddressRow(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name   string
		mutate func(row *biz.EventDeliveryOutboxRow, seq int64)
	}{
		{"wrong session", func(row *biz.EventDeliveryOutboxRow, seq int64) { row.SessionID = "sess-OTHER" }},
		{"wrong seq", func(row *biz.EventDeliveryOutboxRow, seq int64) { row.Seq = seq + 1 }},
		{"empty event id", func(row *biz.EventDeliveryOutboxRow, seq int64) { row.EventID = "  " }},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo, _ := newEventOutboxTestRepo(t)
			alloc := repo.(biz.EventOutboxSeqAllocatingInserter)

			err := alloc.InsertAllocatingSeq(ctx, "sess-y", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
				row, berr := d7AllocBuild("sess-y")(seq)
				if berr != nil {
					return row, berr
				}
				c.mutate(&row, seq) // 偏离分配寻址
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
		})
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

// --- D7-f 评审修复钉（wf_e35434d8-996 确认项） ---

// seedSeqCounter 直接写计数器行（模拟混版本窗口/异常态下的水位错位）。
func seedSeqCounter(t *testing.T, db *sql.DB, sessionID string, nextSeq int64) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO session_seq_counters (session_id, next_seq) VALUES ($1, $2)`,
		sessionID, nextSeq); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
}

func readSeqCounter(t *testing.T, db *sql.DB, sessionID string) int64 {
	t.Helper()
	var next int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT next_seq FROM session_seq_counters WHERE session_id = $1`, sessionID).Scan(&next); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return next
}

// TestEventOutboxAlloc_WatermarkConflictRepairBump 是 D7-f 钉（评审确认项
// D7-TQ-2：repair-bump 整链曾零覆盖——conflict 检测/有界重试/bump/同
// event_id 幂等四段任一被删全套件仍绿）。构造真水位冲突：存量行占 seq
// 1..3（异 event_id，模拟混版本窗口 Go 计数器副本直插）+ 计数器行落后
// （next_seq=2）⇒ 首次分配得 2 撞车 ⇒ repair-bump 顶到 MAX+2=5 ⇒ 重试
// 得 5 落库。断言：got=5、4 行无洞、计数器=6。
func TestEventOutboxAlloc_WatermarkConflictRepairBump(t *testing.T) {
	ctx := context.Background()
	repo, db := newEventOutboxTestRepo(t)
	for i := int64(1); i <= 3; i++ {
		if err := repo.Insert(ctx, biz.EventDeliveryOutboxRow{
			ID:        fmt.Sprintf("k-%d", i),
			SessionID: "sess-k",
			Seq:       i,
			EventID:   fmt.Sprintf("v2:sess-k:%d:task.completed:t", i),
			Kind:      "task.completed",
			Payload:   []byte(`{}`),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	seedSeqCounter(t, db, "sess-k", 2) // 计数器落后于持久水位（混版本窗口态）

	alloc := repo.(biz.EventOutboxSeqAllocatingInserter)
	var got int64
	if err := alloc.InsertAllocatingSeq(ctx, "sess-k", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		got = seq
		return d7AllocBuild("sess-k")(seq)
	}); err != nil {
		t.Fatalf("alloc with repair: %v", err)
	}
	if got != 5 {
		t.Fatalf("repair-bump 后分配 seq = %d, want 5（MAX(seq)+2=5）", got)
	}
	if next := readSeqCounter(t, db, "sess-k"); next != 6 {
		t.Fatalf("计数器 = %d, want 6（bump 到 5 后重试消耗至 6）", next)
	}
	rows, err := repo.ListPending(ctx, time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("durable rows = %d, want 4（3 存量 + 1 新落）", len(rows))
	}
}

// TestEventOutboxAlloc_SameEventIDConflictIdempotent：占号者同 event_id =
// 幂等重放成功（Insert 既有语义同律，崩溃-重试窗口）——nil error、零新增行。
// 钉死 insertAllocatingSeqOnce 的同 event_id 幂等分支（评审确认项 D7-TQ-2 副钉）。
func TestEventOutboxAlloc_SameEventIDConflictIdempotent(t *testing.T) {
	ctx := context.Background()
	repo, db := newEventOutboxTestRepo(t)
	const eventID = "v2:sess-i:1:task.completed:t"
	if err := repo.Insert(ctx, biz.EventDeliveryOutboxRow{
		ID:        "i-1",
		SessionID: "sess-i",
		Seq:       1,
		EventID:   eventID,
		Kind:      "task.completed",
		Payload:   []byte(`{"origin":true}`),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	seedSeqCounter(t, db, "sess-i", 1) // 计数器指向已被同 event_id 占用的 seq

	alloc := repo.(biz.EventOutboxSeqAllocatingInserter)
	var got int64
	err := alloc.InsertAllocatingSeq(ctx, "sess-i", func(seq int64) (biz.EventDeliveryOutboxRow, error) {
		got = seq
		row, berr := d7AllocBuild("sess-i")(seq)
		if berr != nil {
			return row, berr
		}
		row.EventID = eventID // 同 event_id 重放（崩溃-重试窗口语义）
		return row, nil
	})
	if err != nil {
		t.Fatalf("同 event_id 冲突应为幂等成功，got %v", err)
	}
	if got != 1 {
		t.Fatalf("重放分配 seq = %d, want 1", got)
	}
	rows, err := repo.ListPending(ctx, time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("幂等重放不得新增行：rows = %d, want 1", len(rows))
	}
}

// TestEventOutboxClaim_ConcurrentWorkersDisjoint 是 D7-f 钉（评审确认项
// D7-TQ-3：既有分片钉两 worker 顺序执行，SKIP LOCKED 从未被真实并发
// 触碰——删掉 FOR UPDATE SKIP LOCKED 全套件仍绿）。两 worker 屏障后并发
// 认领同一批行：返回集合必须互不相交、并集=全部可认领行（行锁互斥的结构
// 性验收；channel_delivery_claim_test.go:14 先例同式）。
func TestEventOutboxClaim_ConcurrentWorkersDisjoint(t *testing.T) {
	ctx := context.Background()
	repo1, db := newEventOutboxTestRepo(t)
	repo2 := secondEventOutboxRepo(db)
	claimer1 := repo1.(biz.EventOutboxClaimer)
	claimer2 := repo2.(biz.EventOutboxClaimer)

	const n = 12
	old := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		if err := repo1.Insert(ctx, biz.EventDeliveryOutboxRow{
			ID:        fmt.Sprintf("cc-%02d", i),
			SessionID: "sess-cc",
			Seq:       int64(i),
			EventID:   fmt.Sprintf("v2:sess-cc:%d:task.completed:t", i),
			Kind:      "task.completed",
			Payload:   []byte(`{}`),
			CreatedAt: old.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("seed cc-%02d: %v", i, err)
		}
	}
	cutoff := time.Now().UTC().Add(-time.Minute)
	freshDeadline := time.Now().UTC().Add(-time.Minute)

	// close(start) 屏障：两 goroutine 同一时刻进入认领（SKIP LOCKED 只在
	// 真实并发下被触碰；顺序调用形态下谓词单独即可分片，锁子句零 exercised）。
	start := make(chan struct{})
	var aIDs, bIDs []string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rows, err := claimer1.ClaimPendingOutboxDeliveries(ctx, "worker-a", cutoff, freshDeadline, n)
		if err != nil {
			t.Errorf("claim worker-a: %v", err)
			return
		}
		aIDs = claimIDs(rows)
	}()
	go func() {
		defer wg.Done()
		<-start
		rows, err := claimer2.ClaimPendingOutboxDeliveries(ctx, "worker-b", cutoff, freshDeadline, n)
		if err != nil {
			t.Errorf("claim worker-b: %v", err)
			return
		}
		bIDs = claimIDs(rows)
	}()
	close(start)
	wg.Wait()

	seen := make(map[string]string, n)
	for _, id := range aIDs {
		seen[id] = "a"
	}
	for _, id := range bIDs {
		if prev, dup := seen[id]; dup {
			t.Fatalf("行 %s 被 worker-a 与 worker-b 双认领（先前归 %s）——SKIP LOCKED 互斥被击穿", id, prev)
		}
		seen[id] = "b"
	}
	if len(seen) != n {
		t.Fatalf("认领并集 = %d 行, want %d（a=%v b=%v）", len(seen), n, aIDs, bIDs)
	}
}
