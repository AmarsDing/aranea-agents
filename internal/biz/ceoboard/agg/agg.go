// Package agg 把注册表声明 + 适配器运行态合成为总览视图模型。
// 排序规则:组内 健康度(红>黄>绿>灰) → 告警数 → 业务量(近 1h 事件数) → id;
// 仅当组内全部绿/灰时可折叠,红/黄组永不折叠。
//
// 移植说明(整合 P2-4):原 ceoboard 直接依赖 *adapter.Poller / *store.Store 等具体类型;
// 本包改为消费者侧窄接口(PollQuerier/WebhookCounter/NATSQuerier/EventReadStore),
// 由 adapter(P2-6)与 store(P2-5)的实现满足;单测用内存 fake,不依赖 PG/NATS。
package agg

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
)

// 健康度档位(数值越小越严重)。
const (
	HealthRed    = "red"
	HealthYellow = "yellow"
	HealthGreen  = "green"
	HealthGray   = "gray" // 未接入/待补/未知
)

func healthRank(h string) int {
	switch h {
	case HealthRed:
		return 0
	case HealthYellow:
		return 1
	case HealthGreen:
		return 2
	default:
		return 3
	}
}

// ---------- 运行配置(agg 需要的最小面;P2-7 由 conf.proto 装配填充) ----------

// Platform 是一个被观测平台(健康灯)。
type Platform struct {
	ID    string
	Name  string
	Probe string // "GET <url>"
}

// Approval 是一个待拍板数据源(只读清单接口)。
type Approval struct {
	Lane  string
	Label string
	Path  string // "GET <url>"
}

// Config 是聚合器需要的运行配置。
// NATSPlatforms: endpoint host:port → 平台 id(事件打标与计数口径,原 config.NATSPlatforms)。
type Config struct {
	Platforms     []Platform
	Approvals     []Approval
	NATSPlatforms map[string]string
}

// ---------- 消费者侧依赖接口(P2-5/P2-6 实现) ----------

// PollQuerier 是 agg 对 REST 轮询器运行态的只读依赖面。
type PollQuerier interface {
	Lookup(key string) *adapter.PollState
}

// WebhookCounter 是 agg 对 webhook 接收端的计数依赖面。
type WebhookCounter interface {
	CountSince(source, event string, since time.Time) int
}

// NATSQuerier 是 agg 对 NATS 管理器运行态的只读依赖面。
type NATSQuerier interface {
	Lookup(nodeKey string) *adapter.NATSNodeState
}

// EventReadStore 是 agg 对事件存储的只读依赖面(P2-5 由 Ent 仓储实现)。
type EventReadStore interface {
	// RecentDeliveries 取某源最近投递(按时间倒序,limit 封顶)。
	RecentDeliveries(source string, limit int) ([]store.Delivery, error)
	// CountEvents 统计窗口内某平台某 subject_key 的事件数。
	CountEvents(platform, subjectKey string, since time.Time) (int, error)
	// LatencyWindow 取窗口内某指标的延迟样本毫秒列(供 p50/p95)。
	LatencyWindow(metricID string, since time.Time) ([]int64, error)
}

// ---------- 视图模型(service 层映射到 proto;json tag 保留原系统线形供双跑对比) ----------

type MetricView struct {
	ID      string   `json:"id"`
	Unit    string   `json:"unit"`
	Window  string   `json:"window"`
	Value   *float64 `json:"value"` // nil = 无数据(不显示 0 冒充)
	Display string   `json:"display"`
	Health  string   `json:"health"`
	Note    string   `json:"note,omitempty"`
	Pending bool     `json:"pending,omitempty"`
	Origin  string   `json:"origin,omitempty"` // event | reconcile:指标数据来源(双轨标注)
}

type SourceView struct {
	Adapter        string     `json:"adapter"`
	RestPath       string     `json:"rest_path,omitempty"`
	Skipped        bool       `json:"skipped,omitempty"`
	SkipReason     string     `json:"skip_reason,omitempty"`
	Unwired        bool       `json:"unwired,omitempty"` // 未接入的适配器(sse/ws/db-query)
	Origin         string     `json:"origin,omitempty"`  // event | reconcile | event+reconcile(双轨标注)
	Stream         string     `json:"stream,omitempty"`
	Consuming      bool       `json:"consuming,omitempty"`      // durable consumer 已建立
	Connected      bool       `json:"connected,omitempty"`      // NATS 连接存活
	MsgCount       int64      `json:"msg_count,omitempty"`      // 累计消费(去重后)
	DupCount       int64      `json:"dup_count,omitempty"`      // 重复投递去重次数
	StreamMissing  bool       `json:"stream_missing,omitempty"` // 声明流在 JetStream 不存在
	StreamOptional bool       `json:"stream_optional,omitempty"`
	Backlog        bool       `json:"backlog,omitempty"`       // 消费积压(持续满批拉取)
	LastEventAt    *time.Time `json:"last_event_at,omitempty"` // 事件侧最后收到时间
	AsOf           *time.Time `json:"as_of,omitempty"`
	LastOKAt       *time.Time `json:"last_ok_at,omitempty"` // 对账侧最后成功时间
	LastErr        string     `json:"last_err,omitempty"`
	FailCount      int        `json:"fail_count,omitempty"`
	LatencyMs      int64      `json:"latency_ms,omitempty"`
}

