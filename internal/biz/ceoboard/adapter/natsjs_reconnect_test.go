// 断连/重连演练(注入式,不碰真实 NATS,不向源系统 publish;移植自
// ceo-dashboard/internal/adapter/natsjs_reconnect_test.go)。
// 移植差异:SQLite 直开 → storetest 基建;NewNATSManager 增日志参数(nil 静默)。
package adapter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"aranea-agents/internal/biz/ceoboard/registry"
)

// fakeJS 是脚本化的 jetAPI:记录 consumer 建立次数,PullSubscribe 返回脚本化 fakeSub。
type fakeJS struct {
	addConsumerCalls *atomic.Int32
	sub              *fakeSub
}

func (f *fakeJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	f.addConsumerCalls.Add(1)
	return &nats.ConsumerInfo{}, nil
}
func (f *fakeJS) UpdateConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	return &nats.ConsumerInfo{}, nil
}
func (f *fakeJS) PullSubscribe(subj, durable string, opts ...nats.SubOpt) (pullSub, error) {
	return f.sub, nil
}
func (f *fakeJS) StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return &nats.StreamInfo{}, nil
}
func (f *fakeJS) StreamNameBySubject(subj string, opts ...nats.JSOpt) (string, error) {
	return "S", nil
}

// fakeSub 脚本化 Fetch:前 failWithConnErr 次返回"连接已关闭"(模拟断连),
// 然后放出 batches 中的消息(含重复投递),耗尽后一直超时(无新流量)。
type fakeSub struct {
	fetchCalls          *atomic.Int32
	failWithConnErr     int
	batches             [][]*nats.Msg
	consumerInfoCalls   atomic.Int32
	consumerInfoResults []consumerInfoResult
}

func (f *fakeSub) Fetch(batch int, opts ...nats.PullOpt) ([]*nats.Msg, error) {
	n := int(f.fetchCalls.Add(1))
	if n <= f.failWithConnErr {
		return nil, errors.New("nats: connection closed") // 断连故障注入
	}
	idx := n - f.failWithConnErr - 1
	if idx < len(f.batches) {
		return f.batches[idx], nil
	}
	return nil, nats.ErrTimeout
}

func mkMsg(subject, id string) *nats.Msg {
	m := nats.NewMsg(subject)
	m.Data = []byte(`{"event_id":"` + id + `","occurred_at":"2026-11-03T12:00:00Z","signal_id":"sig-1"}`)
	return m
}

func testNode(subjects ...string) *registry.Node {
	return &registry.Node{
		ID: "n1", Name: "n1", Order: 1,
		Enter: `event(subject='` + subjects[0] + `')`,
		Source: &registry.Source{
			Adapter: "nats-jetstream", Endpoint: "nats://127.0.0.1:4222",
			Stream: "S", Subjects: subjects,
		},
	}
}

// 演练 1:连接级失败 N 次后恢复 → WatchNode 退避重试、consumer 建立、事件不丢不重。
func TestReconnect_ConnFailureThenRecover(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.FetchRetryWait = time.Millisecond

	var dialCalls atomic.Int32
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) {
		n := dialCalls.Add(1)
		if n <= 2 {
			return nil, errors.New("dial tcp: connect refused") // 前 2 次连接失败
		}
		return nil, nil // 第 3 次成功(fake jsOf 不需要真实 conn)
	}
	var addConsumerCalls atomic.Int32
	var fetchCalls atomic.Int32
	sub := &fakeSub{
		fetchCalls:      &fetchCalls,
		failWithConnErr: 3, // 建好后先断 3 次(Fetch 报连接关闭)
		batches: [][]*nats.Msg{
			{mkMsg("trade.order.v1", "e1"), mkMsg("trade.order.v1", "e2")},
			{mkMsg("trade.order.v1", "e1")}, // 重复投递(JetStream redelivery)
		},
	}
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &fakeJS{addConsumerCalls: &addConsumerCalls, sub: sub}, nil
	}

	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go mgr.WatchNode(watchCtx, "wf.t", testNode("trade.order.v1"))

	// 等消费完成(3s 超时)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := mgr.Lookup("wf.t/n1")
		if s != nil && s.MsgCount == 2 && s.DupCount == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := mgr.Lookup("wf.t/n1")
	if s == nil {
		t.Fatal("节点状态未建立")
	}
	if dialCalls.Load() < 3 {
		t.Fatalf("连接失败应退避重试,dial 实际 %d 次", dialCalls.Load())
	}
	if addConsumerCalls.Load() < 1 {
		t.Fatal("恢复后 consumer 未建立")
	}
	if s.MsgCount != 2 || s.DupCount != 1 {
		t.Fatalf("断连恢复后应不丢不重: 新 %d 重复 %d(期望 2/1)", s.MsgCount, s.DupCount)
	}
	if !s.Consuming {
		t.Fatal("恢复后 Consuming 应为 true")
	}
	// Fetch 断连注入后协程仍继续拉取(消费者随 durable 恢复,不重建也不丢)
	if int(fetchCalls.Load()) <= sub.failWithConnErr+len(sub.batches) {
		t.Fatalf("Fetch 失败后协程未恢复拉取: 总 %d 次", fetchCalls.Load())
	}
	// 落库幂等复核:库里恰好 2 条
	n, err := st.CountEvents(ctx, "test", "trade.order", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("库内应恰好 2 条事件(幂等),实际 %d", n)
	}
}

// 演练 2:声明漂移——声明的流不存在时标 StreamMissing;stream_optional 者不标漂移。
func TestStreamDrift_Detected(t *testing.T) {
	st := openTestStore(t)

	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	var addConsumerCalls atomic.Int32
	var fetchCalls atomic.Int32
	driftJS := &fakeJS{addConsumerCalls: &addConsumerCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}
	// 覆盖 StreamInfo:返回 not found
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &driftStreamJS{fakeJS: driftJS}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", testNode("mm.map.>"))

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s := mgr.Lookup("wf.t/n1")
		if s != nil && s.StreamMissing {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := mgr.Lookup("wf.t/n1")
	if s == nil || !s.StreamMissing {
		t.Fatalf("声明流不存在应标 StreamMissing: %+v", s)
	}
	if addConsumerCalls.Load() != 0 {
		t.Fatal("流不存在时不应建 consumer")
	}
}

// driftStreamJS 的 StreamInfo 恒返回 not found(模拟声明漂移)。
type driftStreamJS struct{ *fakeJS }

func (d *driftStreamJS) StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, errors.New("nats: stream not found")
}

// stream_optional 声明者:流缺失不视为漂移(条件未启用),且不反复重试刷屏。
func TestStreamDrift_OptionalNotDrift(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	var acc atomic.Int32
	var fc atomic.Int32
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &driftStreamJS{fakeJS: &fakeJS{addConsumerCalls: &acc, sub: &fakeSub{fetchCalls: &fc}}}, nil
	}
	node := testNode("mm.map.>")
	node.Source.StreamOptional = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", node)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s := mgr.Lookup("wf.t/n1")
		if s != nil && s.StreamMissing {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := mgr.Lookup("wf.t/n1")
	if s == nil || !s.StreamMissing || !s.StreamOptional {
		t.Fatalf("optional 流缺失应标 StreamMissing+StreamOptional: %+v", s)
	}
}
