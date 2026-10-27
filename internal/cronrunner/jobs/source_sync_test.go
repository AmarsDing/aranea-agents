// source_sync_test.go —— W4 单例 fan-out worker 钉阵（fake 双缝全内存：
// due 拾取/崩溃回收前置/限流并发/single-flight/kill-switch 热生效/逾期合并单轮）
package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

// ──────────────────────────────── 双缝内存替身 ────────────────────────────────

// sweepOrder 双挂钩共用的调用序列记录器（PIN-4 评审修：崩溃回收前置序钉）。
type sweepOrder struct {
	mu  sync.Mutex
	seq []string
}

func (o *sweepOrder) record(tag string) {
	o.mu.Lock()
	o.seq = append(o.seq, tag)
	o.mu.Unlock()
}

func (o *sweepOrder) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.seq...)
}

type fakeSyncDispatcher struct {
	mu         sync.Mutex
	calls      map[string]int
	reclaims   int
	reclaimErr error
	reclaimCh  chan struct{} // PIN-5 钉：ReclaimZombies 进场信号（非阻塞投递）
	syncErr    map[string]error
	gate       map[string]chan struct{} // 闸：非 nil 时该 connector 阻塞至 close
	order      *sweepOrder              // 非 nil 时记录 reclaim 序列
	sleep      time.Duration
	cur, max   int
}

func newFakeSyncDispatcher() *fakeSyncDispatcher {
	return &fakeSyncDispatcher{calls: map[string]int{}, gate: map[string]chan struct{}{}}
}

func (f *fakeSyncDispatcher) ReclaimZombies(context.Context) (int, error) {
	f.mu.Lock()
	f.reclaims++
	if f.order != nil {
		f.order.record("reclaim")
	}
	if f.reclaimCh != nil {
		select {
		case f.reclaimCh <- struct{}{}:
		default:
		}
	}
	err := f.reclaimErr
	f.mu.Unlock()
	return 0, err
}

func (f *fakeSyncDispatcher) SyncOnce(_ context.Context, connectorID string) (knowledgesync.RunOutcome, error) {
	f.mu.Lock()
	f.calls[connectorID]++
	f.cur++
	if f.cur > f.max {
		f.max = f.cur
	}
	g := f.gate[connectorID]
	slp := f.sleep
	f.mu.Unlock()
	if g != nil {
		<-g
	}
	if slp > 0 {
		time.Sleep(slp)
	}
	f.mu.Lock()
	f.cur--
	err := f.syncErr[connectorID]
	f.mu.Unlock()
	if err != nil {
		return knowledgesync.RunOutcome{}, err
	}
	return knowledgesync.RunOutcome{ConnectorID: connectorID, Status: bizsync.RunStatusSuccess}, nil
}

func (f *fakeSyncDispatcher) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func (f *fakeSyncDispatcher) curCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cur
}

type fakeDueRepo struct {
	conns       []bizsync.Connector
	err         error
	panicInList bool // PIN-3 钉：ListDueConnectors panic 注入
	limitSeen   int
	order       *sweepOrder // 非 nil 时记录 listdue 序列
	mu          sync.Mutex
}