type NodeView struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Order   int          `json:"order"`
	Health  string       `json:"health"`
	Reason  string       `json:"reason"`
	Metrics []MetricView `json:"metrics"`
	Source  *SourceView  `json:"source,omitempty"`
	Drill   []DrillView  `json:"drill,omitempty"`
}

type DrillView struct {
	Label      string `json:"label"`
	Type       string `json:"type"`
	Path       string `json:"path"`
	EmptyState string `json:"empty_state,omitempty"`
	Note       string `json:"note,omitempty"`
	Permission string `json:"-"`
}

type WorkflowView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	BizLine     string     `json:"biz_line"`
	Owner       string     `json:"owner"`
	Version     int        `json:"version"`
	Permissions []string   `json:"permissions"`
	Health      string     `json:"health"`
	Alerts      int        `json:"alerts"`
	Volume1h    int        `json:"volume_1h"`
	Nodes       []NodeView `json:"nodes"`
}

type LaneView struct {
	BizLine     string         `json:"biz_line"`
	Collapsible bool           `json:"collapsible"` // 仅全部绿/灰组可折叠
	Workflows   []WorkflowView `json:"workflows"`
}

type PlatformView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Health    string     `json:"health"`
	Probe     string     `json:"probe"`
	LastOKAt  *time.Time `json:"last_ok_at,omitempty"`
	LastErr   string     `json:"last_err,omitempty"`
	LatencyMs int64      `json:"latency_ms,omitempty"`
}

type AlertView struct {
	ID     string    `json:"id"`
	Level  string    `json:"level"`
	Target string    `json:"target"` // 工作流/节点/平台
	Text   string    `json:"text"`
	Since  time.Time `json:"since"`
}

type ApprovalItem struct {
	Lane  string `json:"lane"`
	ID    string `json:"id"`
	Title string `json:"title"`
	Extra string `json:"extra,omitempty"`
}

type ApprovalsView struct {
	Health   string         `json:"health"`
	LastOKAt *time.Time     `json:"last_ok_at,omitempty"`
	Count    int            `json:"count"`
	Items    []ApprovalItem `json:"items"`
	Note     string         `json:"note,omitempty"`
}

type RegistryView struct {
	Dir      string               `json:"dir"`
	LoadedAt time.Time            `json:"loaded_at"`
	Loaded   []string             `json:"loaded"`
	Failed   []registry.FailEntry `json:"failed"`
	Skipped  []registry.SkipEntry `json:"skipped"`
}

type Overview struct {
	AsOf      time.Time      `json:"as_of"`
	Platforms []PlatformView `json:"platforms"`
	Alerts    []AlertView    `json:"alerts"`
	Approvals ApprovalsView  `json:"approvals"`
	Lanes     []LaneView     `json:"lanes"`
	Registry  RegistryView   `json:"registry"`
	Notice    string         `json:"notice,omitempty"` // 兜底提示(如权限裁剪后全不可见)
	// RegistryNotice 注册表热加载提示(失败整体回退时非空,总览顶条展示)。
	RegistryNotice string `json:"registry_notice,omitempty"`
}

// ---------- 聚合器 ----------

// Aggregator 聚合器。Tracker 跨 Build 存活(持续阈值状态);Store/NATS 为事件侧依赖(可空)。
type Aggregator struct {
	Reg      *registry.Registry
	Rep      *registry.ReportHolder
	Poller   PollQuerier
	Receiver WebhookCounter
	NATS     NATSQuerier // 可为 nil(无 nats 节点时)
	Store    EventReadStore
	Tracker  *DurationTracker
	Cfg      *Config
	StaleDef time.Duration

	regNotice atomic.Value // string:注册表热加载提示(空串=正常)
}

// SetRegNotice 设置/清除注册表热加载提示(热加载回退时由装配层调用)。
func (a *Aggregator) SetRegNotice(s string) { a.regNotice.Store(s) }

// 告警去重窗口(持续告警 30m 聚合防刷屏)。
const alertDedupWindow = 30 * time.Minute

// biz_line 组顺序:内容/电商/投资/底座,新业务线追加末尾。
var laneOrder = []string{"content", "ecommerce", "invest", "platform"}

