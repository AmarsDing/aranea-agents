// consumer 采样与事件归一化单测(移植自 ceo-dashboard/internal/adapter/natsjs_test.go;
// 归一化用例为 P2-6 验收"事件解析"新增)。
package adapter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

type consumerInfoResult struct {
	info *nats.ConsumerInfo
	err  error
}

func (f *fakeSub) ConsumerInfo() (*nats.ConsumerInfo, error) {
	n := int(f.consumerInfoCalls.Add(1)) - 1
	if len(f.consumerInfoResults) == 0 {
		return &nats.ConsumerInfo{}, nil
	}
	if n >= len(f.consumerInfoResults) {
		n = len(f.consumerInfoResults) - 1
	}
	return f.consumerInfoResults[n].info, f.consumerInfoResults[n].err
}

func consumerTestManager(t *testing.T, results []consumerInfoResult) (*NATSManager, *fakeJS) {
	t.Helper()
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	mgr.ConsumerSampleInterval = time.Millisecond
	var addCalls, fetchCalls atomic.Int32
	sub := &fakeSub{fetchCalls: &fetchCalls, consumerInfoResults: results}
	js := &fakeJS{addConsumerCalls: &addCalls, sub: sub}
	return mgr, js
}

func TestConsumerStatesExposeRealNumPendingAndTimestamp(t *testing.T) {
	mgr, js := consumerTestManager(t, []consumerInfoResult{{info: &nats.ConsumerInfo{NumPending: 42}}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := &NATSNodeState{NodeKey: "wf.commerce-content/a1-intel", Connected: true}
	if err := mgr.consumeSubject(ctx, js, "S", "D", "trade.order.v1", "hcf", "", st); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		states := mgr.ConsumerStates()
		if len(states) == 1 && states[0].NumPending == 42 && !states[0].AsOf.IsZero() {
			got := states[0]
			if got.NodeKey != st.NodeKey || got.Platform != "hcf" || got.Stream != "S" || got.Durable != "D" || got.Subject != "trade.order.v1" || !got.Connected {
				t.Fatalf("consumer 元数据错误: %+v", got)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("未暴露真实 ConsumerInfo: %+v", mgr.ConsumerStates())
}

func TestConsumerStatesHideSamplingError(t *testing.T) {
	mgr, js := consumerTestManager(t, []consumerInfoResult{
		{info: &nats.ConsumerInfo{NumPending: 23}},
		{err: errors.New("consumer info unavailable")},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := &NATSNodeState{NodeKey: "wf.t/n1", Connected: true}
	if err := mgr.consumeSubject(ctx, js, "S", "D", "a", "hcf", "", st); err != nil {
		t.Fatal(err)
	}

	var firstAsOf time.Time
	var observedCalls int32
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		states := mgr.ConsumerStates()
		if len(states) == 1 && states[0].SampleError != "" {
			got := states[0]
			if firstAsOf.IsZero() {
				firstAsOf = got.AsOf
				observedCalls = js.sub.consumerInfoCalls.Load()
			}
			if js.sub.consumerInfoCalls.Load() <= observedCalls {
				time.Sleep(time.Millisecond)
				continue
			}
			if got.NumPending != 23 {
				t.Fatalf("采样失败不得把未知写成 0 或覆盖上次值: %+v", got)
			}
			if firstAsOf.IsZero() || !got.AsOf.Equal(firstAsOf) {
				t.Fatalf("采样失败必须保留上次成功时间: first=%s got=%s", firstAsOf, got.AsOf)
			}
			return
		}
		if len(states) == 1 && states[0].NumPending == 23 {
			firstAsOf = states[0].AsOf
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("采样错误未显式暴露: %+v", mgr.ConsumerStates())
}

// ---- 事件解析(归一化):P2-6 验收新增 ----

// 去重键优先级:event_id > id > Nats-Msg-Id > 内容摘要;subject 版本后缀剥离。
func TestNormalizeEvent_DedupeKeyPriority(t *testing.T) {
	// event_id 优先
	ev, err := NormalizeEvent("hcf", "mm.intel.normalized.v1",
		[]byte(`{"event_id":"e-1","id":"e-2"}`), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ev.UID != "hcf:e-1" || ev.EventID != "e-1" {
		t.Fatalf("event_id 应最优先: %+v", ev)
	}
	if ev.SubjectKey != "mm.intel.normalized" {
		t.Fatalf("版本后缀应剥离: %q", ev.SubjectKey)
	}
	// 无 event_id 时 id 次之
	ev, _ = NormalizeEvent("hcf", "s.v2", []byte(`{"id":"e-2"}`), nil, "")
	if ev.UID != "hcf:e-2" {
		t.Fatalf("id 应次优先: %+v", ev)
	}
	// 皆无走 Nats-Msg-Id 头
	h := nats.Header{}
	h.Set("Nats-Msg-Id", "nm-9")
	ev, _ = NormalizeEvent("hcf", "s.v1", []byte(`{"x":1}`), h, "")
	if ev.UID != "hcf:nm-9" {
		t.Fatalf("Nats-Msg-Id 应兜底于内容摘要前: %+v", ev)
	}
	// 全缺 → 内容摘要(同 body 同键,幂等)
	ev1, _ := NormalizeEvent("hcf", "s.v1", []byte(`{"x":1}`), nil, "")
	ev2, _ := NormalizeEvent("hcf", "s.v1", []byte(`{"x":1}`), nil, "")
	if ev1.UID != ev2.UID || ev1.EventID[:7] != "sha256:" {
		t.Fatalf("内容摘要应稳定: %q vs %q", ev1.UID, ev2.UID)
	}
}

// occurred_at 解析:RFC3339 / unix 秒 / unix 毫秒;缺失为零值(落库归一 NULL)。
func TestNormalizeEvent_OccurredAtParsing(t *testing.T) {
	ev, _ := NormalizeEvent("hcf", "s.v1", []byte(`{"event_id":"a","occurred_at":"2026-11-03T12:00:00Z"}`), nil, "")
	if ev.OccurredAt.IsZero() || ev.OccurredAt.UTC().Format("2006-01-02") != "2026-11-03" {
		t.Fatalf("RFC3339 未解析: %v", ev.OccurredAt)
	}
	ev, _ = NormalizeEvent("hcf", "s.v1", []byte(`{"event_id":"b","timestamp":1730000000}`), nil, "")
	if ev.OccurredAt.Unix() != 1730000000 {
		t.Fatalf("unix 秒未解析: %v", ev.OccurredAt)
	}
	ms := int64(1730000000123)
	body := []byte(`{"event_id":"c","time":1730000000123}`)
	ev, _ = NormalizeEvent("hcf", "s.v1", body, nil, "")
	if ev.OccurredAt.UnixMilli() != ms {
		t.Fatalf("unix 毫秒未解析: %v", ev.OccurredAt)
	}
	ev, _ = NormalizeEvent("hcf", "s.v1", []byte(`{"event_id":"d"}`), nil, "")
	if !ev.OccurredAt.IsZero() {
		t.Fatalf("无时间字段应为零值: %v", ev.OccurredAt)
	}
}

// entityPath 点路径提取实体 id;路径缺失/类型不符为空。
func TestNormalizeEvent_EntityPath(t *testing.T) {
	body := []byte(`{"event_id":"e","payload":{"signal_id":"sig-1"}}`)
	ev, _ := NormalizeEvent("hcf", "s.v1", body, nil, "payload.signal_id")
	if ev.EntityID != "sig-1" {
		t.Fatalf("点路径应取到实体: %q", ev.EntityID)
	}
	ev, _ = NormalizeEvent("hcf", "s.v1", body, nil, "payload.missing.x")
	if ev.EntityID != "" {
		t.Fatalf("缺失路径应为空: %q", ev.EntityID)
	}
}

// 非 JSON:返回解码错误(进死信),事件仍以摘要键落库;原文保留。
func TestNormalizeEvent_NonJSONStillPersistable(t *testing.T) {
	ev, err := NormalizeEvent("hcf", "mm.intel.raw", []byte("{bad"), nil, "")
	if err == nil {
		t.Fatal("非 JSON 应返回解码错误")
	}
	if ev.UID == "" || ev.Payload != "{bad" {
		t.Fatalf("摘要键与原文必须保留: %+v", ev)
	}
}

// durable 名清洗:非法字符归一为 -,同名输入恒同名(风险点⑩ 重启复用)。
func TestSanitizeDurable_Stable(t *testing.T) {
	in := "wf.t/n1+mm.intel.>"
	got := sanitizeDurable(in)
	if got != "wf-t-n1-mm-intel--" {
		t.Fatalf("清洗结果: %q", got)
	}
	if sanitizeDurable(in) != got {
		t.Fatal("同名输入必须恒同名")
	}
}