func (f *fakeDueRepo) ListDueConnectors(_ context.Context, _ time.Time, limit int) ([]bizsync.Connector, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.order != nil {
		f.order.record("listdue")
	}
	f.limitSeen = limit
	if f.panicInList {
		panic("due repo boom")
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.conns, nil
}

func dueConn(id string, kind bizsync.Kind) bizsync.Connector {
	return bizsync.Connector{ID: id, Kind: kind, Enabled: true}
}

func newSourceSyncWorker(d SourceSyncDispatcher, repo *fakeDueRepo) *SourceSyncWorker {
	w := NewSourceSyncWorker(time.Hour /* 不依赖 ticker 的手动 RunOnce */, d, repo, loggateway.NewNoop())
	return w
}

// ──────────────────────────────── 钉阵 ────────────────────────────────

// TestSourceSync_FanOutAllDueAndReclaimPrelude due 全量 fan-out（每行恰好一次）+
// 崩溃回收前置每个 sweep 先跑（序钉：reclaim 必在 listdue 之前）+ due 拾取上限
// 透传（32）。
func TestSourceSync_FanOutAllDueAndReclaimPrelude(t *testing.T) {
	d := newFakeSyncDispatcher()
	ord := &sweepOrder{}
	d.order = ord
	repo := &fakeDueRepo{conns: []bizsync.Connector{
		dueConn("c1", bizsync.KindSharedDir), dueConn("c2", bizsync.KindSharedDir), dueConn("c3", bizsync.KindNotion),
	}, order: ord}
	w := newSourceSyncWorker(d, repo)

	w.RunOnce(context.Background())
	if d.reclaims != 1 {
		t.Fatalf("崩溃回收前置: reclaims = %d want 1", d.reclaims)
	}
	seq := ord.snapshot()
	if len(seq) != 2 || seq[0] != "reclaim" || seq[1] != "listdue" {
		t.Fatalf("回收前置序钉: seq = %v want [reclaim listdue]", seq)
	}
	for _, id := range []string{"c1", "c2", "c3"} {
		if d.callCount(id) != 1 {
			t.Fatalf("逾期合并单轮钉: %s calls = %d want 1", id, d.callCount(id))
		}
	}
	if repo.limitSeen != sourceSyncDueLimit {
		t.Fatalf("due 上限透传: %d want %d", repo.limitSeen, sourceSyncDueLimit)
	}
	if w.InflightCount() != 0 {
		t.Fatalf("sweep 收尾 inflight 必清: %d", w.InflightCount())
	}
}

// TestSourceSync_SingleflightBlocksOverlap 进程内 single-flight：长轮跨 sweep
// 不双跑（W5 手动触发同入口共享闸）——阻塞在 A 期间第二 sweep 对 A 恰好跳过。
func TestSourceSync_SingleflightBlocksOverlap(t *testing.T) {
	d := newFakeSyncDispatcher()
	d.gate["c1"] = make(chan struct{})
	repo := &fakeDueRepo{conns: []bizsync.Connector{dueConn("c1", bizsync.KindSharedDir)}}
	w := newSourceSyncWorker(d, repo)

	done := make(chan struct{})
	go func() { w.RunOnce(context.Background()); close(done) }()
	// 清理闸（PIN-7 评审修）：任何断言失败路径都放行+汇合，杜绝泄露被闸住的
	// 后台 sweep goroutine。
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(d.gate["c1"]) })
		<-done
	})
	// 等 A 进场（inflight 可见；5s 上限防高负载 CI 调度抽搐误判）。
	for i := 0; i < 2500 && w.InflightCount() == 0; i++ {
		time.Sleep(2 * time.Millisecond)
	}
	if w.InflightCount() != 1 {
		t.Fatalf("A 应在飞: %d", w.InflightCount())
	}
	w.RunOnce(context.Background()) // 第二 sweep：A 必跳过
	if d.callCount("c1") != 1 {
		t.Fatalf("single-flight 重叠必跳: calls = %d want 1", d.callCount("c1"))
	}
	releaseOnce.Do(func() { close(d.gate["c1"]) })
	<-done
	if w.InflightCount() != 0 {
		t.Fatalf("收尾清零: %d", w.InflightCount())
	}
}

// TestSourceSync_GlobalKillSwitchHot SOURCE_SYNC_DISABLED 热生效：env 一贴
// 本 sweep 即静默（回收与拾取全停），撤贴后恢复。
func TestSourceSync_GlobalKillSwitchHot(t *testing.T) {
	t.Setenv("SOURCE_SYNC_DISABLED", "1")
	d := newFakeSyncDispatcher()
	repo := &fakeDueRepo{conns: []bizsync.Connector{dueConn("c1", bizsync.KindSharedDir)}}
	w := newSourceSyncWorker(d, repo)

	w.RunOnce(context.Background())
	if d.reclaims != 0 || d.callCount("c1") != 0 {
		t.Fatalf("全局 kill-switch 热断电: reclaims=%d calls=%d", d.reclaims, d.callCount("c1"))
	}
	t.Setenv("SOURCE_SYNC_DISABLED", "")
	w.RunOnce(context.Background())
	if d.callCount("c1") != 1 {
		t.Fatalf("撤贴恢复: calls = %d want 1", d.callCount("c1"))
	}
}