// Build 生成一次总览快照。
func (a *Aggregator) Build(now time.Time) *Overview {
	ov := &Overview{AsOf: now, Platforms: []PlatformView{}, Lanes: []LaneView{}}
	alerts := []AlertView{}

	// 平台健康灯
	for _, p := range a.Cfg.Platforms {
		pv := a.evalPlatform(p, now)
		ov.Platforms = append(ov.Platforms, pv)
		if pv.Health == HealthRed {
			alerts = append(alerts, AlertView{
				ID: "platform/" + pv.ID, Level: "red", Target: pv.Name,
				Text:  fmt.Sprintf("平台探针失败: %s", emptyAs(pv.LastErr, "从未成功")),
				Since: now,
			})
		}
	}

	// 工作流
	byLane := map[string][]WorkflowView{}
	volume := map[string]int{}
	for _, wf := range a.Reg.List() {
		wv := WorkflowView{ID: wf.ID, Name: wf.Name, BizLine: wf.BizLine, Owner: wf.Owner, Version: wf.Version, Permissions: wf.Permissions, Health: HealthGreen, Nodes: []NodeView{}}
		worst := HealthGray
		if len(wf.Nodes) > 0 {
			worst = HealthGreen
		}
		for i := range wf.Nodes {
			nv, nAlerts := a.evalNode(wf, &wf.Nodes[i], now)
			alerts = append(alerts, nAlerts...)
			nvAlerts := len(nAlerts)
			wv.Alerts += nvAlerts
			volume[wf.ID] += a.nodeVolume1h(wf, &wf.Nodes[i], now)
			if healthRank(nv.Health) < healthRank(worst) {
				worst = nv.Health
			}
			wv.Nodes = append(wv.Nodes, nv)
		}
		wv.Health = worst
		wv.Alerts = countAlertsFor(alerts, wf.ID)
		wv.Volume1h = volume[wf.ID]
		byLane[wf.BizLine] = append(byLane[wf.BizLine], wv)
	}

	// 组排序与折叠规则
	var laneKeys []string
	for _, k := range laneOrder {
		if _, ok := byLane[k]; ok {
			laneKeys = append(laneKeys, k)
		}
	}
	for k := range byLane { // 新业务线追加末尾
		if !contains(laneKeys, k) {
			laneKeys = append(laneKeys, k)
		}
	}
	for _, k := range laneKeys {
		ws := byLane[k]
		sort.SliceStable(ws, func(i, j int) bool {
			if healthRank(ws[i].Health) != healthRank(ws[j].Health) {
				return healthRank(ws[i].Health) < healthRank(ws[j].Health)
			}
			if ws[i].Alerts != ws[j].Alerts {
				return ws[i].Alerts > ws[j].Alerts
			}
			if ws[i].Volume1h != ws[j].Volume1h {
				return ws[i].Volume1h > ws[j].Volume1h
			}
			return ws[i].ID < ws[j].ID
		})
		collapsible := true
		for _, w := range ws {
			if w.Health == HealthRed || w.Health == HealthYellow {
				collapsible = false
				break
			}
		}
		ov.Lanes = append(ov.Lanes, LaneView{BizLine: k, Collapsible: collapsible, Workflows: ws})
	}

	// 告警排序:红在前
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].Level != alerts[j].Level {
			return alerts[i].Level == "red"
		}
		return alerts[i].ID < alerts[j].ID
	})
	ov.Alerts = alerts

	ov.Approvals = a.evalApprovals(now)
	rep := a.Rep.Get()
	ov.Registry = RegistryView{Dir: rep.Dir, LoadedAt: rep.LoadedAt, Loaded: rep.Loaded, Failed: rep.Failed, Skipped: rep.Skipped}
	if s, ok := a.regNotice.Load().(string); ok && s != "" {
		ov.RegistryNotice = s
	}
	return ov
}

func countAlertsFor(alerts []AlertView, wfID string) int {
	n := 0
	for _, al := range alerts {
		if strings.HasPrefix(al.ID, "wf/"+wfID+"/") {
			n++
		}
	}
	return n
}

func (a *Aggregator) evalPlatform(p Platform, now time.Time) PlatformView {
	pv := PlatformView{ID: p.ID, Name: p.Name, Probe: p.Probe, Health: HealthGray}
	st := a.Poller.Lookup("platform/" + p.ID)
	if st == nil {
		pv.Health = HealthGray
		return pv
	}
	pv.LatencyMs = st.LatencyMs
	pv.LastErr = st.LastErr
	if !st.LastOKAt.IsZero() {
		t := st.LastOKAt
		pv.LastOKAt = &t
	}
	switch {
	case st.LastOKAt.IsZero():
		pv.Health = HealthRed // 从未成功
	case now.Sub(st.LastOKAt) > a.StaleDef:
		pv.Health = HealthYellow // 陈旧
	default:
		pv.Health = HealthGreen
	}
	return pv
}

// evalNode 评估单节点健康与指标。
func (a *Aggregator) evalNode(wf *registry.Workflow, n *registry.Node, now time.Time) (NodeView, []AlertView) {
	nv := NodeView{ID: n.ID, Name: n.Name, Order: n.Order, Health: HealthGray, Metrics: []MetricView{}, Drill: []DrillView{}}
	var alerts []AlertView

	src := n.Source
	probeAlive := false // "且探针活" 判定依据
	switch {
	case src == nil:
		nv.Health, nv.Reason = HealthGray, "无数据源声明"
	case src.Skipped:
		nv.Health, nv.Reason = HealthGray, "数据未接通(声明待补)"
		nv.Source = &SourceView{Adapter: src.Adapter, Skipped: true, SkipReason: src.SkipReason}
	case src.Adapter == adapter.KindSSE || src.Adapter == adapter.KindWS || src.Adapter == adapter.KindDBQuery:
		nv.Health, nv.Reason = HealthGray, "适配器未接入(P2)"
		nv.Source = &SourceView{Adapter: src.Adapter, Unwired: true}
	default:
		nv.Health, nv.Reason, probeAlive, alerts = a.evalSource(wf, n, &nv, now)
	}

	// 指标求值(事件类走 store,轮询类走 Poller;算不出的一律 note 说明,不假装)
	for i := range n.Metrics {
		mv, mAlerts := a.evalMetric(wf, n, &n.Metrics[i], nv.Source, probeAlive, now)
		alerts = append(alerts, mAlerts...)
		nv.Metrics = append(nv.Metrics, mv)
	}

	for _, d := range n.Drill {
		nv.Drill = append(nv.Drill, DrillView{Label: d.Label, Type: d.Type, Path: d.Path, EmptyState: d.EmptyState, Note: d.Note, Permission: d.Permission})
	}
	return nv, alerts
}

