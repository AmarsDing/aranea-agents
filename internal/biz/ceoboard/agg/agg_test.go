// agg 聚合器单测(移植自 ceo-dashboard/internal/agg/agg_test.go)。
// 移植差异:原测试用真实 SQLite store + httptest 轮询;本包依赖已收敛为
// 消费者侧窄接口,改用内存 fake 直填运行态——agg 只读 Lookup/CountSince,
// 语义等价且不依赖 PG/SQLite(F 盘环境窗口外可跑)。
package agg

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
)

// ---------- 内存 fake(agg 只读依赖面 + latency 物化依赖面) ----------

// fakePoller 直填轮询运行态。
type fakePoller struct {
	states map[string]*adapter.PollState
}

func (f *fakePoller) Lookup(key string) *adapter.PollState { return f.states[key] }

// fakeReceiver 直填 webhook 投递计数。
type fakeReceiver struct {
	counts map[string]int // source|event → 24h 计数(测试按窗口直接给)
}

func (f *fakeReceiver) CountSince(source, event string, since time.Time) int {
	if f.counts == nil {
		return 0
	}
	return f.counts[source+"|"+event] + f.counts[source+"|"]
}

// fakeStore 内存事件库:实现 EventReadStore + LatencyStore。
type fakeStore struct {
	events     []store.Event
	samples    map[string]*store.LatencySample // key = metricID + "\x00" + b_uid
	deliveries []store.Delivery
}

func newFakeStore() *fakeStore {
	return &fakeStore{samples: map[string]*store.LatencySample{}}
}

// insertEvent 测试直插(模拟 InsertEvent 成功路径)。
func (f *fakeStore) insertEvent(ev store.Event) {
	ev.PayloadComplete = true
	f.events = append(f.events, ev)
}

func (f *fakeStore) RecentDeliveries(source string, limit int) ([]store.Delivery, error) {
	out := []store.Delivery{}
	for i := len(f.deliveries) - 1; i >= 0 && len(out) < limit; i-- {
		if f.deliveries[i].Source == source {
			out = append(out, f.deliveries[i])
		}
	}
	return out, nil
}

