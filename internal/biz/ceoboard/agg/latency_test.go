// latency 物化器的数据验证(移植自 ceo-dashboard/internal/agg/latency_test.go):
// 事件先入内存 fake(口径同 ListPairEvents:occurred_at 缺失退 received_at),
// 再跑物化器配对,分位数与手工计算逐一对齐;并覆盖配对规则、幂等、join 失配跳过。
package agg

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
)

const latencyFixture = `
api_version: ceoboard/v1
kind: Workflow
id: wf.latency-test
name: 延迟测试
biz_line: test
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: b2-signal
    name: 信号
    order: 1
    enter: "event(subject='strategy.signal.updated')"
    exit: "signal(done)"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:4222"
      subjects: ["strategy.signal.updated"]
      dedupe_key: event_id
      field_map: { entity_id: "payload.signal_id" }
  - id: b3-order
    name: 下单
    order: 2
    enter: "event(subject='trade.order')"
    exit: "order(done)"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:4222"
      subjects: ["trade.order"]
      dedupe_key: event_id
      field_map: { entity_id: "payload.signal_id" }
    metrics:
      - { id: m.b2b3.lead, unit: ms, window: 1h, expr: "latency(b2.enter, b3.enter)", precompute: rolling_p50_p95, join: "payload.signal_id" }
`

func latencyReg(t *testing.T, yaml string) *registry.Registry {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir+"/w.yaml", yaml)
	reg, rep, err := registry.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Failed {
		t.Fatalf("声明加载失败 %s: %s", f.File, strings.Join(f.Errors, "; "))
	}
	if len(reg.List()) == 0 {
		t.Fatal("注册表为空")
	}
	return reg
}

func latencyPlatforms() map[string]string {
	return map[string]string{"10.0.0.1:4222": "novatrader"}
}

func insertPairEvent(t *testing.T, st *fakeStore, uid, subjectKey, entity string, occurred, received time.Time) {
	t.Helper()
	st.insertEvent(store.Event{
		UID: uid, Platform: "novatrader", Subject: subjectKey, SubjectKey: subjectKey,
		EventID: uid, EntityID: entity, OccurredAt: occurred, ReceivedAt: received,
		Payload: fmt.Sprintf(`{"signal_id":%q}`, entity),
	})
}