// evalSource 评估节点数据源健康(双轨:事件侧 event / 对账侧 reconcile;冲突时健康度以对账为准)。
// 返回 (健康度, 原因, 探针是否活, 告警);并填充 nv.Source。
func (a *Aggregator) evalSource(wf *registry.Workflow, n *registry.Node, nv *NodeView, now time.Time) (string, string, bool, []AlertView) {
	src := n.Source
	stale := a.StaleDef
	if wf.Probe != nil && wf.Probe.StaleAfter.D() > 0 {
		stale = wf.Probe.StaleAfter.D()
	}
	var alerts []AlertView
	sv := &SourceView{Adapter: src.Adapter}
	nv.Source = sv

	// ---- 事件侧 ----
	evOK := false
	evNote := ""
	switch src.Adapter {
	case adapter.KindNATS, adapter.KindHybrid:
		var ns *adapter.NATSNodeState
		if a.NATS != nil {
			ns = a.NATS.Lookup(wf.ID + "/" + n.ID)
		}
		switch {
		case ns == nil:
			evNote = "事件侧未建立消费(nats 未接线)"
		default:
			sv.Stream, sv.Connected, sv.Consuming = ns.Stream, ns.Connected, ns.Consuming
			sv.MsgCount, sv.DupCount = ns.MsgCount, ns.DupCount
			sv.StreamMissing, sv.StreamOptional, sv.Backlog = ns.StreamMissing, ns.StreamOptional, ns.Backlog
			if !ns.LastMsgAt.IsZero() {
				t := ns.LastMsgAt
				sv.LastEventAt = &t
			}
			switch {
			case ns.StreamMissing && ns.StreamOptional:
				// 条件创建流(如 MM_PUB 仅 MAP worker 启用时建):缺失=未启用,非漂移;
				// 事件侧不判红,健康度交给对账侧(evOK=false 由双轨裁决处理)
				evNote = "声明流 " + ns.Stream + " 未创建(声明显式可选,平台方未启用;见 smoke.sh 预期降级)"
				sv.LastErr = ns.LastErr
			case ns.StreamMissing:
				evNote = "声明漂移: stream " + ns.Stream + " 在 JetStream 不存在(声明与平台实际不符)"
				sv.LastErr = ns.LastErr
			case !ns.Consuming:
				evNote = "durable consumer 未建立: " + emptyAs(ns.LastErr, "stream/subject 不可达")
				sv.LastErr = ns.LastErr
			case !ns.Connected:
				evNote = "NATS 连接断开(自动重连中)"
			default:
				evOK = true
				evNote = fmt.Sprintf("durable 消费中(累计 %d 条,去重 %d)", ns.MsgCount, ns.DupCount)
				if ns.Backlog {
					evNote += ";消费积压中(持续满批拉取)"
				}
			}
		}
	case adapter.KindWebhook:
		cnt := a.Receiver.CountSince(webhookSourceOf(n), "", now.Add(-24*time.Hour))
		sv.MsgCount = int64(cnt)
		evOK = true // 接收端常驻;不代表源可达(无对账时按流量分级)
		if last := a.lastDeliveryAt(webhookSourceOf(n)); !last.IsZero() {
			t := last
			sv.LastEventAt = &t
		}
		evNote = fmt.Sprintf("24h 内收到 %d 条推送", cnt)
	}

	// ---- 对账侧(rest 轮询;aranea webhook 重试耗尽后的兜底轨) ----
	recHealth := ""
	if src.RestPath != "" {
		method, url := splitREST(src.RestPath)
		sv.RestPath = method + " " + url
		st := a.Poller.Lookup("node/" + wf.ID + "/" + n.ID)
		switch {
		case st == nil:
			recHealth = HealthGray
		default:
			if !st.AsOf.IsZero() {
				t := st.AsOf
				sv.AsOf = &t
			}
			if !st.LastOKAt.IsZero() {
				t := st.LastOKAt
				sv.LastOKAt = &t
			}
			sv.FailCount, sv.LatencyMs = st.FailCount, st.LatencyMs
			if st.LastErr != "" {
				sv.LastErr = st.LastErr
			}
			switch {
			case st.LastOKAt.IsZero():
				recHealth = HealthRed
				alerts = append(alerts, AlertView{
					ID: "wf/" + wf.ID + "/" + n.ID, Level: "red", Target: wf.Name + "/" + n.Name,
					Text: "对账数据源不可达: " + st.LastErr, Since: now,
				})
			case now.Sub(st.LastOKAt) > stale:
				recHealth = HealthYellow
			default:
				recHealth = HealthGreen
			}
		}
	}

	// ---- 双轨裁决(返回的第三值为"探针活",供持续阈值"且探针活"判定) ----
	hasEventSide := src.Adapter == adapter.KindNATS || src.Adapter == adapter.KindHybrid || src.Adapter == adapter.KindWebhook
	switch {
	case !hasEventSide:
		// 纯 rest-poll
		sv.Origin = "reconcile"
		return recHealthOrGray(recHealth), pollReason(sv, recHealth), recHealth == HealthGreen, alerts
	case recHealth == "":
		// 纯事件侧
		sv.Origin = "event"
		if evOK {
			if src.Adapter == adapter.KindWebhook && sv.MsgCount == 0 {
				return HealthGray, "webhook 接收端在线,尚未收到推送(无对账路径)", true, alerts
			}
			return HealthGreen, evNote, true, alerts
		}
		alerts = append(alerts, AlertView{
			ID: "wf/" + wf.ID + "/" + n.ID + "/event", Level: "red", Target: wf.Name + "/" + n.Name,
			Text: "事件侧不可用: " + evNote, Since: now,
		})
		return HealthRed, evNote, false, alerts
	case recHealth == HealthRed:
		// 对账失败 → 以对账为准(事件再流畅也标红)
		sv.Origin = "reconcile"
		return HealthRed, "对账失败(以此为准): " + emptyAs(sv.LastErr, "从未成功") + ";" + evNote, evOK, alerts
	case recHealth == HealthGreen:
		if evOK {
			sv.Origin = "event+reconcile"
			return HealthGreen, "对账正常;" + evNote, true, alerts
		}
		sv.Origin = "reconcile"
		return HealthYellow, "事件侧中断,以对账为准;" + evNote, true, alerts
	case recHealth == HealthYellow:
		if evOK {
			sv.Origin = "event"
			return HealthYellow, "对账陈旧,以事件为准;" + evNote, true, alerts
		}
		sv.Origin = "reconcile"
		return HealthYellow, "对账陈旧且事件侧中断;" + evNote, false, alerts
	default: // gray(等待首次轮询)
		if evOK {
			sv.Origin = "event"
			return HealthGreen, "对账未就绪,以事件为准;" + evNote, true, alerts
		}
		sv.Origin = "reconcile"
		return HealthGray, "等待首次轮询", false, alerts
	}
}

