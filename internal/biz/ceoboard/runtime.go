// Package ceoboard 是 CEO 驾驶舱域的装配层(整合 P2-7)。
//
// 本包把 registry(工作流声明)/ agg(健康裁决)/ store(事件存储)/
// adapter(轮询·webhook·NATS)四个子包按 conf.Bootstrap.Ceoboard 装配成
// 运行时,并驱动全部周期循环——语义逐条对应 ceo-dashboard/cmd/server/main.go:
//
//   - spool 回填:每 10s ImportSpool(DB 故障期 fsync 的 JSONL 幂等回灌,风险点④);
//   - 闭日归档:每 1h ArchiveClosedDays(UTC+8 闭日,风险点③);
//   - 注册表热加载:每 30s Reload,全部成功才原子替换,任一失败整体回退 +
//     审计 + 总览顶条提示(风险点⑧),成功后增量接线新增节点;
//   - latency 物化:每 30s 配对 latency(A,B) 落 latency_samples;
//   - 探针接线:平台健康灯/待拍板/节点 rest 面对账轨轮询 + nats·hybrid 节点 WatchNode。
//
// 与原系统的差异仅在外部条件:存储由 aranea data 层供给(PG,共享写柄连接池,
// 迁移随主迁移链);鉴权由 P1 JWT + ceoboard.view/ops 权限码承担(原 sessions
// 废弃);日志走 loggateway(禁 log/slog 红线)。
package ceoboard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/agg"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/conf"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// 默认值(与 ceo-dashboard config.Load 一致;目录在 aranea 下归入 data/ceoboard/ 子树)。
const (
	defaultRegistryDir       = "configs/workflows"
	defaultSpoolDir          = "data/ceoboard/spool"
	defaultArchiveDir        = "data/ceoboard/archive"
	defaultSpoolSegmentBytes = 16 * 1024 * 1024
	defaultSpoolTotalBytes   = 256 * 1024 * 1024
	defaultRetentionDays     = 90
	defaultDBCapacityBytes   = 10 * 1024 * 1024 * 1024
	defaultExportMaxRows     = 100000
	defaultExportTimeout     = 60 * time.Second
	defaultQPSPerHost        = 5.0
	defaultPollInterval      = 30 * time.Second
	defaultStaleAfter        = 90 * time.Second

	spoolImportInterval = 10 * time.Second
	archiveInterval     = time.Hour
	reloadInterval      = 30 * time.Second
	latencyInterval     = 30 * time.Second
)

