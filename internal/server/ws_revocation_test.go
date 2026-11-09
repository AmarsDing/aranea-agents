package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
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

// TestSweepClosesOnAccountDeleted 评审 H1（2026-11-09）：版本查询命中
// ErrAdminNotFound（账号已删）时扫描必须主动关闭连接——删号后不会再有任何
// bump 事件，skip 会让连接存活至 token 自然过期。与故障 fail-open
// （TestSweepSkipsOnCheckerError）互补：删除是终态，不是抖动。
func TestSweepClosesOnAccountDeleted(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	_, client := newRevocationTestWSConn(t, srv, "s1", "7", 3)

	auth.SetTokenVersionChecker(fakeTvChecker{err: biz.ErrAdminNotFound})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	expectCloseCode(t, client, wsCloseCodeSessionRevoked)
	if n := srv.store.count("s1"); n != 0 {
		t.Fatalf("deleted account's session still has %d conns", n)
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

// ---- 评审 2026-11-09 MEDIUM-2：/v1/voice 纳入会话吊销覆盖 ----

// dialRevocationTestVoice 装配「VoiceWSServer + 吊销登记注入 WSServer」并拨号
// 一条真实语音连接（bypass 鉴权：DevBypassPrincipal → userID=1, tv=0）。
// WS 握手 101 返回即放行，服务端登记发生在 Upgrade 之后，故轮询等待登记完成。
func dialRevocationTestVoice(t *testing.T, wsSrv *WSServer, sessionID string) (*websocket.Conn, *VoiceWSServer) {
	t.Helper()
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	voiceSrv := newVoiceTestServer(&voiceTestASRSession{events: make(chan biz.ASREvent, 1)})
	voiceSrv.SetRevocationRegistrar(wsSrv)
	httpSrv := httptest.NewServer(http.HandlerFunc(voiceSrv.handleVoiceWS))
	t.Cleanup(httpSrv.Close)
	before := extraConnCountForUser(wsSrv, "1")
	conn := voiceDial(t, httpSrv, sessionID)
	waitExtraConnCount(t, wsSrv, "1", before+1)
	return conn, voiceSrv
}

// waitExtraConnCount 轮询等待某用户的吊销登记连接数达到 want（以及 dial 后
// 服务端登记竞态的稳定判定辅助）。
func waitExtraConnCount(t *testing.T, srv *WSServer, userID string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if extraConnCountForUser(srv, userID) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("extra conns for user %s = %d, want %d", userID, extraConnCountForUser(srv, userID), want)
}

// extraConnCountForUser 统计吊销登记表中某用户的连接数（测试断言辅助）。
func extraConnCountForUser(srv *WSServer, userID string) int {
	n := 0
	srv.revocation.forEachExtraForUser(userID, func(RevocableConn) { n++ })
	return n
}

// TestRevocationExtraRegistryIndex 校验外部登记索引的增删、幂等与 nil 安全。
func TestRevocationExtraRegistryIndex(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	a := &voiceRevocableConn{userID: "7", sessionID: "s1"}
	b := &voiceRevocableConn{userID: "7", sessionID: "s2"}
	c := &voiceRevocableConn{userID: "8", sessionID: "s3"}

	srv.RegisterRevocableConn(a)
	srv.RegisterRevocableConn(b)
	srv.RegisterRevocableConn(c)
	srv.RegisterRevocableConn(a) // 重复登记幂等
	srv.RegisterRevocableConn(nil)
	srv.RegisterRevocableConn(&voiceRevocableConn{sessionID: "nouid"}) // 空 userID 拒绝

	if ids := srv.revocation.extraUserIDs(); len(ids) != 2 {
		t.Fatalf("extraUserIDs = %v, want 2 users", ids)
	}
	if n := extraConnCountForUser(srv, "7"); n != 2 {
		t.Fatalf("user 7 extra conns = %d, want 2", n)
	}

	srv.UnregisterRevocableConn(a)
	srv.UnregisterRevocableConn(a) // 幂等
	if n := extraConnCountForUser(srv, "7"); n != 1 {
		t.Fatalf("after unregister, user 7 conns = %d, want 1", n)
	}
	srv.UnregisterRevocableConn(b)
	srv.UnregisterRevocableConn(c)
	if ids := srv.revocation.extraUserIDs(); len(ids) != 0 {
		t.Fatalf("extraUserIDs after drain = %v, want empty", ids)
	}

	// nil 接收者安全（WS 未启用时组合根仍可注入）。
	var nilSrv *WSServer
	nilSrv.RegisterRevocableConn(a)
	nilSrv.UnregisterRevocableConn(a)
}

// TestSweepClosesVoiceConnection 兜底扫描同时覆盖聊天与语音连接：同一用户的
// /v1/ws 与 /v1/voice 连接 tv 均落后时应一并收到 4401。
func TestSweepClosesVoiceConnection(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	_, chatClient := newRevocationTestWSConn(t, srv, "sc", "1", 0)
	voiceClient, _ := dialRevocationTestVoice(t, srv, "sv")

	if n := extraConnCountForUser(srv, "1"); n != 1 {
		t.Fatalf("voice conn not registered for revocation, got %d", n)
	}

	auth.SetTokenVersionChecker(fakeTvChecker{versions: map[int64]int64{1: 3}})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	expectCloseCode(t, voiceClient, wsCloseCodeSessionRevoked)
	expectCloseCode(t, chatClient, wsCloseCodeSessionRevoked)
	if n := extraConnCountForUser(srv, "1"); n != 0 {
		t.Fatalf("revoked voice conn still registered, got %d", n)
	}
}

// TestSweepClosesVoiceConnectionOnAccountDeleted 评审 H1 同语义覆盖语音链路：
// 版本查询命中 ErrAdminNotFound（销户）时语音连接必须主动关闭。
func TestSweepClosesVoiceConnectionOnAccountDeleted(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	voiceClient, _ := dialRevocationTestVoice(t, srv, "sv")

	auth.SetTokenVersionChecker(fakeTvChecker{err: biz.ErrAdminNotFound})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	expectCloseCode(t, voiceClient, wsCloseCodeSessionRevoked)
	if n := extraConnCountForUser(srv, "1"); n != 0 {
		t.Fatalf("deleted account's voice conn still registered, got %d", n)
	}
}

// TestSweepSkipsVoiceOnCheckerError 版本源故障时语音连接同样守 fail-open 边界
// （与 TestSweepSkipsOnCheckerError 同语义，不放大存储抖动为全站断连）。
func TestSweepSkipsVoiceOnCheckerError(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	voiceClient, _ := dialRevocationTestVoice(t, srv, "sv")

	auth.SetTokenVersionChecker(fakeTvChecker{err: errors.New("redis+db down")})
	defer auth.SetTokenVersionChecker(nil)

	srv.sweepRevokedConnections(context.Background())

	if n := extraConnCountForUser(srv, "1"); n != 1 {
		t.Fatalf("voice conn must stay registered on checker error, got %d", n)
	}
	// 连接未被关闭：短读期限应超时而非收到关闭帧。
	_ = voiceClient.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := voiceClient.ReadMessage()
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		t.Fatalf("voice connection must stay open on checker error, got close %v", ce)
	}
	if n := extraConnCountForUser(srv, "1"); n != 1 {
		t.Fatalf("voice conn wrongly revoked, registry count = %d", n)
	}
}

// TestRevokeUserConnectionsClosesVoice pub/sub 主路径：bump 到达时语音连接与
// 聊天连接一并以 4401 关闭；幂等。
func TestRevokeUserConnectionsClosesVoice(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	voiceClient, _ := dialRevocationTestVoice(t, srv, "sv")
	_, otherVoice := dialRevocationTestVoice(t, srv, "sv2")
	_ = otherVoice // 同一 bypass 主体（userID=1），一并被踢

	srv.revokeUserConnections(1, "session_revoked")

	expectCloseCode(t, voiceClient, wsCloseCodeSessionRevoked)
	if n := extraConnCountForUser(srv, "1"); n != 0 {
		t.Fatalf("revoked user's voice conns still registered, got %d", n)
	}

	// 幂等：再次吊销不得 panic。
	srv.revokeUserConnections(1, "session_revoked")
}

// TestVoiceConnUnregisteredOnNaturalClose 语音连接自然断开时吊销登记被摘除
// （不残留 → 后续 bump/扫描不再触及已死连接）。
func TestVoiceConnUnregisteredOnNaturalClose(t *testing.T) {
	srv := newRevocationTestWSServer(t)
	voiceClient, _ := dialRevocationTestVoice(t, srv, "sv")

	_ = voiceClient.Close()
	waitExtraConnCount(t, srv, "1", 0)
}