func recHealthOrGray(h string) string {
	if h == "" {
		return HealthGray
	}
	return h
}

func pollReason(sv *SourceView, recHealth string) string {
	switch recHealth {
	case HealthRed:
		return "探针失败: " + sv.LastErr
	case HealthYellow:
		return "数据陈旧"
	case HealthGreen:
		return "正常"
	}
	return "等待首次轮询"
}

// lastDeliveryAt 取某源最近一次成功投递时间。
func (a *Aggregator) lastDeliveryAt(source string) time.Time {
	if a.Store == nil {
		return time.Time{}
	}
	ds, err := a.Store.RecentDeliveries(source, 1)
	if err != nil || len(ds) == 0 {
		return time.Time{}
	}
	return ds[0].At
}

// platformOfNode 由 source.endpoint 映射平台 id(事件打标与计数口径)。
func (a *Aggregator) platformOfNode(n *registry.Node) string {
	if n.Source == nil {
		return ""
	}
	return a.PlatformOfNodePublic(n.Source.Endpoint)
}

// PlatformOfNodePublic 由 endpoint 映射平台 id(api 层下钻用)。
func (a *Aggregator) PlatformOfNodePublic(endpoint string) string {
	if endpoint == "" || a.Cfg == nil {
		return ""
	}
	return a.Cfg.NATSPlatforms[adapter.HostPort(endpoint)]
}

// evalMetric 评估单指标;返回视图与该指标应产生的告警(持续阈值只在跃迁/续报时产生)。
func (a *Aggregator) evalMetric(wf *registry.Workflow, n *registry.Node, m *registry.Metric, src *SourceView, probeAlive bool, now time.Time) (MetricView, []AlertView) {
	mv := MetricView{ID: m.ID, Unit: m.Unit, Window: m.Window, Health: HealthGray}
	if m.Pending != "" {
		mv.Pending = true
		mv.Display = "待补"
		mv.Note = m.Pending
		return mv, nil
	}
	expr := strings.TrimSpace(m.Expr)
	items := pollItems(a.Poller, wf, n)
	win := windowDur(m.Window)

	switch {
	case strings.HasPrefix(expr, "latency("):
		// 物化:latency_samples 由 LatencyMaterializer 周期配对产出
		if a.Store == nil {
			mv.Display = "—"
			mv.Note = "store 未接线"
			return mv, nil
		}
		samples, err := a.Store.LatencyWindow(m.ID, now.Add(-win))
		if err != nil || len(samples) == 0 {
			mv.Display = "—"
			mv.Note = "窗口内无配对样本(事件侧无流量或关联键缺失)"
			return mv, nil
		}
		v, _ := Percentile(samples, 50)
		mv.Value = &v
		mv.Origin = "event"
		mv.Note = fmt.Sprintf("p50/%d 样本(物化)", len(samples))
		if strings.Contains(m.Precompute, "p95") {
			if p95, ok := Percentile(samples, 95); ok {
				mv.Note = fmt.Sprintf("p50;p95=%s/%d 样本(物化)", strconv.FormatFloat(p95, 'f', -1, 64), len(samples))
			}
		}
	case m.Precompute != "":
		mv.Display = "—"
		mv.Note = "物化 job 未注册: " + m.Precompute + "(需专用物化器,不编数据)"
		return mv, nil
	case expr == "gauge(rtt)":
		if src != nil && src.LastOKAt != nil {
			v := float64(src.LatencyMs)
			mv.Value = &v
			mv.Origin = "reconcile"
		}
	case expr == "count(enter)" || expr == "count(exit)":
		v, note, ok := a.countEnterExit(wf, n, expr == "count(enter)", now, win)
		if !ok {
			mv.Display = "—"
			mv.Note = note
			return mv, nil
		}
		mv.Value = &v
		mv.Origin = "event"
		mv.Note = note
	case strings.HasPrefix(expr, "count(subject="):
		subj := parseSubjectLit(expr)
		if subj == "" {
			mv.Display = "—"
			mv.Note = "subject 字面量解析失败"
			return mv, nil
		}
		v, ok := a.countSubject(wf, n, subj, now, win)
		if !ok {
			mv.Display = "—"
			mv.Note = "事件侧未接线或无事件流"
			return mv, nil
		}
		mv.Value = &v
		mv.Origin = "event"
	case strings.HasPrefix(expr, "ratio(count("):
		sub := splitTopLevelRatio(expr)
		if len(sub) != 2 {
			mv.Display = "—"
			mv.Note = "ratio 表达式不支持(仅支持 ratio(count,count))"
			return mv, nil
		}
		num, nOK, nOrigin := a.countOperand(wf, n, sub[0], items, now, win)
		den, dOK, dOrigin := a.countOperand(wf, n, sub[1], items, now, win)
		if !nOK || !dOK {
			mv.Display = "—"
			mv.Note = "无数据(事件侧未接线或无轮询数据)"
			return mv, nil
		}
		if den == 0 {
			mv.Display = "—"
			mv.Note = "分母为 0"
			return mv, nil
		}
		v := num / den * 100
		mv.Value = &v
		mv.Origin = nOrigin
		if nOrigin != dOrigin {
			mv.Origin = "event+reconcile"
			mv.Note = "分子分母来源不同(" + nOrigin + "/" + dOrigin + ")"
		}
	case strings.HasPrefix(expr, "gauge(count("):
		if items == nil {
			mv.Display = "—"
			mv.Note = "无轮询数据"
			return mv, nil
		}
		v := float64(countWhere(items, expr))
		mv.Value = &v
		mv.Origin = "reconcile"
	default:
		mv.Display = "—"
		mv.Note = "未实现的算子"
		return mv, nil
	}

	if mv.Value != nil {
		mv.Display = fmt.Sprintf("%s%s", strconv.FormatFloat(*mv.Value, 'f', -1, 64), unitSuffix(m.Unit))
		health, alerts := a.applyThresholds(wf, n, m, *mv.Value, probeAlive, now)
		mv.Health = health
		return mv, alerts
	}
	if mv.Display == "" {
		mv.Display = "—"
	}
	return mv, nil
}

