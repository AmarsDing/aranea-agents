// 连接生命周期演练(评审 2026-11-09 H2):dial 成功后的任一接线失败必须
// 摘除并关闭连接(conns 不增不减),Runtime.Stop 必须关闭全部纳管连接——
// 否则 nats.Conn 的读写/ping 协程与 MaxReconnects(-1) 重连循环永久泄漏,
// 重试风暴放大为 FD 耗尽。
package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
)

func trackedConns(m *NATSManager) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.conns)
}

// dial ok + jsOf 失败 → wireOnce 报错,连接被摘除(不得留在纳管清单)。
func TestWireOnceJSFailureReleasesConn(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) { return nil, errors.New("jetstream 不可用") }

	node := testNode("trade.order.v1")
	nodeState := &NATSNodeState{NodeKey: "wf.t/n1"}
	if err := mgr.wireOnce(context.Background(), "wf.t", node, nodeState, "test"); err == nil {
		t.Fatal("jsOf 失败应返回错误")
	}
	if n := trackedConns(mgr); n != 0 {
		t.Fatalf("接线失败连接应被摘除(H2 防泄漏),实际纳管 %d 个", n)
	}
}

// dial ok + 声明流不存在(非 optional)→ wireOnce 报"声明漂移",连接被摘除。
func TestWireOnceDriftReleasesConn(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &driftStreamJS{fakeJS: &fakeJS{}}, nil
	}

	node := testNode("mm.map.>")
	nodeState := &NATSNodeState{NodeKey: "wf.t/n1"}
	if err := mgr.wireOnce(context.Background(), "wf.t", node, nodeState, "test"); err == nil {
		t.Fatal("非 optional 声明流缺失应返回漂移错误")
	}
	if n := trackedConns(mgr); n != 0 {
		t.Fatalf("漂移失败连接应被摘除(H2 防泄漏),实际纳管 %d 个", n)
	}
}

// stream_optional 探测连接按设计保留在纳管清单(StreamInfo 缺失不算接线失败),
// 但必须能被 Close 关闭;Close 对 nil 连接(测试 fake dial 产物)不得 panic,
// 且幂等清空清单。
func TestCloseClosesTrackedConns_NilGuard(t *testing.T) {
	st := openTestStore(t)
	mgr := NewNATSManager(st, map[string]string{"127.0.0.1:4222": "test"}, nil)
	mgr.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nil, nil }
	mgr.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		return &driftStreamJS{fakeJS: &fakeJS{}}, nil
	}

	node := testNode("mm.map.>")
	node.Source.StreamOptional = true
	nodeState := &NATSNodeState{NodeKey: "wf.t/n1"}
	if err := mgr.wireOnce(context.Background(), "wf.t", node, nodeState, "test"); err != nil {
		t.Fatalf("stream_optional 探测不算接线失败,应返回 nil: %v", err)
	}
	if n := trackedConns(mgr); n != 1 {
		t.Fatalf("探测连接应留在纳管清单(Stop 统一关闭),实际 %d 个", n)
	}

	mgr.Close() // nil 连接(nil-guard)+ 清空清单;不得 panic
	if n := trackedConns(mgr); n != 0 {
		t.Fatalf("Close 后纳管清单应为空,实际 %d 个", n)
	}
	mgr.Close() // 幂等:第二次调用不得 panic
}