// 落库 5 对事件(手算延迟 100/200/300/400/500ms)→ 物化 → p50=300、p95=400;
// 第二轮物化幂等(0 新增);指标求值路径(evalMetric)取到同一物化值。
func TestLatency_MaterializeMatchesHandCalc(t *testing.T) {
	reg := latencyReg(t, latencyFixture)
	st := newFakeStore()
	base := time.Now().Add(-30 * time.Minute).Truncate(time.Millisecond)

	lats := []int64{100, 200, 300, 400, 500}
	for i, lat := range lats {
		a := base.Add(time.Duration(i) * 10 * time.Second)
		insertPairEvent(t, st, fmt.Sprintf("a%d", i), "strategy.signal.updated", fmt.Sprintf("sig-%d", i), a, a)
		b := a.Add(time.Duration(lat) * time.Millisecond)
		insertPairEvent(t, st, fmt.Sprintf("b%d", i), "trade.order", fmt.Sprintf("sig-%d", i), b, b)
	}
	// 无 A 配对的孤儿 B:不得产出样本
	insertPairEvent(t, st, "b-orphan", "trade.order", "sig-orphan", base, base)

	mat := NewLatencyMaterializer(reg, st, latencyPlatforms(), nil)
	n, notes := mat.MaterializeOnce(time.Now())
	if len(notes) != 0 {
		t.Fatalf("意外跳过: %v", notes)
	}
	if n != 5 {
		t.Fatalf("首轮应产出 5 条样本,实得 %d", n)
	}
	// 幂等:同窗重跑 0 新增
	if n2, _ := mat.MaterializeOnce(time.Now()); n2 != 0 {
		t.Fatalf("重复物化应幂等,实得新增 %d", n2)
	}

	samples, err := st.LatencyWindow("m.b2b3.lead", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 5 {
		t.Fatalf("窗口样本数 = %d, 期望 5", len(samples))
	}
	// 手算:排序 [100 200 300 400 500],p50 idx=int(0.5*4)=2 → 300;p95 idx=int(0.95*4)=3 → 400
	if p50, ok := Percentile(samples, 50); !ok || p50 != 300 {
		t.Fatalf("p50 = %v, 期望 300(手算)", p50)
	}
	if p95, ok := Percentile(samples, 95); !ok || p95 != 400 {
		t.Fatalf("p95 = %v, 期望 400(手算)", p95)
	}

	// 求值路径:aggregator 读物化样本出值,origin=event
	ag := &Aggregator{Reg: reg, Store: st, Poller: &fakePoller{},
		Receiver: &fakeReceiver{}, Cfg: &Config{NATSPlatforms: latencyPlatforms()}, Tracker: NewDurationTracker(), StaleDef: 90 * time.Second}
	wf := reg.Get("wf.latency-test")
	mv, _ := ag.evalMetric(wf, &wf.Nodes[1], &wf.Nodes[1].Metrics[0], nil, false, time.Now())
	if mv.Value == nil || *mv.Value != 300 {
		t.Fatalf("evalMetric 值 = %v, 期望 300", mv.Value)
	}
	if mv.Origin != "event" {
		t.Fatalf("origin = %q, 期望 event(物化样本)", mv.Origin)
	}
}

// 配对规则:B 取之前最近的 A;occurred_at 缺失退 received_at;A 晚于 B 不配。
func TestLatency_PairingRules(t *testing.T) {
	reg := latencyReg(t, latencyFixture)
	st := newFakeStore()
	mat := NewLatencyMaterializer(reg, st, latencyPlatforms(), nil)
	base := time.Now().Add(-20 * time.Minute).Truncate(time.Millisecond)

	// sig-near:两个 A,B 应配最近的 A2(5s 处)→ 延迟 3000ms
	insertPairEvent(t, st, "a-near-1", "strategy.signal.updated", "sig-near", base, base)
	insertPairEvent(t, st, "a-near-2", "strategy.signal.updated", "sig-near", base.Add(5*time.Second), base.Add(5*time.Second))
	insertPairEvent(t, st, "b-near", "trade.order", "sig-near", base.Add(8*time.Second), base.Add(8*time.Second))

	// sig-noocc:A 信封无 occurred_at(零值),退 received_at=base → 延迟 2000ms
	insertPairEvent(t, st, "a-noocc", "strategy.signal.updated", "sig-noocc", time.Time{}, base)
	insertPairEvent(t, st, "b-noocc", "trade.order", "sig-noocc", base.Add(2*time.Second), base.Add(2*time.Second))

	// sig-late:A 晚于 B → 不配对
	insertPairEvent(t, st, "b-late", "trade.order", "sig-late", base, base)
	insertPairEvent(t, st, "a-late", "strategy.signal.updated", "sig-late", base.Add(time.Second), base.Add(time.Second))

	n, notes := mat.MaterializeOnce(time.Now())
	if len(notes) != 0 {
		t.Fatalf("意外跳过: %v", notes)
	}
	if n != 2 {
		t.Fatalf("应产出 2 条样本(sig-near/sig-noocc),实得 %d", n)
	}
	samples, err := st.LatencyWindow("m.b2b3.lead", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, v := range samples {
		got[v] = true
	}
	if !got[3000] || !got[2000] {
		t.Fatalf("样本集合 = %v, 期望含 3000(最近A) 与 2000(occurred退received)", samples)
	}
}

// join 与节点 entity_id 提取路径不一致:跳过并说明,不编配对。
func TestLatency_JoinMismatch_Skips(t *testing.T) {
	yaml := `
api_version: ceoboard/v1
kind: Workflow
id: wf.latency-mismatch
name: 失配测试
biz_line: test
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: b2-signal
    name: 信号
    order: 1
    enter: "event(subject='strategy.signal.updated')"
    exit: "signal(done)"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:4222"
      subjects: ["strategy.signal.updated"]
      field_map: { entity_id: "payload.other_id" }
  - id: b3-order
    name: 下单
    order: 2
    enter: "event(subject='trade.order')"
    exit: "order(done)"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:4222"
      subjects: ["trade.order"]
      field_map: { entity_id: "payload.signal_id" }
    metrics:
      - { id: m.mismatch, unit: ms, window: 1h, expr: "latency(b2.enter, b3.enter)", precompute: rolling_p50, join: "payload.signal_id" }
`
	reg := latencyReg(t, yaml)
	st := newFakeStore()
	base := time.Now().Add(-10 * time.Minute)
	insertPairEvent(t, st, "a1", "strategy.signal.updated", "sig-1", base, base)
	insertPairEvent(t, st, "b1", "trade.order", "sig-1", base.Add(time.Second), base.Add(time.Second))

	mat := NewLatencyMaterializer(reg, st, latencyPlatforms(), nil)
	n, notes := mat.MaterializeOnce(time.Now())
	if n != 0 {
		t.Fatalf("join 失配不应产出样本,实得 %d", n)
	}
	found := false
	for _, note := range notes {
		if strings.Contains(note, "≠ join") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应有 join 失配说明,实得 %v", notes)
	}
}