// applyThresholds 应用阈值(即时/持续);持续阈值经 DurationTracker 判定,告警按 30m 去重。
func (a *Aggregator) applyThresholds(wf *registry.Workflow, n *registry.Node, m *registry.Metric, v float64, probeAlive bool, now time.Time) (string, []AlertView) {
	if m.Thresholds == nil {
		return HealthGreen, nil
	}
	if a.Tracker == nil {
		a.Tracker = NewDurationTracker()
	}
	for _, level := range []string{HealthRed, HealthYellow} { // 红优先
		expr, ok := m.Thresholds[level]
		if !ok {
			continue
		}
		te := ParseThresh(expr)
		if te.Op == "" {
			continue
		}
		cond := te.Match(v)
		if te.NeedProbe {
			cond = cond && probeAlive
		}
		if te.Dur == 0 {
			if cond {
				return level, []AlertView{{
					ID: "wf/" + wf.ID + "/" + n.ID + "/" + m.ID, Level: level, Target: wf.Name + "/" + n.Name,
					Text: fmt.Sprintf("指标 %s 越阈(%s): %s%s", m.ID, expr,
						strconv.FormatFloat(v, 'f', -1, 64), unitSuffix(m.Unit)),
					Since: now,
				}}
			}
			continue
		}
		// 持续阈值
		key := wf.ID + "/" + n.ID + "/" + m.ID + "/" + level
		hold := a.historicalHold(wf, n, m, te, now)
		res := a.Tracker.Eval(key, now, cond, te.Dur, alertDedupWindow, hold)
		if res.Firing {
			var alerts []AlertView
			if res.NewAlert {
				alerts = append(alerts, AlertView{
					ID: "wf/" + wf.ID + "/" + n.ID + "/" + m.ID, Level: level, Target: wf.Name + "/" + n.Name,
					Text: fmt.Sprintf("指标 %s 持续越阈(%s,已持续 %s)", m.ID, expr,
						now.Sub(res.Since).Round(time.Minute).String()),
					Since: res.Since,
				})
			}
			return level, alerts
		}
	}
	return HealthGreen, nil
}

// historicalHold 判定"过去 te.Dur 内条件一直成立"(重启后无 30m 盲区)。
// 仅支持 =0 / <1 类 count 条件(窗口内任意一条匹配事件即违背);且事件侧须已覆盖整个窗口。
func (a *Aggregator) historicalHold(wf *registry.Workflow, n *registry.Node, m *registry.Metric, te ThreshExpr, now time.Time) bool {
	if a.Store == nil || a.NATS == nil {
		return false
	}
	if !((te.Op == "=" && te.Limit == 0) || (te.Op == "<" && te.Limit <= 1)) {
		return false
	}
	subj, ok := countSubjectOf(n, strings.TrimSpace(m.Expr))
	if !ok {
		return false
	}
	ns := a.NATS.Lookup(wf.ID + "/" + n.ID)
	if ns == nil || !ns.Consuming || ns.StartedAt.After(now.Add(-te.Dur)) {
		return false // 覆盖不足一个窗口,不假装历史成立
	}
	platform := a.platformOfNode(n)
	if platform == "" {
		return false
	}
	cnt, err := a.Store.CountEvents(platform, subj, now.Add(-te.Dur))
	return err == nil && cnt == 0
}

