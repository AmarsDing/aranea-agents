// source_sync.go —— G-38 W4 SourceSync 单例 fan-out worker（设计 ⑤ 接入面 B 定型：
// KnowledgeCurate 模板 + due 拾取走 next_run_at 部分索引 + per-connector 跨副本
// 排他在 Runner.SyncOnce 内部[lockKeyPrefix=knowledge-sourcesync:<id>] + 进程内
// single-flight + workers.go goAfterReady 注册 + env kill-switch）。
//
// W4 as-built 登记（design 条文以条目注册）：
//   - 调度语义：ticker 30s 一轮 sweep（SourceSyncDefaultPollInterval——due 拾取
//     延迟上限；min interval 900s 的 connector 粒度下 30s 扫描是 3% 开销比）；
//     sweep＝崩溃回收前置（ReclaimZombies——⑤ 观测+抢救面，成功计数由 runner
//     独家打点[W4-JOB-2 评审修：worker 同名 Warn 双发即删]，失败仅 Warn 继续，
//     due 拾取共用 DB 故障会自然再失败）＋ListDueConnectors（部分索引 SQL，
//     禁 Go 全表内存筛）＋due 行逐条 fan-out。
//   - skipped_missed 宕机对齐（⑤）：逾期 connector 每 sweep 恰好补一轮——
//     due 列表按 next_run_at ASC LIMIT 返回（W1 仓口契约），本 worker 一轮对
//     一行至多一次 SyncOnce，无 backlog 追跑循环（walk 全量对账天然幂等）；
//     finalize 把 next_run_at=now+interval 推进到未来，逾期全部坍缩进这一轮。
//   - 进程内 single-flight：inflight map 防同 connector 并发双跑（本 ticker 串行
//     天然不撞，此闸为 W5 手动触发共用 SyncOnce 入口预留 + 长轮跨 sweep 防御）；
//     跨副本真排在 Runner 内部 advisory lock（fail-closed）。
//   - 限流 fan-out：maxParallel=4 并发上限（外部 API 配额美德；shared_dir 本地盘
//     不受限但统一闸免 per-kind 分叉调参——登记可调参）；dueLimit=32 单 sweep
//     拾取上限（超出随 next_run_at ASC 序下 tick 自然续——登记可调参）。
//   - kill-switch 热生效（登记裁决）：SOURCE_SYNC_DISABLED 全局 +
//     SOURCE_SYNC_<KIND>_DISABLED 按 kind，**每 sweep 重读 env**（配额/凭据事故
//     的停机手段不应要求进程重启；30s 周期读 env 零成本）。构造期检查保留于
//     wire provider（禁用时整个 worker 不装配，KnowledgeCurate 同款）。
//   - 退避分级表不在本层（as-built 消歧）：凭据直置 auth_paused、walk 失败
//     2^n min cap 60、死信与审计均由 W2 runner finalize + runs 表承载；本
//     worker 只做拾取/分发/观察日志，不二次重试（in-tick retry 会与 claim
//     CAS/租约语义打架——首轮持有 token，重试必撞 locked）。
//   - 结果打点分级：skipped（locked 等抢常态）/全不变成功=Debug；有活动量
//     成功=Info；failed/partial 或 SyncOnce 返回非 nil error（锁面/仓口故障
//     fail-closed 面）=Warn——30s 周期下静默路径必须零日志洪。
package jobs

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

const (
	// SourceSyncDefaultPollInterval due 拾取周期（W4 登记：30s——connector 最小
	// interval 900s 对应的拾取延迟上限足够低，DB 开销一次部分索引点查）。
	SourceSyncDefaultPollInterval = 30 * time.Second
	// sourceSyncDueLimit 单 sweep due 拾取上限（W4 登记可调参；超出按
	// next_run_at ASC 序下一 sweep 自然续）。
	sourceSyncDueLimit = 32
	// sourceSyncMaxParallel fan-out 并发上限（W4 登记可调参——外部 API 配额美德，
	// 全 kind 统一闸免分叉调参）。
	sourceSyncMaxParallel = 4
)