// TestSourceSync_KindKillSwitch SOURCE_SYNC_<KIND>_DISABLED 按 kind 闸：
// shared_dir 停、notion 不停，且热生效——撤贴下一 sweep 即恢复（PIN-2 评审修：
// kind 闸热读钉）；kind 跳过路径不得留 inflight 残位（漏槽即该连接器永久
// 单飞饿死）。
func TestSourceSync_KindKillSwitch(t *testing.T) {
	t.Setenv("SOURCE_SYNC_SHARED_DIR_DISABLED", "true")
	d := newFakeSyncDispatcher()
	repo := &fakeDueRepo{conns: []bizsync.Connector{
		dueConn("sd", bizsync.KindSharedDir), dueConn("nt", bizsync.KindNotion),
	}}
	w := newSourceSyncWorker(d, repo)

	w.RunOnce(context.Background())
	if d.callCount("sd") != 0 {
		t.Fatalf("shared_dir 必停: %d", d.callCount("sd"))
	}
	if d.callCount("nt") != 1 {
		t.Fatalf("notion 不同族不停: %d", d.callCount("nt"))
	}
	if w.InflightCount() != 0 {
		t.Fatalf("kind 跳过不得留 inflight 残位: %d", w.InflightCount())
	}
	// kind kill-switch 热生效钉：撤贴后下一 sweep 恢复。
	t.Setenv("SOURCE_SYNC_SHARED_DIR_DISABLED", "")
	w.RunOnce(context.Background())
	if d.callCount("sd") != 1 {
		t.Fatalf("撤贴恢复: sd calls = %d want 1", d.callCount("sd"))
	}
	if d.callCount("nt") != 2 {
		t.Fatalf("notion 跨 sweep 照跑: nt calls = %d want 2", d.callCount("nt"))
	}
}

// TestSourceSync_BoundedParallelism fan-out 并发上限=4（外部 API 配额美德闸）+
// 窗后 due 逐条全量 fan-out（8>4——截断到并发窗即饿死尾部，PIN-1/6 评审修：
// 屏障栅确定性钉，8 条同闸占满窗断言第 5 条不入，放闸后逐条恰好一次）。
func TestSourceSync_BoundedParallelism(t *testing.T) {
	d := newFakeSyncDispatcher()
	release := make(chan struct{})
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	var conns []bizsync.Connector
	for _, id := range ids {
		d.gate[id] = release
		conns = append(conns, dueConn(id, bizsync.KindSharedDir))
	}
	w := newSourceSyncWorker(d, &fakeDueRepo{conns: conns})

	done := make(chan struct{})
	go func() { w.RunOnce(context.Background()); close(done) }()
	// 清理闸（PIN-6 评审修）：任何断言失败路径都放闸+汇合，杜绝泄漏。
	var relOnce sync.Once
	t.Cleanup(func() { relOnce.Do(func() { close(release) }); <-done })
	// 首波恰 4 条占据并发窗（5s 上限防 CI 抽搐误判）。
	for i := 0; i < 2500 && d.curCount() < sourceSyncMaxParallel; i++ {
		time.Sleep(2 * time.Millisecond)
	}
	if d.curCount() != sourceSyncMaxParallel {
		t.Fatalf("首波必占满并发窗: cur = %d want %d", d.curCount(), sourceSyncMaxParallel)
	}
	total := 0
	for _, id := range ids {
		total += d.callCount(id)
	}
	if total != sourceSyncMaxParallel {
		t.Fatalf("第 5 条不得越窗: total = %d want %d", total, sourceSyncMaxParallel)
	}
	relOnce.Do(func() { close(release) })
	<-done
	if d.max != sourceSyncMaxParallel {
		t.Fatalf("并发上限钉: max = %d want %d", d.max, sourceSyncMaxParallel)
	}
	for _, id := range ids {
		if d.callCount(id) != 1 {
			t.Fatalf("窗后逐条全量 fan-out 钉: %s calls = %d want 1", id, d.callCount(id))
		}
	}
}