// Runtime 持有驾驶舱全部运行时组件(service 层经这些字段读视图与写操作)。
type Runtime struct {
	Agg      *agg.Aggregator
	Reg      *registry.Registry
	Rep      *registry.ReportHolder
	Store    *store.Store
	Receiver *adapter.WebhookReceiver
	NATS     *adapter.NATSManager
	Poller   *adapter.Poller

	// ExportMaxRows / ExportTimeout 供 service 层导出审计用(原 api.Server 同名字段)。
	ExportMaxRows int
	ExportTimeout time.Duration

	registryDir  string
	pollInterval time.Duration
	st           *store.Store
	lg           loggateway.Logger

	wireMu sync.Mutex
	wired  map[string]bool
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRuntime 装配驾驶舱运行时(不启动循环;Start 才启动)。
// st 由装配层(wire provider)在 aranea data 层写柄上构建;bc 可为 nil(全默认)。
// 回源凭据密码从环境变量展开(pass_env),缺失即报错——与原 config.Load 的
// 启动时校验语义一致(不裸奔)。
func NewRuntime(bc *conf.Ceoboard, st *store.Store, lg loggateway.Logger) (*Runtime, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if st == nil {
		return nil, fmt.Errorf("ceoboard: store 为 nil")
	}
	c := resolveConfig(bc)

	if err := st.ConfigureSpool(c.spoolDir, c.spoolSegmentBytes, c.spoolTotalBytes); err != nil {
		return nil, fmt.Errorf("ceoboard: 配置 spool: %w", err)
	}
	if err := st.ConfigureArchive(c.archiveDir, c.retentionDays, c.dbCapacityBytes); err != nil {
		return nil, fmt.Errorf("ceoboard: 配置归档: %w", err)
	}

	reg, rep, err := registry.LoadDir(c.registryDir)
	if err != nil {
		return nil, fmt.Errorf("ceoboard: 加载注册表 %s: %w", c.registryDir, err)
	}
	repH := registry.NewReportHolder(rep)
	lg.Info("ceoboard registry loaded",
		loggateway.Str("dir", rep.Dir),
		loggateway.Int("loaded", len(rep.Loaded)),
		loggateway.Int("failed", len(rep.Failed)),
		loggateway.Int("skipped", len(rep.Skipped)))
	for _, f := range rep.Failed {
		lg.Warn("ceoboard registry 失败隔离", loggateway.Str("file", f.File), loggateway.Str("errors", strings.Join(f.Errors, "; ")))
	}

	poller := adapter.NewPoller(adapter.PollerConfig{QPSPerHost: c.qpsPerHost})
	for _, ap := range c.authProviders {
		pass := os.Getenv(ap.GetPassEnv())
		if pass == "" {
			return nil, fmt.Errorf("ceoboard: 回源凭据 %s 的密码环境变量 %s 未设置", ap.GetMatch(), ap.GetPassEnv())
		}
		tp, err := adapter.NewTokenProvider(ap.GetType(), ap.GetLoginUrl(), ap.GetUser(), pass)
		if err != nil {
			return nil, fmt.Errorf("ceoboard: 回源凭据 %s: %w", ap.GetMatch(), err)
		}
		poller.SetAuthProvider(ap.GetMatch(), tp)
		lg.Info("ceoboard auth provider 挂接",
			loggateway.Str("match", ap.GetMatch()),
			loggateway.Str("login_url", ap.GetLoginUrl()),
			loggateway.Str("user", ap.GetUser()))
	}

	receiver := adapter.NewWebhookReceiver(st, c.webhookKey)
	natsMgr := adapter.NewNATSManager(st, c.natsPlatforms, lg)

	agCfg := &agg.Config{NATSPlatforms: c.natsPlatforms}
	for _, p := range c.platforms {
		agCfg.Platforms = append(agCfg.Platforms, agg.Platform{ID: p.GetId(), Name: p.GetName(), Probe: p.GetProbe()})
	}
	for _, ap := range c.approvals {
		agCfg.Approvals = append(agCfg.Approvals, agg.Approval{Lane: ap.GetLane(), Label: ap.GetLabel(), Path: ap.GetPath()})
	}

	ag := &agg.Aggregator{
		Reg: reg, Rep: repH, Poller: poller, Receiver: receiver,
		NATS: natsMgr, Store: st, Tracker: agg.NewDurationTracker(),
		Cfg: agCfg, StaleDef: c.staleAfter,
	}

	rt := &Runtime{
		Agg: ag, Reg: reg, Rep: repH, Store: st,
		Receiver: receiver, NATS: natsMgr, Poller: poller,
		ExportMaxRows: c.exportMaxRows, ExportTimeout: c.exportTimeout,
		registryDir: c.registryDir, pollInterval: c.pollInterval,
		st: st, lg: lg,
		wired: map[string]bool{},
	}
	return rt, nil
}

// Start 启动全部周期循环与探针接线;Stop(或 ctx 取消)停止。
// 语义逐条对应原 main.go 的 goroutine 清单。
func (rt *Runtime) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	rt.ctx = ctx
	rt.cancel = cancel

	// DB 故障期间先 fsync 到独立 spool;恢复只按 event_uid 幂等导入(风险点④)。
	safego.Go(ctx, "ceoboard.spool_import", func() {
		tk := time.NewTicker(spoolImportInterval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				if n, err := rt.st.ImportSpool(ctx); err != nil {
					rt.lg.Warn("ceoboard spool 恢复失败(pending 保留)", loggateway.Err(err))
				} else if n > 0 {
					rt.lg.Info("ceoboard spool 幂等恢复", loggateway.Int("events", n))
				}
			}
		}
	})

	// 每小时归档超期闭日;先校验压缩文件和 manifest,再删除本次已归档 UID(风险点③)。
	safego.Go(ctx, "ceoboard.archive", func() {
		tk := time.NewTicker(archiveInterval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				if n, err := rt.st.ArchiveClosedDays(ctx, time.Now()); err != nil {
					rt.lg.Warn("ceoboard 归档失败(热数据保留)", loggateway.Err(err))
				} else if n > 0 {
					rt.lg.Info("ceoboard 闭日归档完成", loggateway.Int("days", n))
				}
			}
		}
	})

	// 平台健康灯 + 待拍板轮询。
	for _, p := range rt.Agg.Cfg.Platforms {
		method, url := agg.SplitREST(p.Probe)
		if url == "" {
			rt.lg.Warn("ceoboard 非法平台探针声明", loggateway.Str("probe", p.Probe))
			continue
		}
		go rt.Poller.RunLoop(ctx, "platform/"+p.ID, method, url, rt.pollInterval)
	}
	for _, ap := range rt.Agg.Cfg.Approvals {
		method, url := agg.SplitREST(ap.Path)
		if url == "" {
			rt.lg.Warn("ceoboard 非法待拍板路径声明", loggateway.Str("path", ap.Path))
			continue
		}
		go rt.Poller.RunLoop(ctx, "approval/"+ap.Lane, method, url, rt.pollInterval)
	}
	rt.wire()

	// 注册表热加载(每 30s):全部声明成功才原子替换;任一失败整体回退 + 审计 + 总览提示(风险点⑧)。
	safego.Go(ctx, "ceoboard.registry_reload", func() {
		tk := time.NewTicker(reloadInterval)
		defer tk.Stop()
		lastSig := strings.Join(rt.Rep.Get().Loaded, ",")
		lastErr := ""
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				newRep, err := rt.Reg.Reload(rt.registryDir)
				if err != nil {
					if err.Error() != lastErr {
						lastErr = err.Error()
						rt.lg.Warn("ceoboard 注册表热加载失败(整体回退,旧版继续服务)", loggateway.Err(err))
						_ = rt.st.AddAudit(ctx, "system", "registry-reload", rt.registryDir, false, err.Error())
						rt.Agg.SetRegNotice("注册表热加载失败,已回退到上一可用版本: " + err.Error())
					}
					continue
				}
				rt.Rep.Set(newRep)
				rt.Agg.SetRegNotice("")
				lastErr = ""
				if sig := strings.Join(newRep.Loaded, ","); sig != lastSig {
					lastSig = sig
					rt.lg.Info("ceoboard 注册表热加载成功", loggateway.Int("workflows", len(newRep.Loaded)))
					_ = rt.st.AddAudit(ctx, "system", "registry-reload", rt.registryDir, true, "loaded="+sig)
				}
				rt.wire()
			}
		}
	})

	// latency 物化:周期配对 latency(A,B) 声明的事件,落 latency_samples。
	lat := agg.NewLatencyMaterializer(rt.Reg, rt.st, rt.Agg.Cfg.NATSPlatforms, rt.lg)
	safego.Go(ctx, "ceoboard.latency", func() { lat.RunLoop(ctx, latencyInterval) })
}

