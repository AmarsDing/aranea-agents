package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
)

// fakeTvChecker 是测试用 TokenVersionChecker。
type fakeTvChecker struct {
	versions map[int64]int64
	err      error
}

func (f fakeTvChecker) CurrentVersion(_ context.Context, userID int64) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.versions[userID], nil
}

// newRevocationTestWSServer 构造最小可用的 WSServer（无事件订阅、Noop 日志）。
func newRevocationTestWSServer(t *testing.T) *WSServer {
	t.Helper()
	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(nil)},
		nil, nil, nil, nil, loggateway.NewNoop(), nil, nil,
	)
	if srv == nil {
		t.Fatal("expected WSServer")
	}
	srv.SetAllowNilSessionAuth(true)
	return srv
}

// newRevocationTestWSPair 建立一对真实 WebSocket 连接（client, server）。
func newRevocationTestWSPair(t *testing.T) (client, serverConn *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverCh := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverCh <- c
	}))
	t.Cleanup(ts.Close)

	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	select {
	case serverConn = <-serverCh:
	case <-time.After(3 * time.Second):
		t.Fatal("server side upgrade timed out")
	}
	return client, serverConn
}

// newRevocationTestWSConn 用真实连接构造 wsConn 并登记进 store。
func newRevocationTestWSConn(t *testing.T, srv *WSServer, sessionID, userID string, tv int64) (*wsConn, *websocket.Conn) {
	t.Helper()
	client, serverConn := newRevocationTestWSPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	wc := &wsConn{
		conn:         serverConn,
		sessionID:    sessionID,
		userID:       userID,
		tokenVersion: tv,
		channels:     map[string]bool{},
		send:         make(chan []byte, 4),
		connCtx:      ctx,
		connCancel:   cancel,
	}
	srv.store.add(sessionID, wc)
	return wc, client
}

// expectCloseCode 读取客户端消息，断言收到指定关闭码。
func expectCloseCode(t *testing.T, client *websocket.Conn, want int) {
	t.Helper()
	_ = client.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := client.ReadMessage(); err != nil {
			var ce *websocket.CloseError
			if !errors.As(err, &ce) {
				t.Fatalf("expected websocket close error, got %v", err)
			}
			if ce.Code != want {
				t.Fatalf("close code = %d, want %d (text=%q)", ce.Code, want, ce.Text)
			}
			return
		}
	}
}

// TestConnStoreUserIndex 校验 userID 次级索引的增删与快照语义。
func TestConnStoreUserIndex(t *testing.T) {
	cs := newConnStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mk := func(session, user string) *wsConn {
		return &wsConn{sessionID: session, userID: user, connCtx: ctx, connCancel: cancel, send: make(chan []byte, 1)}
	}
	a1 := mk("s1", "7")
	a2 := mk("s2", "7")
	b1 := mk("s3", "8")
	cs.add("s1", a1)
	cs.add("s2", a2)
	cs.add("s3", b1)

	var seen []string
	cs.forEachConnForUser("7", func(wc *wsConn) { seen = append(seen, wc.sessionID) })
	if len(seen) != 2 {
		t.Fatalf("user 7 conns = %v, want 2", seen)
	}
	if ids := cs.userIDs(); len(ids) != 2 {
		t.Fatalf("userIDs = %v, want 2 users", ids)
	}

	cs.remove(a1)
	if n := cs.count("s1"); n != 0 {
		t.Fatalf("s1 count = %d, want 0", n)
	}
	seen = nil
	cs.forEachConnForUser("7", func(wc *wsConn) { seen = append(seen, wc.sessionID) })
	if len(seen) != 1 || seen[0] != "s2" {
		t.Fatalf("after remove, user 7 conns = %v, want [s2]", seen)
	}

	cs.remove(a2)
	if ids := cs.userIDs(); len(ids) != 1 || ids[0] != "8" {
		t.Fatalf("userIDs = %v, want [8]", ids)
	}
}