// countSubjectOf 从 count 类表达式与节点声明解析出事件 subject_key(版本后缀剥离)。
func countSubjectOf(n *registry.Node, expr string) (string, bool) {
	switch {
	case expr == "count(enter)":
		return subjectOfCond(n.Enter)
	case expr == "count(exit)":
		return subjectOfCond(n.Exit)
	case strings.HasPrefix(expr, "count(subject="):
		s := parseSubjectLit(expr)
		return s, s != ""
	case strings.HasPrefix(expr, "count(branch."):
		return branchSubject(n, strings.TrimSuffix(strings.TrimPrefix(expr, "count(branch."), ")"))
	}
	return "", false
}

// subjectOfCond 从 enter/exit 条件表达式解析 event(subject='X')。
func subjectOfCond(cond string) (string, bool) {
	re := regexp.MustCompile(`event\(subject='([^']+)'\)`)
	m := re.FindStringSubmatch(cond)
	if len(m) != 2 {
		return "", false
	}
	return adapter.SubjectKey(m[1]), true
}

// branchSubject 按分支名(terminal.xxx 尾段)找 branch_out 的 when subject。
func branchSubject(n *registry.Node, name string) (string, bool) {
	for _, b := range n.BranchOut {
		if strings.HasSuffix(b.To, name) || strings.HasSuffix(b.To, "."+name) {
			return subjectOfCond(b.When)
		}
	}
	return "", false
}

// parseSubjectLit 解析 count(subject='X') 字面量并剥离版本后缀。
func parseSubjectLit(expr string) string {
	re := regexp.MustCompile(`count\(subject='([^']+)'\)`)
	m := re.FindStringSubmatch(expr)
	if len(m) != 2 {
		return ""
	}
	return adapter.SubjectKey(m[1])
}

// countEnterExit 计算 count(enter)/count(exit):webhook 节点走投递表,nats/hybrid 走事件表。
func (a *Aggregator) countEnterExit(wf *registry.Workflow, n *registry.Node, enter bool, now time.Time, win time.Duration) (float64, string, bool) {
	if n.Source == nil {
		return 0, "无数据源声明", false
	}
	if n.Source.Adapter == adapter.KindWebhook {
		if !strings.HasPrefix(n.Enter, "webhook(") || !enter {
			return 0, "webhook 节点仅 enter 可计数", false
		}
		v := float64(a.Receiver.CountSince(webhookSourceOf(n), webhookEventOf(n), now.Add(-win)))
		note := ""
		if v == 0 {
			note = "窗口内未收到 webhook"
		}
		return v, note, true
	}
	// nats / hybrid:按 enter/exit 的 subject 计数
	cond := n.Exit
	if enter {
		cond = n.Enter
	}
	subj, ok := subjectOfCond(cond)
	if !ok {
		return 0, "enter/exit 非 event(subject=...) 形,不可计数", false
	}
	v, ok := a.countSubject(wf, n, subj, now, win)
	if !ok {
		return 0, "事件侧未接线", false
	}
	return v, "", true
}

// countSubject 统计窗口内某 subject_key 的事件数(平台由 endpoint 映射)。
func (a *Aggregator) countSubject(wf *registry.Workflow, n *registry.Node, subjectKey string, now time.Time, win time.Duration) (float64, bool) {
	if a.Store == nil {
		return 0, false
	}
	platform := a.platformOfNode(n)
	if platform == "" {
		return 0, false
	}
	cnt, err := a.Store.CountEvents(platform, subjectKey, now.Add(-win))
	if err != nil {
		return 0, false
	}
	return float64(cnt), true
}

// countOperand 计算 ratio 的单个 count 操作数;返回 (值, 是否可算, 来源 origin)。
func (a *Aggregator) countOperand(wf *registry.Workflow, n *registry.Node, operand string, items []map[string]any, now time.Time, win time.Duration) (float64, bool, string) {
	op := strings.TrimSpace(operand)
	switch {
	case op == "count(enter)":
		v, _, ok := a.countEnterExit(wf, n, true, now, win)
		return v, ok, "event"
	case op == "count(exit)":
		v, _, ok := a.countEnterExit(wf, n, false, now, win)
		return v, ok, "event"
	case strings.HasPrefix(op, "count(branch."):
		subj, ok := countSubjectOf(n, op)
		if !ok {
			return 0, false, ""
		}
		v, ok := a.countSubject(wf, n, subj, now, win)
		return v, ok, "event"
	case strings.HasPrefix(op, "count(subject="):
		subj := parseSubjectLit(op)
		if subj == "" {
			return 0, false, ""
		}
		v, ok := a.countSubject(wf, n, subj, now, win)
		return v, ok, "event"
	case strings.HasPrefix(op, "count("):
		if items == nil {
			return 0, false, ""
		}
		return float64(countWhere(items, op)), true, "reconcile"
	}
	return 0, false, ""
}

func unitSuffix(u string) string {
	switch u {
	case "%", "s", "ms", "bp":
		return u
	}
	if u == "" {
		return ""
	}
	return " " + u
}

// pollItems 取节点最近一次轮询 items。
func pollItems(p PollQuerier, wf *registry.Workflow, n *registry.Node) []map[string]any {
	if n.Source == nil || n.Source.RestPath == "" || p == nil {
		return nil
	}
	st := p.Lookup("node/" + wf.ID + "/" + n.ID)
	if st == nil || st.LastOKAt.IsZero() {
		return nil
	}
	return st.Items
}