// SourceSyncDispatcher 轮执行面（生产实现：*knowledgesync.Runner）。
// Stability:internal（W5+ 手动触发同入口）。
type SourceSyncDispatcher interface {
	ReclaimZombies(ctx context.Context) (int, error)
	SyncOnce(ctx context.Context, connectorID string) (knowledgesync.RunOutcome, error)
}

// SourceSyncDueRepo due 拾取面（生产实现：data.NewSourceSyncRepo 聚合仓）。
// Stability:internal
type SourceSyncDueRepo interface {
	ListDueConnectors(ctx context.Context, now time.Time, limit int) ([]bizsync.Connector, error)
}

// SourceSyncWorker 单例 fan-out 调度器（无锁内部状态——inflight 单 flight 闸
// 是唯一可变态，mutex 保卫；并发安全由构造期单例+runner 内部 CAS 承载）。
// 实现 jobs.BackgroundStarter（Start(ctx) 阻塞至 ctx 取消）。
type SourceSyncWorker struct {
	poll time.Duration
	run  SourceSyncDispatcher
	due  SourceSyncDueRepo
	lg   loggateway.Logger
	now  func() time.Time

	mu       sync.Mutex
	inflight map[string]struct{}
}

// NewSourceSyncWorker 构造。runner/due 任一 nil 返回 nil（装配面省 nil 检查，
// KnowledgeCurate 同款）；poll≤0 取默认 30s。
func NewSourceSyncWorker(
	poll time.Duration,
	runner SourceSyncDispatcher,
	due SourceSyncDueRepo,
	lg loggateway.Logger,
) *SourceSyncWorker {
	if runner == nil || due == nil {
		return nil
	}
	if poll <= 0 {
		poll = SourceSyncDefaultPollInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SourceSyncWorker{
		poll:     poll,
		run:      runner,
		due:      due,
		lg:       lg.With(loggateway.Domain("knowledge/sourcesync/worker")),
		now:      time.Now,
		inflight: map[string]struct{}{},
	}
}

// Start 周期 sweep（首扫先行——启动即崩溃回收+due 拾取），ctx 取消即返回。
func (w *SourceSyncWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	w.lg.Info("sourcesync worker started", loggateway.Str("poll", w.poll.String()))
	w.RunOnce(ctx)
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce 单轮 sweep：kill-switch 热检 → 崩溃回收前置（观测+抢救，失败仅
// Warn）→ due 拾取 → 限流并发 fan-out。panic 收尸记 Error（KnowledgeCurate
// 模板同款），单 sweep 失败不致死——下 tick 自然重试。
func (w *SourceSyncWorker) RunOnce(ctx context.Context) {
	if w == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			w.lg.Error("sourcesync sweep panic recovered", loggateway.Any("panic", rec))
		}
	}()
	if SourceSyncDisabled() {
		w.lg.Debug("sourcesync sweep skipped: global kill-switch")
		return
	}
	if _, err := w.run.ReclaimZombies(ctx); err != nil {
		w.lg.Warn("sourcesync zombie reclaim failed", loggateway.Err(err))
	}
	due, err := w.due.ListDueConnectors(ctx, w.now(), sourceSyncDueLimit)
	if err != nil {
		w.lg.Warn("sourcesync due pickup failed", loggateway.Err(err))
		return
	}
	if len(due) == 0 {
		return
	}
	sem := make(chan struct{}, sourceSyncMaxParallel)
	var wg sync.WaitGroup
	for i := range due {
		conn := due[i]
		if SourceSyncKindDisabled(conn.Kind) {
			w.lg.Debug("sourcesync kind kill-switch skip",
				loggateway.Str("kind", string(conn.Kind)),
				loggateway.Str("connector_id", conn.ID))
			continue
		}
		if !w.enterInflight(conn.ID) {
			w.lg.Debug("sourcesync single-flight skip (round in flight)",
				loggateway.Str("connector_id", conn.ID))
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer w.leaveInflight(conn.ID)
			defer func() {
				if rec := recover(); rec != nil {
					// 桶级收尸：goroutine panic 不外逸（主 goroutine 的 defer recover
					// 管不到子协程——不拦即整进程崩；单轮病态圈禁单连接器）。
					w.lg.Error("sourcesync round panic contained",
						loggateway.Str("connector_id", conn.ID), loggateway.Any("panic", rec))
				}
			}()
			w.runOne(ctx, conn)
		}()
	}
	wg.Wait()
}