// TestSourceSync_FailureTolerance 故障面三钉合一：崩溃回收失败仅 Warn 不致死
// （sweep 照跑）；due 拾取失败本 sweep 空跑不 fan-out；SyncOnce 基础设施错误
// 圈禁单连接器不影响邻接 due。
func TestSourceSync_FailureTolerance(t *testing.T) {
	// 回收失败：sweep 继续
	d := newFakeSyncDispatcher()
	d.reclaimErr = errors.New("pg blink")
	repo := &fakeDueRepo{conns: []bizsync.Connector{dueConn("c1", bizsync.KindSharedDir)}}
	newSourceSyncWorker(d, repo).RunOnce(context.Background())
	if d.callCount("c1") != 1 {
		t.Fatalf("回收失败不致死: calls = %d", d.callCount("c1"))
	}

	// due 失败：零 fan-out
	d2 := newFakeSyncDispatcher()
	newSourceSyncWorker(d2, &fakeDueRepo{err: errors.New("pg down")}).RunOnce(context.Background())
	if d2.callCount("c1") != 0 || len(d2.calls) != 0 {
		t.Fatalf("due 失败零 fan-out: %+v", d2.calls)
	}

	// 单连接器基础设施错误圈禁
	d3 := newFakeSyncDispatcher()
	d3.syncErr = map[string]error{"bad": errors.New("locker down")}
	repo3 := &fakeDueRepo{conns: []bizsync.Connector{
		dueConn("bad", bizsync.KindSharedDir), dueConn("ok", bizsync.KindSharedDir),
	}}
	newSourceSyncWorker(d3, repo3).RunOnce(context.Background())
	if d3.callCount("ok") != 1 {
		t.Fatalf("邻接 due 不受池鱼: ok calls = %d", d3.callCount("ok"))
	}
}

// TestSourceSync_RoundPanicContained 桶级收尸钉：单连接器 SyncOnce panic 圈禁
// 单轮——sweep 照常收尾、邻接 due 不受影响、inflight 清零（goroutine panic
// 不外逸——主 goroutine recover 管不到子协程）。
func TestSourceSync_RoundPanicContained(t *testing.T) {
	d := newFakeSyncDispatcher()
	dsp := &panicDispatcher{fake: d, panicOn: map[string]bool{"boom": true}}
	repo := &fakeDueRepo{conns: []bizsync.Connector{
		dueConn("boom", bizsync.KindSharedDir), dueConn("ok", bizsync.KindSharedDir),
	}}
	w := newSourceSyncWorker(dsp, repo)

	w.RunOnce(context.Background()) // 不崩即过半
	if d.callCount("ok") != 1 {
		t.Fatalf("邻接 due 不受影响: ok calls = %d", d.callCount("ok"))
	}
	if w.InflightCount() != 0 {
		t.Fatalf("panic 轮 inflight 必清: %d", w.InflightCount())
	}
}

// panicDispatcher 对 panicOn 集合抛 panic，其余委托 fake（复用计数器）。
type panicDispatcher struct {
	fake    *fakeSyncDispatcher
	panicOn map[string]bool
}

func (p *panicDispatcher) ReclaimZombies(ctx context.Context) (int, error) {
	return p.fake.ReclaimZombies(ctx)
}

func (p *panicDispatcher) SyncOnce(ctx context.Context, connectorID string) (knowledgesync.RunOutcome, error) {
	if p.panicOn[connectorID] {
		panic("dispatcher boom")
	}
	return p.fake.SyncOnce(ctx, connectorID)
}

