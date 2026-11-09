// 评审 2026-11-09 L1/L2 演练:
//   - L1 同前缀陷阱:sanitizeDurable 有损映射致不同 subject 算出同名 durable,
//     互相判漂移删建(DeliverAll 下放大为重复摄入);命名须含 subject_key 稳定锚
//     且碰撞可消解,既有 durable 名字节级不变。
//   - L2 部分 subject 失败纳入重试循环(成功的不重建),缺口收敛。
package adapter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// recordingJS 记录每次 AddConsumer/UpdateConsumer/DeleteConsumer 的配置(并发安全)。
type recordingJS struct {
	*fakeJS
	mu      sync.Mutex
	added   []nats.ConsumerConfig
	updated []nats.ConsumerConfig
	deleted []string
}

func (j *recordingJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.mu.Lock()
	j.added = append(j.added, *cfg)
	j.mu.Unlock()
	return &nats.ConsumerInfo{Config: *cfg}, nil
}

func (j *recordingJS) UpdateConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.mu.Lock()
	j.updated = append(j.updated, *cfg)
	j.mu.Unlock()
	return &nats.ConsumerInfo{Config: *cfg}, nil
}

func (j *recordingJS) DeleteConsumer(stream, consumer string, opts ...nats.JSOpt) error {
	j.mu.Lock()
	j.deleted = append(j.deleted, consumer)
	j.mu.Unlock()
	return nil
}

func (j *recordingJS) addedFor(subject string) (durables []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range j.added {
		if c.FilterSubject == subject {
			durables = append(durables, c.Durable)
		}
	}
	return durables
}

// L1:同节点两个 subject 的 subject_key 经有损清洗后同名("mm.intel.normalized"
// 与 "mm-intel-normalized" 都算出 "mm-intel-normalized")——命名必须消解为两个
// 稳定 durable;首个继承基础名(存量兼容),且结果是纯函数(重算不变)。
func TestDurableNames_SamePrefixDifferentSubjectsNoCollision(t *testing.T) {
	subjects := []string{"mm.intel.normalized.v1", "mm-intel-normalized"}
	names := durableNames("ceoboard", "wf.t/n1", subjects)
	d1, d2 := names[subjects[0]], names[subjects[1]]
	if d1 == d2 {
		t.Fatalf("同名前缀不同 subject 算出同一 durable(互踩): %q", d1)
	}
	base := "ceoboard-wf-t-n1-mm-intel-normalized"
	if d1 != base {
		t.Fatalf("首个 subject 必须继承基础名(既有 durable 不变): got %q want %q", d1, base)
	}
	// 纯函数:重算(含换序失败重试场景的稳定锚)恒同。
	again := durableNames("ceoboard", "wf.t/n1", subjects)
	if again[subjects[0]] != d1 || again[subjects[1]] != d2 {
		t.Fatalf("durable 名必须确定性稳定: %v vs %v", names, again)
	}
}

