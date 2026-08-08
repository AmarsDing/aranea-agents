package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type readMsg struct {
	mt   int
	data []byte
	err  error
}

// fakeWSConn 是 channel 驱动的 wsConn 假实现。
type fakeWSConn struct {
	written chan []byte
	toRead  chan readMsg
	closed  atomic.Bool
}

func newFakeWSConn() *fakeWSConn {
	return &fakeWSConn{written: make(chan []byte, 32), toRead: make(chan readMsg, 32)}
}

func (f *fakeWSConn) WriteMessage(_ int, data []byte) error {
	if f.closed.Load() {
		return errors.New("closed")
	}
	f.written <- data
	return nil
}

func (f *fakeWSConn) ReadMessage() (int, []byte, error) {
	msg, ok := <-f.toRead
	if !ok {
		return 0, nil, io.EOF
	}
	return msg.mt, msg.data, msg.err
}

func (f *fakeWSConn) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		close(f.toRead)
	}
	return nil
}

func newTestASRProvider(conn *fakeWSConn) biz.StreamingASRProvider {
	return &volcASRProvider{
		cfg: biz.ASRProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", ResourceID: "rid", Language: "zh-CN",
		},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return conn, nil },
		lg:   loggateway.NewNoop(),
	}
}

func mustReadFrame(t *testing.T, conn *fakeWSConn) volcFrame {
	t.Helper()
	select {
	case data := <-conn.written:
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		require.NoError(t, err)
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for written frame")
		return volcFrame{}
	}
}

func pushServerJSON(t *testing.T, conn *fakeWSConn, flags byte, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullServerResponse, flags: flags, json: true, payload: raw}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: frame}
}

// pushServerAck 推入 full client request 的服务端应答（空结果）。SAUC 协议保证
// full client request 必有 full server response，Open 会同步消费该首帧。
func pushServerAck(t *testing.T, conn *fakeWSConn) {
	t.Helper()
	pushServerJSON(t, conn, volcFlagNone, map[string]any{"result": map[string]any{}})
}

func TestASROpenSendsFullClientRequest(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgFullClientRequest, f.msgType)
	require.True(t, f.json)
	// 真机校准（2026-08-08）：bigmodel 端点把 full client request 计入序号空间，
	// 必须显式携带 seq=1（正序号），音频帧从 2 续号，否则服务端报
	// "autoAssignedSequence (2) mismatch sequence in request (1)"。
	require.Equal(t, volcFlagPositiveSeq, f.flags)
	require.Equal(t, int32(1), f.seq)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	audio := body["audio"].(map[string]any)
	require.Equal(t, "pcm", audio["format"])
	require.Equal(t, float64(16000), audio["rate"])
}

func TestASRWriteAndFinishSeq(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()
	_ = mustReadFrame(t, conn) // full client request (seq=1)

	// 音频帧续 full client request 的序号空间：首音频帧 seq=2。
	pcm := bytes.Repeat([]byte{0x01}, 640)
	require.NoError(t, sess.Write(pcm))
	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgAudioOnlyRequest, f.msgType)
	require.Equal(t, int32(2), f.seq)
	require.Equal(t, pcm, f.payload)

	require.NoError(t, sess.Write(pcm))
	f = mustReadFrame(t, conn)
	require.Equal(t, int32(3), f.seq)

	// 末帧：flags=0b0010（最后一包、无序号字段），空 payload。
	require.NoError(t, sess.Finish())
	f = mustReadFrame(t, conn)
	require.Equal(t, volcFlagLastPackage, f.flags)
	require.Empty(t, f.payload)
}

func TestASRPartialAndFinalEvents(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()
	_ = mustReadFrame(t, conn)

	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "你好", "utterances": []any{}},
	})
	ev := <-sess.Events()
	require.Equal(t, biz.ASREventPartial, ev.Type)
	require.Equal(t, "你好", ev.Text)

	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{
			"text":       "你好世界",
			"utterances": []any{map[string]any{"text": "你好世界", "definite": true, "end_time": 1200}},
		},
	})
	ev = <-sess.Events()
	require.Equal(t, biz.ASREventFinal, ev.Type)
	require.Equal(t, "你好世界", ev.Text)
	require.Equal(t, 1200, ev.DurationMs)
}

func TestASRErrorFrameAndClose(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	_ = mustReadFrame(t, conn)

	errFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgError, flags: volcFlagNone, json: true, payload: []byte(`{"code":4501}`)}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}
	ev := <-sess.Events()
	require.Equal(t, biz.ASREventError, ev.Type)
	require.Error(t, ev.Err)

	require.NoError(t, sess.Close())
	// events channel 最终关闭
	_, ok := <-sess.Events()
	require.False(t, ok)
}

func TestASRDialFailureIsUnavailable(t *testing.T) {
	p := &volcASRProvider{
		cfg:  biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk"},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return nil, errors.New("boom") },
		lg:   loggateway.NewNoop(),
	}
	_, err := p.Open(context.Background(), biz.ASRSessionConfig{})
	require.True(t, apierror.IsCode(err, apierror.CodeUnavailable), "got %v", err)
}

// TestASROpenServerSilentTimesOut 覆盖真机事故场景：WS 握手成功但上游永不应答
// （如协议不匹配的 v2 端点），Open 必须在 ack 超时后 fail-fast 返回 UNAVAILABLE，
// 而不是让语音会话静默挂起到 10min 空闲回收。
func TestASROpenServerSilentTimesOut(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	p.(*volcASRProvider).ackTimeout = 100 * time.Millisecond

	start := time.Now()
	_, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	elapsed := time.Since(start)

	require.Error(t, err)
	require.True(t, apierror.IsCode(err, apierror.CodeUnavailable), "got %v", err)
	require.Less(t, elapsed, 3*time.Second, "ack wait must fail fast, not hang")
	require.True(t, conn.closed.Load(), "timed-out conn must be closed")
}

// TestASROpenServerErrorFrame 首帧为服务端错误帧时，Open 返回带服务端详情的错误。
func TestASROpenServerErrorFrame(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	errFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgError, flags: volcFlagNone, json: true, payload: []byte(`{"code":4500,"message":"unsupported protocol"}`)}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}

	_, err = p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.Error(t, err)
	require.True(t, apierror.IsCode(err, apierror.CodeUnavailable), "got %v", err)
	require.Contains(t, err.Error(), "unsupported protocol")
	require.True(t, conn.closed.Load(), "rejected conn must be closed")
}