// TestSourceSync_TopSweepPanicContained 顶层 sweep 收尸钉（PIN-3 评审修）：
// 仓口链路 panic（ListDueConnectors/ReclaimZombies 驱动级）由 RunOnce 顶层
// recover 圈禁单轮——不外逸至死 ticker goroutine，下一 sweep 照常拾取
// （头部登记：单 sweep 失败不致死，下 tick 自然重试）。
func TestSourceSync_TopSweepPanicContained(t *testing.T) {
	d := newFakeSyncDispatcher()
	repo := &fakeDueRepo{
		conns:       []bizsync.Connector{dueConn("c1", bizsync.KindSharedDir)},
		panicInList: true,
	}
	w := newSourceSyncWorker(d, repo)

	w.RunOnce(context.Background()) // 不崩即过半（顶层 recover 圈禁本 sweep）
	if d.callCount("c1") != 0 {
		t.Fatalf("panic 轮必零分发: calls = %d", d.callCount("c1"))
	}
	if w.InflightCount() != 0 {
		t.Fatalf("panic 轮 inflight 必清: %d", w.InflightCount())
	}
	repo.mu.Lock()
	repo.panicInList = false
	repo.mu.Unlock()
	w.RunOnce(context.Background()) // 下 tick 自然重试钉
	if d.reclaims != 2 || d.callCount("c1") != 1 {
		t.Fatalf("下 tick 照常拾取: reclaims=%d calls=%d", d.reclaims, d.callCount("c1"))
	}
}

// TestSourceSync_StartContractImmediateSweepAndCancelReturn Start 合同钉
// （PIN-5 评审修：Start 此前零钉——该方法是 workers.go goAfterReady 唯一
// 入口）：首扫先行（不等首 tick 即崩溃回收+fan-out），ctx 取消即返回。
func TestSourceSync_StartContractImmediateSweepAndCancelReturn(t *testing.T) {
	d := newFakeSyncDispatcher()
	d.reclaimCh = make(chan struct{}, 1)
	repo := &fakeDueRepo{conns: []bizsync.Connector{dueConn("c1", bizsync.KindSharedDir)}}
	w := newSourceSyncWorker(d, repo) // poll=1h：reclaim 信号若到必=首扫先行

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Start(ctx); close(done) }()
	t.Cleanup(func() {
		cancel() // 任何失败路径兜底回收 Start goroutine
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	select {
	case <-d.reclaimCh:
	case <-time.After(5 * time.Second):
		t.Fatal("首扫先行钉：reclaim 信号超时（5s）")
	}
	// 等首扫 fan-out 落定再取消（fake 无闸立即完成，5s 上限防抖动）。
	for i := 0; i < 2500 && d.callCount("c1") == 0; i++ {
		time.Sleep(2 * time.Millisecond)
	}
	if d.callCount("c1") != 1 {
		t.Fatalf("首扫 fan-out 钉: calls = %d want 1", d.callCount("c1"))
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 Start 必返回（5s）")
	}
}

// TestSourceSync_NilDepsAndEnvTruth 构造闸：runner/due 任一 nil → nil worker
// （装配面省判）；envTruth 值域钉（on/yes/true/1 大小写免疫收，其余一律拒）。
func TestSourceSync_NilDepsAndEnvTruth(t *testing.T) {
	if NewSourceSyncWorker(0, nil, &fakeDueRepo{}, nil) != nil {
		t.Fatal("runner nil 必拒")
	}
	if NewSourceSyncWorker(0, newFakeSyncDispatcher(), nil, nil) != nil {
		t.Fatal("due nil 必拒")
	}
	if w := NewSourceSyncWorker(0, newFakeSyncDispatcher(), &fakeDueRepo{}, nil); w == nil || w.poll != SourceSyncDefaultPollInterval {
		t.Fatalf("零值规范化: %+v", w)
	}
	for _, on := range []string{"1", "true", "TRUE", "yes", "on", " On "} {
		if !envTruth(on) {
			t.Fatalf("envTruth(%q) 必收", on)
		}
	}
	for _, off := range []string{"", "0", "false", "no", "2", "truest"} {
		if envTruth(off) {
			t.Fatalf("envTruth(%q) 必拒", off)
		}
	}
	if SourceSyncKindDisabled("") {
		t.Fatal("空 kind 永不杀")
	}
}
