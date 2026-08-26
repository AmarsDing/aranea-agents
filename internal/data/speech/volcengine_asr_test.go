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
	// 判停参数（2026-08-15 延迟事故修复）：缺失时 SAUC 走默认语义分句 +
	// force_to_speech_time 默认 10000ms 地板，短句终稿被拖 2.3s~15s。
	req := body["request"].(map[string]any)
	require.Equal(t, float64(800), req["end_window_size"])
	require.Equal(t, float64(0), req["force_to_speech_time"])
}

// readRequestCorpusHotwords 从 full client request 帧提取 request.corpus.context
// 中的热词表（V11-T4）。SAUC bigmodel 官方参数：corpus.context 为 JSON 字符串，
// 格式 {"hotwords":[{"word":"..."}]}。
func readRequestCorpusHotwords(t *testing.T, f volcFrame) []string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	req, ok := body["request"].(map[string]any)
	require.True(t, ok, "full client request must contain request object")
	corpus, ok := req["corpus"].(map[string]any)
	require.True(t, ok, "request must contain corpus object (hotwords injection)")
	ctxStr, ok := corpus["context"].(string)
	require.True(t, ok, "corpus.context must be a JSON string")
	var ctx struct {
		Hotwords []struct {
			Word string `json:"word"`
		} `json:"hotwords"`
	}
	require.NoError(t, json.Unmarshal([]byte(ctxStr), &ctx))
	words := make([]string, 0, len(ctx.Hotwords))
	for _, h := range ctx.Hotwords {
		words = append(words, h.Word)
	}
	return words
}

// TestASROpenInjectsDefaultHotwords 默认热词表（拦截链识别增强）随 full client
// request 注入 request.corpus.context：唤醒词同音表 + 退出词 + 确认词，去重。
func TestASROpenInjectsDefaultHotwords(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()

	f := mustReadFrame(t, conn)
	words := readRequestCorpusHotwords(t, f)
	require.Contains(t, words, "小媛")  // 唤醒词（对应 internal/voice wakeWords）
	require.Contains(t, words, "休息吧") // 退出词（exitWords）
	require.Contains(t, words, "好的")  // 确认批准词（approveWords）
	require.Contains(t, words, "算了")  // 确认否认词（denyWords）
	// 「不用了」同现于 exitWords/denyWords，默认表必须去重（boost 重复无意义）。
	seen := map[string]int{}
	for _, w := range words {
		seen[w]++
	}
	require.Equal(t, 1, seen["不用了"], "hotwords must be deduplicated")
}

// TestASROpenHotwordsOverride biz.ASRProviderConfig.Hotwords 非空时覆盖默认表
// （预留配置通道，暂不接 DB/UI）。
func TestASROpenHotwordsOverride(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	p.(*volcASRProvider).cfg.Hotwords = []string{"自定义热词"}
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, []string{"自定义热词"}, readRequestCorpusHotwords(t, f))
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

// TestASRFinalDedupesCumulativeUtterances 覆盖真机事故（2026-08-09 听写无限输入）：
// SAUC 服务端每帧响应都携带全量 utterances 累积列表，已 definite 的语句会持续
// 出现在后续每一帧中。Provider 必须按游标去重——同一终稿只发射一次 Final，
// 否则听写模式每个重复 Final 都会追加进输入框（无限输入根因）。
func TestASRFinalDedupesCumulativeUtterances(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	pushServerAck(t, conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()
	_ = mustReadFrame(t, conn)

	utt1 := map[string]any{"text": "你好", "definite": true, "end_time": 1200}
	// 帧 1：第一句定稿 → 1 条 Final。
	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "你好", "utterances": []any{utt1}},
	})
	ev := <-sess.Events()
	require.Equal(t, biz.ASREventFinal, ev.Type)
	require.Equal(t, "你好", ev.Text)

	// 帧 2：用户继续说第二句，服务端累积回放 utt1 + 新 partial。
	// 修复前：utt1 被重复发射 Final（无限输入根因）；修复后：仅下行 Partial。
	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "今天", "utterances": []any{utt1}},
	})
	ev = <-sess.Events()
	require.Equal(t, biz.ASREventPartial, ev.Type)
	require.Equal(t, "今天", ev.Text)

	// 帧 3：第二句定稿，累积列表含两条 definite → 只发射新终稿。
	utt2 := map[string]any{"text": "今天天气不错", "definite": true, "end_time": 3500}
	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "今天天气不错", "utterances": []any{utt1, utt2}},
	})
	ev = <-sess.Events()
	require.Equal(t, biz.ASREventFinal, ev.Type)
	require.Equal(t, "今天天气不错", ev.Text)
	require.Equal(t, 3500, ev.DurationMs)

	// 帧 4：同内容回放 → 无任何事件。
	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "", "utterances": []any{utt1, utt2}},
	})
	select {
	case dup := <-sess.Events():
		t.Fatalf("duplicate cumulative replay must not emit event, got %+v", dup)
	case <-time.After(200 * time.Millisecond):
	}
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