// TestForEachConnForUserReentrantRemove 回调内摘除自身不得死锁
// （快照在锁外回调是硬要求：关闭连接会触发 readPump → removeConn）。
func TestForEachConnForUserReentrantRemove(t *testing.T) {
	cs := newConnStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wc := &wsConn{sessionID: "s1", userID: "7", connCtx: ctx, connCancel: cancel, send: make(chan []byte, 1)}
	cs.add("s1", wc)

	done := make(chan struct{})
	go func() {
		cs.forEachConnForUser("7", func(c *wsConn) { cs.remove(c) })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forEachConnForUser deadlocked on reentrant remove")
	}
	if n := cs.count("s1"); n != 0 {
		t.Fatalf("s1 count = %d, want 0", n)
	}
}

// TestRevokeUserConnections 校验 P1-4 主路径：按用户踢连接，客户端收到 4401。
func TestRevokeUserConnections(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	_, client7 := newRevocationTestWSConn(t, srv, "s1", "7", 0)
	_, client8 := newRevocationTestWSConn(t, srv, "s2", "8", 0)

	srv.revokeUserConnections(7, "session_revoked")

	expectCloseCode(t, client7, wsCloseCodeSessionRevoked)
	if n := srv.store.count("s1"); n != 0 {
		t.Fatalf("revoked user's session still has %d conns", n)
	}
	if ids := srv.store.userIDs(); len(ids) != 1 || ids[0] != "8" {
		t.Fatalf("userIDs = %v, want [8] (other user untouched)", ids)
	}

	// 幂等：再次吊销同一用户不得 panic，其它用户连接仍存活。
	srv.revokeUserConnections(7, "session_revoked")
	if n := srv.store.count("s2"); n != 1 {
		t.Fatalf("unrelated session conns = %d, want 1", n)
	}
	_ = client8
}

// TestSweepRevokedConnections 校验兜底扫描：tv 落后即关闭；版本一致不动。
func TestSweepRevokedConnections(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	_, staleClient := newRevocationTestWSConn(t, srv, "s1", "7", 0) // 服务端已是 3
	newRevocationTestWSConn(t, srv, "s2", "8", 5)                   // 服务端就是 5

	auth.SetTokenVersionChecker(fakeTvChecker{versions: map[int64]int64{7: 3, 8: 5}})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	expectCloseCode(t, staleClient, wsCloseCodeSessionRevoked)
	if n := srv.store.count("s1"); n != 0 {
		t.Fatalf("stale session still has %d conns", n)
	}
	if n := srv.store.count("s2"); n != 1 {
		t.Fatalf("fresh session conns = %d, want 1", n)
	}
}

// TestSweepSkipsOnCheckerError 版本源故障（Redis+DB 均不可用）时不得误杀连接：
// 此时 HTTP/gRPC 面已 fail-closed 拒绝该用户请求，WS 侧不放大为全站断连。
func TestSweepSkipsOnCheckerError(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	_, client := newRevocationTestWSConn(t, srv, "s1", "7", 0)

	auth.SetTokenVersionChecker(fakeTvChecker{err: errors.New("redis+db down")})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	if n := srv.store.count("s1"); n != 1 {
		t.Fatalf("conns after failed sweep = %d, want 1 (must not close on checker error)", n)
	}
	// 连接仍可读（未被关闭）：短读期限应超时而非收到关闭帧。
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := client.ReadMessage()
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		t.Fatalf("connection must stay open on checker error, got close %v", ce)
	}
}

// TestSweepNoopWhenCheckerNotWired checker 未接线（测试/工具进程）时扫描为 no-op。
func TestSweepNoopWhenCheckerNotWired(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	newRevocationTestWSConn(t, srv, "s1", "7", 0)

	auth.SetTokenVersionChecker(nil)
	srv.sweepRevokedConnections(context.Background())

	if n := srv.store.count("s1"); n != 1 {
		t.Fatalf("conns = %d, want 1", n)
	}
}

// TestCloseWithCodeIdempotent 关闭码写入幂等（pub/sub 与兜底扫描竞态）。
func TestCloseWithCodeIdempotent(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	wc, client := newRevocationTestWSConn(t, srv, "s1", "7", 0)

	wc.closeWithCode(wsCloseCodeSessionRevoked, "session_revoked")
	wc.closeWithCode(wsCloseCodeSessionRevoked, "session_revoked")
	wc.close()

	expectCloseCode(t, client, wsCloseCodeSessionRevoked)
}
