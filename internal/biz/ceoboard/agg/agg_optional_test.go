// MEDIUM-5 双轨裁决演练:可选流未启用(stream_optional 缺失)不应使
// "reconcile green" 节点永久 yellow——缺失≠中断,健康度跟随对账侧。
package agg

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/registry"
)

// fakeNATS 直填 NATS 运行态(agg 只读 Lookup 窄接口)。
type fakeNATS struct {
	states map[string]*adapter.NATSNodeState
}

func (f *fakeNATS) Lookup(key string) *adapter.NATSNodeState { return f.states[key] }

func optionalRig(t *testing.T, streamOptional bool) *Aggregator {
	yaml := `
api_version: ceoboard/v1
kind: Workflow
id: wf.o
name: o
biz_line: ecommerce
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: n1
    name: 可选流节点
    order: 1
    enter: "event(subject='x')"
    exit: "event(subject='y')"
    source: { adapter: nats-jetstream, subjects: ["x"], rest_path: "GET http://x/v1/ok" }
`
	dir := t.TempDir()
	writeFile(t, dir+"/w.yaml", yaml)
	reg, rep, _ := registry.LoadDir(dir)
	now := time.Now()
	nats := &fakeNATS{states: map[string]*adapter.NATSNodeState{
		"wf.o/n1": {
			NodeKey: "wf.o/n1", Connected: true,
			Stream: "OPT_STREAM", StreamMissing: true, StreamOptional: streamOptional,
			Consuming: false, LastMsgAt: time.Time{},
		},
	}}
	poller := &fakePoller{states: map[string]*adapter.PollState{
		"node/wf.o/n1": {LastOKAt: now.Add(-5 * time.Second), AsOf: now, LatencyMs: 12},
	}}
	return &Aggregator{Reg: reg, Rep: registry.NewReportHolder(rep), Poller: poller,
		Receiver: &fakeReceiver{}, NATS: nats, Store: newFakeStore(), Cfg: &Config{}, StaleDef: 90 * time.Second}
}

func TestReconcileGreenOptionalStreamStaysGreen(t *testing.T) {
	ag := optionalRig(t, true)
	ov := ag.Build(context.Background(), time.Now())
	n := ov.Lanes[0].Workflows[0].Nodes[0]
	if n.Health != HealthGreen {
		t.Fatalf("可选流未启用 + 对账正常应判绿(MEDIUM-5),实际 %s: %s", n.Health, n.Reason)
	}
	if !strings.Contains(n.Reason, "未启用") || !strings.Contains(n.Reason, "对账正常") {
		t.Fatalf("裁决说明应点明「非中断/跟随对账」: %s", n.Reason)
	}
}

func TestReconcileGreenEventMissingNotOptionalStaysYellow(t *testing.T) {
	ag := optionalRig(t, false) // 非 optional 的声明缺失仍按"事件侧中断"判黄
	ov := ag.Build(context.Background(), time.Now())
	n := ov.Lanes[0].Workflows[0].Nodes[0]
	if n.Health != HealthYellow {
		t.Fatalf("非可选声明流缺失 + 对账正常应仍判黄,实际 %s: %s", n.Health, n.Reason)
	}
}