// L1 端到端:DeliverAll 开启 + 共用前缀,碰撞 subject 各自建独立 consumer,
// 不得触发不可变漂移的删旧重建(互相覆盖消费)。
func TestWireOnce_CollidingSubjectsDoNotFightOverDurable(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.DeliverAll = true // 双跑口径:从最早 offset 重放——碰撞时事故放大器
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	var fetchCalls atomic.Int32
	js := &recordingJS{fakeJS: &fakeJS{addConsumerCalls: &atomic.Int32{}, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) { return js, nil }

	node := testNode("mm.intel.normalized.v1", "mm-intel-normalized")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", node)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := mgr.Lookup("wf.t/n1"); s != nil && s.Consuming {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	d1 := js.addedFor("mm.intel.normalized.v1")
	d2 := js.addedFor("mm-intel-normalized")
	if len(d1) != 1 || len(d2) != 1 {
		t.Fatalf("两 subject 应各建一次 consumer: %v / %v", d1, d2)
	}
	if d1[0] == d2[0] {
		t.Fatalf("同名前缀不同 subject 共用 durable(互踩): %q", d1[0])
	}
	js.mu.Lock()
	deleted := append([]string(nil), js.deleted...)
	added := append([]nats.ConsumerConfig(nil), js.added...)
	js.mu.Unlock()
	if len(deleted) != 0 {
		t.Fatalf("碰撞 subject 不得互判漂移删旧重建: deleted=%v", deleted)
	}
	for _, c := range added {
		if c.DeliverPolicy != nats.DeliverAllPolicy {
			t.Fatalf("DeliverAll 应对全部新建 consumer 生效: %+v", c)
		}
	}
}

// L1 跨节点面:不同归属(节点×subject_key)抢同一 durable 名必须报错拒绝,
// 同归属重复登记幂等(断线重接线/版本升级)。
func TestClaimDurable_CrossNodeCollisionRejected(t *testing.T) {
	mgr := NewNATSManager(nil, nil, nil)
	if err := mgr.claimDurable("ceoboard-x-a-b-s", "wf.x/a.b", "s"); err != nil {
		t.Fatalf("首次登记: %v", err)
	}
	if err := mgr.claimDurable("ceoboard-x-a-b-s", "wf.x/a.b", "s"); err != nil {
		t.Fatalf("同归属重复登记应幂等: %v", err)
	}
	if err := mgr.claimDurable("ceoboard-x-a-b-s", "wf.x/a-b", "s"); err == nil {
		t.Fatal("同名不同主必须拒绝(静默共享会分流消息、统计串话)")
	}
}

// flakySubjectJS 模拟单 subject 持续失败(badSubject 在 ready 前 Add/Update 均失败)。
type flakySubjectJS struct {
	*fakeJS
	badSubject string
	ready      *atomic.Bool
	mu         sync.Mutex
	addCalls   map[string]int
}

func (j *flakySubjectJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.mu.Lock()
	j.addCalls[cfg.FilterSubject]++
	j.mu.Unlock()
	if cfg.FilterSubject == j.badSubject && !j.ready.Load() {
		return nil, errors.New("nats: permissions violation for consumer on subject")
	}
	return &nats.ConsumerInfo{Config: *cfg}, nil
}

func (j *flakySubjectJS) UpdateConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	if cfg.FilterSubject == j.badSubject && !j.ready.Load() {
		return nil, errors.New("nats: permissions violation for consumer on subject")
	}
	return &nats.ConsumerInfo{Config: *cfg}, nil
}

func (j *flakySubjectJS) adds(subject string) int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.addCalls[subject]
}

// L2:多 subject 接线时单个失败不再"永不重试"——失败项进入主题级重试循环,
// 成功项不重建;故障恢复后缺口收敛,错误钉清除。
func TestWireOnce_FailedSubjectRetriedUntilWired(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.RetryWait = func(int) time.Duration { return time.Millisecond }
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	ready := &atomic.Bool{}
	var fetchCalls atomic.Int32
	js := &flakySubjectJS{
		fakeJS:     &fakeJS{addConsumerCalls: &atomic.Int32{}, sub: &fakeSub{fetchCalls: &fetchCalls}},
		badSubject: "bad.subject.v1",
		ready:      ready,
		addCalls:   map[string]int{},
	}
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) { return js, nil }

	node := testNode("good.subject.v1", "bad.subject.v1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.WatchNode(ctx, "wf.t", node)

	// 阶段一:好 subject 接线,坏 subject 失败中——部分消费可用,错误钉在。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := mgr.Lookup("wf.t/n1"); s != nil && s.Consuming && js.adds("bad.subject.v1") >= 2 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	s := mgr.Lookup("wf.t/n1")
	if s == nil || !s.Consuming {
		t.Fatalf("部分成功应报告 Consuming: %+v", s)
	}
	if js.adds("bad.subject.v1") < 2 {
		t.Fatalf("失败 subject 应纳入重试循环: adds=%d", js.adds("bad.subject.v1"))
	}
	if s.LastErr == "" {
		t.Fatal("失败重试期应有错误钉")
	}
	// 重试若干轮后:成功项不得重建(不重复 PullSubscribe/不重开拉取协程)。
	goodAdds := js.adds("good.subject.v1")
	time.Sleep(20 * time.Millisecond)
	if got := js.adds("good.subject.v1"); got != goodAdds {
		t.Fatalf("成功 subject 不得随失败项重试而重建: %d → %d", goodAdds, got)
	}

	// 阶段二:故障恢复 → 缺口收敛,错误钉清除,成功项仍只建一次。
	ready.Store(true)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := mgr.Lookup("wf.t/n1"); s != nil && s.LastErr == "" && js.adds("bad.subject.v1") >= 2 {
			// 等 bad 侧 consumer 出现在采样视图
			if len(mgr.ConsumerStates()) == 2 {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	s = mgr.Lookup("wf.t/n1")
	if s == nil || !s.Consuming || s.LastErr != "" {
		t.Fatalf("缺口收敛后应全绿: %+v", s)
	}
	if got := len(mgr.ConsumerStates()); got != 2 {
		t.Fatalf("两个 subject 应各有 consumer: %d", got)
	}
	if got := js.adds("good.subject.v1"); got != goodAdds {
		t.Fatalf("收敛全程成功项不得重建: %d → %d", goodAdds, got)
	}
}