// countWhere 统计 items 中满足 "count(field='v')" 或 "count(field in ('a','b'))" 的元素数;
// 无过滤条件(count(exit)/count(enter) 等)不支持此处(事件侧)。
func countWhere(items []map[string]any, expr string) int {
	field, op, vals := parseCountFilter(expr)
	if field == "" {
		return 0
	}
	n := 0
	for _, it := range items {
		s, _ := it[field].(string)
		if op == "=" && s == vals[0] {
			n++
		}
		if op == "in" && contains(vals, s) {
			n++
		}
	}
	return n
}

// parseCountFilter 解析 count(state='queued') 或 count(status in ('a','b'))。
func parseCountFilter(expr string) (field, op string, vals []string) {
	re := regexp.MustCompile(`count\(\s*([a-zA-Z_]+)\s*(=|in)\s*(.+?)\s*\)`)
	mm := re.FindStringSubmatch(expr)
	if len(mm) != 4 {
		return "", "", nil
	}
	field, op = mm[1], mm[2]
	raw := mm[3]
	if op == "in" {
		raw = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "("), ")")
	}
	for _, part := range strings.Split(raw, ",") {
		vals = append(vals, strings.Trim(strings.TrimSpace(part), `'"`))
	}
	return field, op, vals
}

// splitTopLevelRatio 把 ratio(A, B) 拆成两个顶层子表达式(按深度 0 的逗号切分)。
func splitTopLevelRatio(expr string) []string {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "ratio(") || !strings.HasSuffix(expr, ")") {
		return nil
	}
	inner := expr[len("ratio(") : len(expr)-1]
	depth := 0
	for i, ch := range inner {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return []string{strings.TrimSpace(inner[:i]), strings.TrimSpace(inner[i+1:])}
			}
		}
	}
	return nil
}

// nodeVolume1h 业务量:webhook 投递 + nats 事件近 1h 计数;纯轮询节点计 0。
func (a *Aggregator) nodeVolume1h(wf *registry.Workflow, n *registry.Node, now time.Time) int {
	if n.Source == nil {
		return 0
	}
	switch n.Source.Adapter {
	case adapter.KindWebhook:
		return a.Receiver.CountSince(webhookSourceOf(n), "", now.Add(-time.Hour))
	case adapter.KindNATS, adapter.KindHybrid:
		if a.Store == nil {
			return 0
		}
		platform := a.platformOfNode(n)
		if platform == "" {
			return 0
		}
		total := 0
		for _, s := range n.Source.Subjects {
			cnt, err := a.Store.CountEvents(platform, adapter.SubjectKey(s), now.Add(-time.Hour))
			if err == nil {
				total += cnt
			}
		}
		return total
	}
	return 0
}

// webhookSourceOf 由 enter 表达式 webhook(event='x') 推源名;缺省 aranea。
func webhookSourceOf(n *registry.Node) string {
	re := regexp.MustCompile(`webhook\(event='([^']+)'\)`)
	if m := re.FindStringSubmatch(n.Enter); len(m) == 2 && strings.HasPrefix(m[1], "opportunity.") {
		return "aranea"
	}
	return "aranea"
}

// webhookEventOf 解析 webhook(event='x') 的事件名(投递计数按事件名前缀过滤)。
func webhookEventOf(n *registry.Node) string {
	re := regexp.MustCompile(`webhook\(event='([^']+)'\)`)
	if m := re.FindStringSubmatch(n.Enter); len(m) == 2 {
		return m[1]
	}
	return ""
}

// evalApprovals 聚合待拍板队列(aranea 纯 REST)。
func (a *Aggregator) evalApprovals(now time.Time) ApprovalsView {
	av := ApprovalsView{Health: HealthGray, Items: []ApprovalItem{}}
	var lastOK time.Time
	allFailed := len(a.Cfg.Approvals) > 0
	for _, src := range a.Cfg.Approvals {
		st := a.Poller.Lookup("approval/" + src.Lane)
		if st == nil {
			continue
		}
		if !st.LastOKAt.IsZero() {
			allOK := st.LastOKAt
			if lastOK.IsZero() || allOK.Before(lastOK) {
				lastOK = allOK
			}
			allFailed = false
			for _, it := range st.Items {
				av.Items = append(av.Items, ApprovalItem{
					Lane:  src.Lane,
					ID:    strOfAny(it["ID"]),
					Title: firstNonEmptyStr(strOfAny(it["DraftTitle"]), strOfAny(it["Title"])),
					Extra: strOfAny(it["Status"]),
				})
			}
		}
	}
	av.Count = len(av.Items)
	switch {
	case allFailed:
		av.Health = HealthRed
		av.Note = "数据源不可达(aranea 8800),无法获取待拍板列表"
	case !lastOK.IsZero():
		av.LastOKAt = &lastOK
		if now.Sub(lastOK) > a.StaleDef {
			av.Health = HealthYellow
			av.Note = "数据陈旧"
		} else {
			av.Health = HealthGreen
		}
	default:
		av.Note = "等待首次轮询"
	}
	return av
}

// splitREST 拆 "GET <url>"。
func splitREST(p string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(p), " ", 2)
	if len(parts) == 2 {
		return strings.ToUpper(parts[0]), parts[1]
	}
	return "GET", p
}

// PollKey 导出轮询键规则供装配层组装目标。
func PollKey(wfID, nodeID string) string { return "node/" + wfID + "/" + nodeID }

// SplitREST 导出供装配层使用。
func SplitREST(p string) (string, string) { return splitREST(p) }

func windowDur(w string) time.Duration {
	switch w {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "1h":
		return time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func emptyAs(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func strOfAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmptyStr(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
