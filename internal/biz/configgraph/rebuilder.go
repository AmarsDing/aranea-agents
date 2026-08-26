package configgraph

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// flowStepRebuild 是全量重建的 flowlog step（design §4.1 step 6）。
const flowStepRebuild = "configgraph.rebuild"

// RebuildResult 汇总一次全量重建的产出（status API / flowlog 共用）。
type RebuildResult struct {
	Generation int64
	Nodes      int
	Edges      int
	Broken     int
	Elapsed    time.Duration
}

// Rebuilder 全量重建器（design §4.1）：gen=current+1 写新代 → 切换内存当前
// 代 → 异步清理 gen-1 之前的旧代。查询恒读 Current() 代，重建期间旧代可
// 读，无清表窗口。
//
// 幂等：节点/边 ID 是 (type,ref)/(src,dst,type) 的确定性 uuid5，任意时刻
// 重跑产出字节一致（upsert 冲突即刷新）。
//
// 并发：Rebuild 串行（互斥锁）；重建内部仅 agentExtractor 的
// GetEffectiveTools 扇出并发 8。
type Rebuilder struct {
	src      SourceRepo
	repo     Repo
	provider EffectiveToolsProvider
	flowLog  monitor.FlowLogWriter // nil-safe
	lg       loggateway.Logger

	mu      sync.Mutex // 串行化并发 Rebuild
	current atomic.Int64
	ready   atomic.Bool
	running atomic.Bool
	lastOK  atomic.Pointer[rebuildSnapshot]
}

// rebuildSnapshot 记录最近一次成功重建的结果与完成时刻（status API 用）。
type rebuildSnapshot struct {
	res RebuildResult
	at  time.Time
}

// NewRebuilder 构造重建器。src/repo 任一为空则返回 nil（装配侧判空）。
func NewRebuilder(src SourceRepo, repo Repo, provider EffectiveToolsProvider, flowLog monitor.FlowLogWriter, lg loggateway.Logger) *Rebuilder {
	if src == nil || repo == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Rebuilder{src: src, repo: repo, provider: provider, flowLog: flowLog, lg: lg.With(loggateway.Domain("config_graph"))}
}

// Init 启动时从 repo.MaxGeneration 播种内存当前代（design §2.4）。播种失败
// 不致命——首次 Rebuild 会惰性补播。
func (r *Rebuilder) Init(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.seedGeneration(ctx)
}

func (r *Rebuilder) seedGeneration(ctx context.Context) error {
	max, err := r.repo.MaxGeneration(ctx)
	if err != nil {
		return err
	}
	r.current.Store(max)
	r.ready.Store(max > 0)
	return nil
}

// Current 返回当前代（查询侧唯一读取点）；未建图时为 0。
func (r *Rebuilder) Current() int64 {
	if r == nil {
		return 0
	}
	return r.current.Load()
}

// Ready 报告是否已有可用代（NOT_READY 判据，design §4.3）。
func (r *Rebuilder) Ready() bool {
	if r == nil {
		return false
	}
	return r.ready.Load()
}

// Running 报告是否正有重建在途（status API 用）。
func (r *Rebuilder) Running() bool {
	if r == nil {
		return false
	}
	return r.running.Load()
}

// LastRebuild 返回最近一次成功重建的结果与完成时刻；从未成功重建时 ok=false。
func (r *Rebuilder) LastRebuild() (res RebuildResult, at time.Time, ok bool) {
	if r == nil {
		return RebuildResult{}, time.Time{}, false
	}
	snap := r.lastOK.Load()
	if snap == nil {
		return RebuildResult{}, time.Time{}, false
	}
	return snap.res, snap.at, true
}

// rebuildTimeout 是异步重建的兜底时限（detach 自请求 ctx，不能随请求取消）。
const rebuildTimeout = 10 * time.Minute

// RebuildAsync 在后台 goroutine 触发一次全量重建（HTTP rebuild 端点用，
// design §6：异步、返回 gen）。已在途时不重复触发，返回 (在途代, false)。
// 返回的代是“将要建成”的代（current+1）；真值以 status API 轮询为准。
func (r *Rebuilder) RebuildAsync() (int64, bool) {
	if r == nil {
		return 0, false
	}
	if !r.running.CompareAndSwap(false, true) {
		return r.current.Load() + 1, false
	}
	gen := r.current.Load() + 1
	go func() {
		defer r.running.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
		defer cancel()
		if _, err := r.Rebuild(ctx); err != nil {
			r.lg.Warn("configgraph async rebuild failed", loggateway.Err(err))
		}
	}()
	return gen, true
}