// Stop 停止全部循环(幂等)。
func (rt *Runtime) Stop() {
	if rt.cancel != nil {
		rt.cancel()
	}
}

// wire 装配轮询/事件接线:已接线 key 记录,热加载后新增节点增量接线
// (存量节点配置变更需重启生效——与原语义一致)。
func (rt *Runtime) wire() {
	rt.wireMu.Lock()
	defer rt.wireMu.Unlock()
	for _, wf := range rt.Reg.List() {
		for i := range wf.Nodes {
			n := &wf.Nodes[i]
			if n.Source == nil || n.Source.Skipped {
				continue
			}
			// 事件侧接线:nats-jetstream 与 hybrid 的 nats 半侧。
			if n.Source.Adapter == adapter.KindNATS || n.Source.Adapter == adapter.KindHybrid {
				if n.Source.Endpoint != "" {
					key := "watch/" + wf.ID + "/" + n.ID
					if !rt.wired[key] {
						rt.wired[key] = true
						go rt.NATS.WatchNode(rt.ctx, wf.ID, n)
					}
				}
			}
			// 对账侧接线:凡声明了 rest_path 的节点都起轮询(rest-poll 主路 / hybrid·nats·webhook 的对账轨)。
			if n.Source.RestPath == "" {
				continue
			}
			key := "poll/" + wf.ID + "/" + n.ID
			if rt.wired[key] {
				continue
			}
			rt.wired[key] = true
			interval := rt.pollInterval
			if wf.Probe != nil && wf.Probe.Interval.D() > 0 {
				interval = wf.Probe.Interval.D()
			}
			method, url := agg.SplitREST(n.Source.RestPath)
			go rt.Poller.RunLoop(rt.ctx, agg.PollKey(wf.ID, n.ID), method, url, interval)
		}
	}
}