// runOne 单连接器分发+结果分级打点（退避/停轮语义全部在 runner finalize，
// 本层不重试不裁决）。
func (w *SourceSyncWorker) runOne(ctx context.Context, conn bizsync.Connector) {
	lg := w.lg.With(loggateway.Str("connector_id", conn.ID), loggateway.Str("kind", string(conn.Kind)))
	out, err := w.run.SyncOnce(ctx, conn.ID)
	if err != nil {
		// 非 nil error 只限锁面/仓口基础设施故障（fail-closed 面）——告警级。
		lg.Warn("sourcesync round infrastructure failure", loggateway.Err(err))
		return
	}
	if out.Skipped {
		if out.SkipReason != "disabled" && out.SkipReason != "not_found" {
			lg.Debug("sourcesync round skipped", loggateway.Str("reason", out.SkipReason))
		}
		return
	}
	st := out.Stats
	active := st.Created + st.Updated + st.Deleted + st.FetchFailed
	fields := []loggateway.Field{
		loggateway.Str("status", string(out.Status)),
		loggateway.Str("error_code", string(out.ErrorCode)),
		loggateway.Int("seen", st.Seen),
		loggateway.Int("created", st.Created),
		loggateway.Int("updated", st.Updated),
		loggateway.Int("unchanged", st.Unchanged),
		loggateway.Int("fetch_failed", st.FetchFailed),
		loggateway.Int("deleted", st.Deleted),
		loggateway.Int("miss_unknown", st.MissUnknown),
		loggateway.Int64("bytes", st.BytesPulled),
	}
	switch {
	case out.Status == bizsync.RunStatusFailed || out.Status == bizsync.RunStatusPartial:
		lg.Warn("sourcesync round finished", fields...)
	case active > 0:
		lg.Info("sourcesync round finished", fields...)
	default:
		lg.Debug("sourcesync round idle",
			loggateway.Str("status", string(out.Status)), loggateway.Int("unchanged", st.Unchanged))
	}
}

// enterInflight 进程内 single-flight 进场（已在飞=false）。
func (w *SourceSyncWorker) enterInflight(connectorID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.inflight[connectorID]; ok {
		return false
	}
	w.inflight[connectorID] = struct{}{}
	return true
}

func (w *SourceSyncWorker) leaveInflight(connectorID string) {
	w.mu.Lock()
	delete(w.inflight, connectorID)
	w.mu.Unlock()
}

// InflightCount 观测面（测试与运维钉用）。
func (w *SourceSyncWorker) InflightCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.inflight)
}

// SourceSyncDisabled 全局 kill-switch（SOURCE_SYNC_DISABLED）。
func SourceSyncDisabled() bool {
	return envTruth(os.Getenv("SOURCE_SYNC_DISABLED"))
}

// SourceSyncKindDisabled 按 kind kill-switch（SOURCE_SYNC_<KIND>_DISABLED，
// kind 大写——shared_dir→SOURCE_SYNC_SHARED_DIR_DISABLED）。
func SourceSyncKindDisabled(kind bizsync.Kind) bool {
	k := strings.ToUpper(strings.TrimSpace(string(kind)))
	if k == "" {
		return false
	}
	return envTruth(os.Getenv("SOURCE_SYNC_" + k + "_DISABLED"))
}

func envTruth(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// SourceSyncKindGAOptIn 按 kind GA 显式开闸（SOURCE_SYNC_<KIND>_ENABLED，
// 默认关）——W6 notion GA 门（P2 拍板上调三层：T4 十三步接线 ✓ + W9 对抗评审
// gate + 至少一个 shared_dir 生产浸泡周期）未达前，装配面据此**不挂** notion
// walker（进程内编译期装配，开闸须重启——与 DISABLED 每 sweep 热读正交：
// 一个管装配准入，一个管运行时事故停机）。W9 收口岸翻转此默认。
func SourceSyncKindGAOptIn(kind bizsync.Kind) bool {
	k := strings.ToUpper(strings.TrimSpace(string(kind)))
	if k == "" {
		return false
	}
	return envTruth(os.Getenv("SOURCE_SYNC_" + k + "_ENABLED"))
}