func (f *fakeStore) CountEvents(platform, subjectKey string, since time.Time) (int, error) {
	n := 0
	for _, e := range f.events {
		if e.Platform == platform && e.SubjectKey == subjectKey && !e.ReceivedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) LatencyWindow(metricID string, since time.Time) ([]int64, error) {
	out := []int64{}
	for _, s := range f.samples {
		if s.MetricID == metricID && !s.At.Before(since) {
			out = append(out, s.LatencyMs)
		}
	}
	return out, nil
}

func (f *fakeStore) ListPairEvents(platform, subjectKey string, since time.Time) ([]store.PairEvent, error) {
	out := []store.PairEvent{}
	for _, e := range f.events {
		if e.Platform != platform || e.SubjectKey != subjectKey || e.EntityID == "" || e.ReceivedAt.Before(since) {
			continue
		}
		pe := store.PairEvent{UID: e.UID, EntityID: e.EntityID, At: e.OccurredAt}
		if e.OccurredAt.IsZero() {
			pe.At = e.ReceivedAt
		}
		out = append(out, pe)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	if len(out) > 5000 {
		out = out[:5000]
	}
	return out, nil
}

func (f *fakeStore) UpsertLatencySample(sm *store.LatencySample) (bool, error) {
	k := sm.MetricID + "\x00" + sm.BUID
	if _, ok := f.samples[k]; ok {
		return false, nil
	}
	cp := *sm
	f.samples[k] = &cp
	return true, nil
}

// ---------- 测试辅助 ----------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 排序:红>黄>绿>灰 → 告警数 → 业务量;折叠规则:全绿/灰组可折叠,红黄组不可。
func TestBuild_SortAndCollapse(t *testing.T) {
	yamlTmpl := `
api_version: ceoboard/v1
kind: Workflow
id: %s
name: %s
biz_line: %s
owner: qa
version: 1
permissions: [ceo]
nodes:
  - { id: n1, name: n1, order: 1, enter: "job(state='queued')", exit: "job(state='done')" }
`
	dir := t.TempDir()
	writeFile(t, dir+"/1.yaml", fmt.Sprintf(yamlTmpl, "wf.green", "绿流", "ecommerce"))
	writeFile(t, dir+"/2.yaml", fmt.Sprintf(yamlTmpl, "wf.red", "红流", "ecommerce"))
	writeFile(t, dir+"/3.yaml", fmt.Sprintf(yamlTmpl, "wf.gray", "灰流", "invest"))
	reg, rep, err := registry.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// 构造:wf.green 节点绿(轮询成功),wf.red 节点红(从未成功)
	reg.Workflows["wf.green"].Nodes[0].Source = &registry.Source{Adapter: "rest-poll", RestPath: "GET http://ok.local/x"}
	reg.Workflows["wf.red"].Nodes[0].Source = &registry.Source{Adapter: "rest-poll", RestPath: "GET http://dead.local/none"}
	p := &fakePoller{states: map[string]*adapter.PollState{
		PollKey("wf.green", "n1"): {LastOKAt: time.Now(), AsOf: time.Now()},
		PollKey("wf.red", "n1"):   {LastErr: "连接被拒绝", FailCount: 3},
	}}

	ag := &Aggregator{Reg: reg, Rep: registry.NewReportHolder(rep), Poller: p, Receiver: &fakeReceiver{},
		Cfg: &Config{}, StaleDef: 90 * time.Second}
	ov := ag.Build(time.Now())

	if len(ov.Lanes) != 2 {
		t.Fatalf("lanes=%d", len(ov.Lanes))
	}
	ec := ov.Lanes[0]
	if ec.BizLine != "ecommerce" {
		t.Fatalf("首组应为 ecommerce,实际 %s", ec.BizLine)
	}
	if ec.Workflows[0].ID != "wf.red" || ec.Workflows[1].ID != "wf.green" {
		t.Fatalf("组内排序错误: %s, %s", ec.Workflows[0].ID, ec.Workflows[1].ID)
	}
	if ec.Collapsible {
		t.Fatal("含红组不应可折叠")
	}
	if !ov.Lanes[1].Collapsible {
		t.Fatal("纯灰组应可折叠")
	}
}

// [待补] 节点显示"数据未接通";NATS 节点在管理器未接线时应标红(事件侧不可用),不再灰色"未接入"。
func TestNodeStatus_PendingAndUnwired(t *testing.T) {
	yaml := `
api_version: ceoboard/v1
kind: Workflow
id: wf.p
name: p
biz_line: ecommerce
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: n1
    name: 待补节点
    order: 1
    enter: "job(state='queued')"
    exit: "job(state='done')"
    source: { adapter: rest-poll, rest_path: "[待补] 路由" }
  - id: n2
    name: NATS节点
    order: 2
    enter: "event(subject='x')"
    exit: "event(subject='y')"
    source: { adapter: nats-jetstream, subjects: ["x"] }
`
	dir := t.TempDir()
	writeFile(t, dir+"/w.yaml", yaml)
	reg, rep, _ := registry.LoadDir(dir)
	ag := &Aggregator{Reg: reg, Rep: registry.NewReportHolder(rep), Poller: &fakePoller{},
		Receiver: &fakeReceiver{}, Cfg: &Config{}, StaleDef: 90 * time.Second}
	ov := ag.Build(time.Now())
	nodes := ov.Lanes[0].Workflows[0].Nodes
	if nodes[0].Health != HealthGray || nodes[0].Reason != "数据未接通(声明待补)" {
		t.Fatalf("待补节点状态错误: %+v", nodes[0])
	}
	// NATS 已实接;管理器缺失(未接线)必须显式红,不许装绿
	if nodes[1].Health != HealthRed || !strings.Contains(nodes[1].Reason, "nats 未接线") {
		t.Fatalf("NATS 节点未接线应标红: %+v", nodes[1])
	}
}