// ---------- 内部配置归一 ----------

// resolvedConfig 是 conf.Ceoboard 归一默认值后的内部表示。
type resolvedConfig struct {
	registryDir       string
	spoolDir          string
	spoolSegmentBytes int64
	spoolTotalBytes   int64
	archiveDir        string
	retentionDays     int
	dbCapacityBytes   int64
	exportMaxRows     int
	exportTimeout     time.Duration
	webhookKey        string
	qpsPerHost        float64
	pollInterval      time.Duration
	staleAfter        time.Duration
	natsPlatforms     map[string]string
	authProviders     []*conf.Ceoboard_AuthProvider
	platforms         []*conf.Ceoboard_Platform
	approvals         []*conf.Ceoboard_Approval
}

func resolveConfig(bc *conf.Ceoboard) *resolvedConfig {
	c := &resolvedConfig{
		registryDir:       defaultRegistryDir,
		spoolDir:          defaultSpoolDir,
		spoolSegmentBytes: defaultSpoolSegmentBytes,
		spoolTotalBytes:   defaultSpoolTotalBytes,
		archiveDir:        defaultArchiveDir,
		retentionDays:     defaultRetentionDays,
		dbCapacityBytes:   defaultDBCapacityBytes,
		exportMaxRows:     defaultExportMaxRows,
		exportTimeout:     defaultExportTimeout,
		qpsPerHost:        defaultQPSPerHost,
		pollInterval:      defaultPollInterval,
		staleAfter:        defaultStaleAfter,
	}
	if bc == nil {
		return c
	}
	if v := bc.GetRegistryDir(); v != "" {
		c.registryDir = v
	}
	if v := bc.GetSpoolDir(); v != "" {
		c.spoolDir = v
	}
	if v := bc.GetSpoolSegmentBytes(); v > 0 {
		c.spoolSegmentBytes = v
	}
	if v := bc.GetSpoolTotalBytes(); v > 0 {
		c.spoolTotalBytes = v
	}
	if v := bc.GetArchiveDir(); v != "" {
		c.archiveDir = v
	}
	if v := bc.GetEventRetentionDays(); v > 0 {
		c.retentionDays = int(v)
	}
	if v := bc.GetDbCapacityBytes(); v > 0 {
		c.dbCapacityBytes = v
	}
	if v := bc.GetExportMaxRows(); v > 0 {
		c.exportMaxRows = int(v)
	}
	if d := bc.GetExportTimeout(); d != nil && d.AsDuration() > 0 {
		c.exportTimeout = d.AsDuration()
	}
	c.webhookKey = bc.GetWebhookKey()
	if v := bc.GetQpsPerPlatform(); v > 0 {
		c.qpsPerHost = v
	}
	if d := bc.GetPollInterval(); d != nil && d.AsDuration() > 0 {
		c.pollInterval = d.AsDuration()
	}
	if d := bc.GetStaleAfter(); d != nil && d.AsDuration() > 0 {
		c.staleAfter = d.AsDuration()
	}
	if len(bc.GetNatsPlatforms()) > 0 {
		c.natsPlatforms = bc.GetNatsPlatforms()
	}
	c.authProviders = bc.GetAuthProviders()
	c.platforms = bc.GetPlatforms()
	c.approvals = bc.GetApprovals()
	return c
}
