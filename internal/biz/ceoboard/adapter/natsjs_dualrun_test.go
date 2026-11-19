// 双跑编排开关(P2-11,方案 §10 P2)单测:
// 独立 durable 名前缀 + 最早 offset 重放(DeliverAllPolicy)。
package adapter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// 默认口径:前缀 "ceoboard"、DeliverNew(不改动即现网行为)。
func TestDualRun_DefaultsUnchanged(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	if mgr.DurablePrefix != "ceoboard" {
		t.Fatalf("默认 durable 前缀应为 ceoboard,实际 %q", mgr.DurablePrefix)
	}
	if mgr.DeliverAll {
		t.Fatal("默认不应开启最早 offset 重放")
	}
}

// 独立 durable 名:自定义前缀进入 durable 名组合(防与旧看板同名形成 queue-group 分流)。
func TestDualRun_DurablePrefixComposed(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.DurablePrefix = "ceoboard-v2"
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	var addCalls, fetchCalls atomic.Int32
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", testNode("trade.order.v1"))

	deadline := time.Now().Add(2 * time.Second)
	want := "ceoboard-v2-wf-t-n1-trade-order"
	for time.Now().Before(deadline) {
		found := false
		for _, cs := range mgr.ConsumerStates() {
			if cs.Durable == want {
				found = true
				break
			}
		}
		if found {
			return // 通过
		}
		time.Sleep(5 * time.Millisecond)
	}
	var got []string
	for _, cs := range mgr.ConsumerStates() {
		got = append(got, cs.Durable)
	}
	t.Fatalf("durable 名未按自定义前缀组合: 期望含 %q,实际 %v", want, got)
}

// 最早 offset 重放:DeliverAll=true 时新建 consumer 用 DeliverAllPolicy;
// false(默认)保持 DeliverNewPolicy(历史回填不打爆窗口指标)。
func TestDualRun_DeliverAllPolicySwitch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var addCalls, fetchCalls atomic.Int32
	js := &captureConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	if err := mgr.consumeSubject(ctx, js, "S", "D1", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if js.cfg.DeliverPolicy != nats.DeliverNewPolicy {
		t.Fatalf("默认应为 DeliverNewPolicy,实际 %v", js.cfg.DeliverPolicy)
	}

	mgr2 := NewNATSManager(openTestStore(t), nil, nil)
	mgr2.DeliverAll = true
	js2 := &captureConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	if err := mgr2.consumeSubject(ctx, js2, "S", "D2", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if js2.cfg.DeliverPolicy != nats.DeliverAllPolicy {
		t.Fatalf("DeliverAll=true 应为 DeliverAllPolicy,实际 %v", js2.cfg.DeliverPolicy)
	}
}
