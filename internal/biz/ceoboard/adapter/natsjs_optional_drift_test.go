// MEDIUM-3/4 演练:consumer 不可变字段漂移删旧重建;可选流等平台启用后自动接线。
package adapter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// driftConsumerJS 模拟「durable 已存在且配置漂移」:existingConfig 非 nil 时
// AddConsumer 报冲突,DeleteConsumer 清除(随后 AddConsumer 成功)。
type driftConsumerJS struct {
	*fakeJS
	addCalls    atomic.Int32
	deleteCalls atomic.Int32
}

func (j *driftConsumerJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.addCalls.Add(1)
	if j.existingConfig != nil {
		return nil, errors.New("nats: consumer name already in use with a different configuration")
	}
	cp := *cfg
	j.existingConfig = &cp
	return &nats.ConsumerInfo{Config: *cfg}, nil
}

func (j *driftConsumerJS) DeleteConsumer(stream, consumer string, opts ...nats.JSOpt) error {
	j.deleteCalls.Add(1)
	j.existingConfig = nil
	return nil
}

// MEDIUM-3:subject 版本升级 v1→v2(durable 名锚定不变 subject_key)时,旧 durable
// 的 FilterSubject 不可变——必须删旧重建,而不是 Update 失败后永久接线失败。
func TestConsumerImmutableDriftDeletesAndRecreates(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var fetchCalls atomic.Int32
	js := &driftConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &atomic.Int32{}, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	// 旧 durable 还锚在 v1(其余字段与目标一致,仅 FilterSubject 漂移)。
	js.existingConfig = &nats.ConsumerConfig{
		Durable: "D", FilterSubject: "mm.intel.raw.v1",
		DeliverPolicy: nats.DeliverNewPolicy, MaxDeliver: -1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.consumeSubject(ctx, js, "S", "D", "mm.intel.raw.v2", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatalf("不可变漂移应删旧重建成功: %v", err)
	}
	if js.deleteCalls.Load() != 1 || js.addCalls.Load() != 2 {
		t.Fatalf("应恰好删 1 次、Add 2 次(冲突 + 重建),实际 del=%d add=%d",
			js.deleteCalls.Load(), js.addCalls.Load())
	}
}

// 同 durable 同 FilterSubject 仅 MaxDeliver 老化 → 必须走 Update 迁移,不得删除。
func TestConsumerMutableDriftUpdatesWithoutDelete(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var fetchCalls atomic.Int32
	var deleteCalls atomic.Int32
	base := &existingConsumerJS{fakeJS: &fakeJS{
		addConsumerCalls: &atomic.Int32{}, sub: &fakeSub{fetchCalls: &fetchCalls},
		deleteCalls: &deleteCalls,
		existingConfig: &nats.ConsumerConfig{
			Durable: "D", FilterSubject: "a", DeliverPolicy: nats.DeliverNewPolicy, MaxDeliver: 3,
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.consumeSubject(ctx, base, "S", "D", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatal("可变字段迁移不得删除 consumer")
	}
	if base.updated.MaxDeliver != -1 {
		t.Fatalf("应 Update 迁移为无限重投,实际 %+v", base.updated)
	}
}

// readyStreamJS 的 StreamInfo 在 ready 后置位前恒返回 not found(模拟平台尚未建流)。
type readyStreamJS struct {
	*fakeJS
	ready *atomic.Bool
}

func (j *readyStreamJS) StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	if !j.ready.Load() {
		return nil, errors.New("nats: stream not found")
	}
	return &nats.StreamInfo{}, nil
}

// MEDIUM-4:stream_optional 声明流缺失 → 慢速重探(不反复失败退避);平台启用
// 建流后自动接线,不再依赖进程重启。
func TestStreamOptionalPendingThenRecovers(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.OptionalProbeInterval = 5 * time.Millisecond
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	ready := &atomic.Bool{}
	var acc, fc atomic.Int32
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &readyStreamJS{fakeJS: &fakeJS{addConsumerCalls: &acc, sub: &fakeSub{fetchCalls: &fc}}, ready: ready}, nil
	}
	node := testNode("mm.map.>")
	node.Source.StreamOptional = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", node)

	// 阶段一:流未建 → StreamMissing 标记 + 未建 consumer(慢探而非失败退避)。
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s := mgr.Lookup("wf.t/n1"); s != nil && s.StreamMissing {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	s := mgr.Lookup("wf.t/n1")
	if s == nil || !s.StreamMissing || !s.StreamOptional {
		t.Fatalf("可选流缺失应标 StreamMissing+StreamOptional: %+v", s)
	}
	if s.LastErr != "" {
		t.Fatalf("可选流缺失不算接线失败,LastErr 应为空: %q", s.LastErr)
	}
	if acc.Load() != 0 {
		t.Fatal("流缺失期不得建 consumer")
	}

	// 阶段二:平台启用建流 → 慢速重探自动接线。
	ready.Store(true)
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s := mgr.Lookup("wf.t/n1"); s != nil && s.Consuming {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	s = mgr.Lookup("wf.t/n1")
	if s == nil || !s.Consuming || s.StreamMissing {
		t.Fatalf("平台建流后应自动接线且清除缺失标记: %+v", s)
	}
	if acc.Load() < 1 {
		t.Fatal("恢复后应建立 consumer")
	}
}

// MEDIUM-4 泄漏面:可选流缺失期每次探测连接都必须摘除关闭(不得累积)。
func TestStreamOptionalProbeReleasesConnEachRound(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, nil, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.OptionalProbeInterval = time.Millisecond
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &readyStreamJS{fakeJS: &fakeJS{addConsumerCalls: &atomic.Int32{}, sub: &fakeSub{fetchCalls: &atomic.Int32{}}}, ready: &atomic.Bool{}}, nil
	}
	node := testNode("mm.map.>")
	node.Source.StreamOptional = true

	ctx, cancel := context.WithCancel(context.Background())
	go mgr.WatchNode(ctx, "wf.t", node)
	// 让慢速重探跑若干轮
	time.Sleep(50 * time.Millisecond)
	cancel()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if trackedConns(mgr) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := trackedConns(mgr); n != 0 {
		t.Fatalf("探测连接每轮必须摘除关闭,实际残留 %d 个", n)
	}
}