// Rebuild 执行一次全量重建。串行；失败时当前代不动，查询不受影响。
func (r *Rebuilder) Rebuild(ctx context.Context) (RebuildResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 盲存与 RebuildAsync 的 CAS 占坑配合：异步路径已置 true，这里重复置
	// true 无害；defer 统一释放。直接调用方（测试/P2 兜底）由此对 Running()
	// 可见。
	r.running.Store(true)
	defer r.running.Store(false)

	if !r.ready.Load() {
		if err := r.seedGeneration(ctx); err != nil {
			r.lg.Warn("configgraph generation seed failed", loggateway.Err(err))
		}
	}
	gen := r.current.Load() + 1
	start := time.Now()
	r.flowStart(ctx, gen)

	nodes, edges, err := r.extractAll(ctx, gen)
	if err != nil {
		r.flowError(ctx, gen, err)
		return RebuildResult{}, err
	}
	stored := ResolveEdges(edges, NewNodeIndex(nodes), gen, start)

	if err := r.repo.UpsertNodes(ctx, nodes); err != nil {
		r.flowError(ctx, gen, err)
		return RebuildResult{}, err
	}
	if err := r.repo.UpsertEdges(ctx, stored); err != nil {
		r.flowError(ctx, gen, err)
		return RebuildResult{}, err
	}

	res := RebuildResult{Generation: gen, Nodes: len(nodes), Edges: len(stored), Elapsed: time.Since(start)}
	for _, e := range stored {
		if e.Broken() {
			res.Broken++
		}
	}

	// 写全部落库后才切换当前代——查询侧要么读旧代、要么读完整新代。
	r.current.Store(gen)
	r.ready.Store(true)
	r.lastOK.Store(&rebuildSnapshot{res: res, at: time.Now()})
	r.flowDone(ctx, res)

	// 异步清理 gen-1 之前的旧代（保留 1 代供对账，design §2.4）。
	go r.cleanupBelow(gen - 1)
	return res, nil
}

// extractAll 按注册序跑 12 个 Extractor（目标资产先于引用方），节点打上
// 目标代。单个 Extractor 失败即整批失败（设计：Extractor 内部单行走
// broken 降级，接口级错误是 DB 级故障，不应产出半图）。
func (r *Rebuilder) extractAll(ctx context.Context, gen int64) ([]Node, []Edge, error) {
	var nodes []Node
	var edges []Edge
	for _, x := range Extractors(r.provider) {
		ns, err := x.ExtractNodes(ctx, r.src)
		if err != nil {
			return nil, nil, err
		}
		for i := range ns {
			ns[i].Generation = gen
		}
		nodes = append(nodes, ns...)
		es, err := x.ExtractEdges(ctx, r.src)
		if err != nil {
			return nil, nil, err
		}
		edges = append(edges, es...)
	}
	return nodes, edges, nil
}

// cleanupBelow 旧代清理：独立后台 context（重建 ctx 可能已取消），失败仅记
// 日志——残留旧代不影响正确性，下次重建会再清。
func (r *Rebuilder) cleanupBelow(belowGen int64) {
	if belowGen <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	deleted, err := r.repo.DeleteGenerationBelow(ctx, belowGen)
	if err != nil {
		r.lg.Warn("configgraph old-generation cleanup failed", loggateway.Err(err))
		return
	}
	if deleted > 0 {
		r.lg.Info("configgraph old generations cleaned",
			loggateway.Int64("below_gen", belowGen), loggateway.Int64("deleted", deleted))
	}
}

func (r *Rebuilder) flowStart(ctx context.Context, gen int64) {
	if r.flowLog == nil {
		return
	}
	r.flowLog.LogFlowStart(ctx, "", flowStepRebuild, "配置资产图谱全量重建开始",
		monitor.LogPair{Key: "generation", Value: gen})
}

func (r *Rebuilder) flowDone(ctx context.Context, res RebuildResult) {
	if r.flowLog == nil {
		return
	}
	r.flowLog.LogFlowDone(ctx, "", flowStepRebuild, "配置资产图谱全量重建完成",
		monitor.LogPair{Key: "generation", Value: res.Generation},
		monitor.LogPair{Key: "nodes", Value: res.Nodes},
		monitor.LogPair{Key: "edges", Value: res.Edges},
		monitor.LogPair{Key: "broken", Value: res.Broken},
		monitor.LogPair{Key: "elapsed_ms", Value: res.Elapsed.Milliseconds()})
}

func (r *Rebuilder) flowError(ctx context.Context, gen int64, err error) {
	if r.flowLog == nil {
		return
	}
	r.flowLog.LogFlowError(ctx, "", flowStepRebuild, "配置资产图谱全量重建失败",
		monitor.LogPair{Key: "generation", Value: gen},
		monitor.LogPair{Key: "error", Value: err.Error()})
}
