package speech

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	nethttp "net/http"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// headerCapture 记录 dial 收到的鉴权头（鉴权模式契约测试用）。
type headerCapture struct {
	header nethttp.Header
}

func newTestTTSProvider(conn *fakeWSConn) biz.StreamingTTSProvider {
	return &volcTTSProvider{
		cfg: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", ResourceID: "rid", Voice: "zh_female_x", SpeedRatio: 1.0,
		},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return conn, nil },
		lg:   loggateway.NewNoop(),
	}
}

func TestPcmS16ToF32(t *testing.T) {
	in := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80} // 0, 32767, -32768
	out := pcmS16ToF32(in)
	require.Len(t, out, 12)
	require.Equal(t, float32(0), math.Float32frombits(binary.LittleEndian.Uint32(out[0:4])))
	require.InDelta(t, 1.0, math.Float32frombits(binary.LittleEndian.Uint32(out[4:8])), 1e-4)
	require.Equal(t, float32(-1.0), math.Float32frombits(binary.LittleEndian.Uint32(out[8:12])))
}

// V3 单向流式上行：full client request 携带 user + req_params（text/speaker/audio_params）。
func TestTTSSubmitRequestFields(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v1", SpeedRatio: 1.2, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("你好世界", true))
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgFullClientRequest, f.msgType)
	require.Equal(t, byte(volcFlagNone), f.flags, "V3 上行 SendText 无 event")
	require.True(t, f.json)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	req := body["req_params"].(map[string]any)
	require.Equal(t, "你好世界", req["text"])
	require.Equal(t, "v1", req["speaker"])
	audio := req["audio_params"].(map[string]any)
	require.Equal(t, "pcm", audio["format"])
	require.Equal(t, float64(16000), audio["sample_rate"])
	require.Equal(t, float64(20), audio["speech_rate"], "1.2x → speech_rate 20")
}

func TestVolcTTSSpeechRateMapping(t *testing.T) {
	require.Equal(t, 0, volcTTSSpeechRate(1.0))
	require.Equal(t, 20, volcTTSSpeechRate(1.2))
	require.Equal(t, 100, volcTTSSpeechRate(2.5))  // 收敛上限
	require.Equal(t, -50, volcTTSSpeechRate(0.25)) // 收敛下限
	require.Equal(t, -20, volcTTSSpeechRate(0.8))
}

// V3 鉴权头：APIKey 模式发 X-Api-Key；legacy 模式发 App/Access Key 对。
func TestTTSAuthHeaderModes(t *testing.T) {
	conn := newFakeWSConn()
	cap := &headerCapture{}
	p := &volcTTSProvider{
		cfg: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", APIKey: "api-key-1", ResourceID: "seed-tts-2.0", Voice: "v", SpeedRatio: 1,
		},
		dial: func(_ context.Context, _ string, h nethttp.Header) (wsConn, error) {
			cap.header = h
			return conn, nil
		},
		lg: loggateway.NewNoop(),
	}
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	require.Equal(t, "api-key-1", cap.header.Get("X-Api-Key"))
	require.Empty(t, cap.header.Get("X-Api-App-Key"))
	require.Equal(t, "seed-tts-2.0", cap.header.Get("X-Api-Resource-Id"))
	require.NotEmpty(t, cap.header.Get("X-Api-Request-Id"))

	conn2 := newFakeWSConn()
	p2 := newTestTTSProvider(conn2)
	cap2 := &headerCapture{}
	p2.(*volcTTSProvider).dial = func(_ context.Context, _ string, h nethttp.Header) (wsConn, error) {
		cap2.header = h
		return conn2, nil
	}
	sess2, err := p2.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess2.Write("x", true))
	defer sess2.Close()
	require.Empty(t, cap2.header.Get("X-Api-Key"))
	require.Equal(t, "ak", cap2.header.Get("X-Api-App-Key"))
	require.Equal(t, "sk", cap2.header.Get("X-Api-Access-Key"))
}

// V3 下行：352 音频事件帧（event+sessionID 布局）→ Data chunk；
// 152 SessionFinished → End chunk；随后服务端关连接，Audio 通道关闭。
func TestTTSAudioChunksUntilSessionFinished(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	pcm16 := []byte{0x01, 0x00, 0x02, 0x00} // 2 samples s16le
	audioFrame, err := marshalVolcFrame(volcFrame{
		msgType: volcMsgAudioOnlyResponse, flags: volcFlagWithEvent, event: volcTTSEventResponse, sessionID: "s-1", payload: pcm16,
	}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: audioFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkData, chunk.Type)
	require.Len(t, chunk.PCM, 8) // 2 samples × 4B f32

	finFrame, err := marshalVolcFrame(volcFrame{
		msgType: volcMsgFullServerResponse, flags: volcFlagWithEvent, event: volcTTSEventSessionFinished, sessionID: "s-1", json: true, payload: []byte(`{}`),
	}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: finFrame}

	chunk = <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkEnd, chunk.Type)

	// 服务端随后关闭连接；Audio 通道关闭
	conn.Close()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sess.Audio():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("audio channel not closed after server close")
		}
	}
}

// 句界事件（350/351）不产生 chunk 也不终结会话。
func TestTTSSentenceBoundaryEventsIgnored(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	_ = mustReadFrame(t, conn)

	for _, ev := range []int32{volcTTSEventSentenceStart, volcTTSEventSentenceEnd} {
		frame, err := marshalVolcFrame(volcFrame{
			msgType: volcMsgFullServerResponse, flags: volcFlagWithEvent, event: ev, sessionID: "s-1", json: true, payload: []byte(`{}`),
		}, false)
		require.NoError(t, err)
		conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: frame}
	}
	select {
	case chunk := <-sess.Audio():
		t.Fatalf("sentence boundary events must not emit chunks, got %#v", chunk)
	case <-time.After(200 * time.Millisecond):
	}
}

// V3 错误帧：errCode 在帧头，payload 为 JSON 错误消息。
func TestTTSErrorFrame(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	errFrame, err := marshalVolcFrame(volcFrame{
		msgType: volcMsgError, flags: volcFlagNone, errCode: 450000001, json: true,
		payload: []byte(`{"code":450000001,"message":"bad speaker"}`),
	}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
	require.Error(t, chunk.Err)
	require.Contains(t, chunk.Err.Error(), "bad speaker")
}

func TestTTSDialFailureEmitsErrorChunk(t *testing.T) {
	p := &volcTTSProvider{
		cfg:  biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://test", APIKey: "api-key-1", Voice: "v", SpeedRatio: 1},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return nil, errors.New("boom") },
		lg:   loggateway.NewNoop(),
	}
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
}

// ---- L5：TTS 连接预热（voice.start 预拨，首句免握手）----

// countingDialer 记录 dial 次数并按序返回连接。
type countingDialer struct {
	mu    sync.Mutex
	calls int
	conns []*fakeWSConn
	err   error
}

func (d *countingDialer) dial(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	conn := newFakeWSConn()
	d.conns = append(d.conns, conn)
	return conn, nil
}

func (d *countingDialer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func newPrewarmTTSProvider(d *countingDialer) *volcTTSProvider {
	return &volcTTSProvider{
		cfg: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", APIKey: "api-key-1", ResourceID: "rid", Voice: "v", SpeedRatio: 1,
		},
		dial: d.dial,
		lg:   loggateway.NewNoop(),
	}
}

// L5：预热后首句 Write 复用温连接（全程仅预热的一次 dial）；温连接一次性，
// 第二句回退新拨。
func TestTTSPrewarmConnReusedOnFirstWrite(t *testing.T) {
	d := &countingDialer{}
	p := newPrewarmTTSProvider(d)

	p.PrewarmTTSConn(context.Background())
	require.Equal(t, 1, d.count(), "prewarm must dial exactly one connection")

	// 首句：复用温连接，不再 dial。
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("第一句", true))
	defer sess.Close()
	require.Equal(t, 1, d.count(), "first sentence must reuse the warm connection")
	f := mustReadFrame(t, d.conns[0])
	require.Equal(t, volcMsgFullClientRequest, f.msgType)

	// 温连接一次性：第二句新拨。
	sess2, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess2.Write("第二句", true))
	defer sess2.Close()
	require.Equal(t, 2, d.count(), "second sentence must dial fresh (warm slot consumed)")
	_ = mustReadFrame(t, d.conns[1])
}

// L5：无预热时 Write 自行拨号（现状行为不变）。
func TestTTSWriteWithoutPrewarmDialsFresh(t *testing.T) {
	d := &countingDialer{}
	p := newPrewarmTTSProvider(d)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	require.Equal(t, 1, d.count())
}

// L5：预热拨号失败仅降级——Write 正常新拨，语音链路不受影响（K3）。
func TestTTSPrewarmFailureDegradesToFreshDial(t *testing.T) {
	d := &countingDialer{err: errors.New("dial boom")}
	p := newPrewarmTTSProvider(d)
	p.PrewarmTTSConn(context.Background()) // 失败，仅内部 Warn

	d.mu.Lock()
	d.err = nil // 恢复网络
	d.mu.Unlock()

	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	require.Equal(t, 2, d.count(), "write must dial fresh after prewarm failure")
	_ = mustReadFrame(t, d.conns[0])
}

// L5：ReleaseWarmTTSConn 关闭未消费的温连接（语音会话 teardown 释放）。
func TestTTSReleaseWarmConnCloses(t *testing.T) {
	d := &countingDialer{}
	p := newPrewarmTTSProvider(d)
	p.PrewarmTTSConn(context.Background())
	require.Equal(t, 1, d.count())

	p.ReleaseWarmTTSConn()
	require.True(t, d.conns[0].closed.Load(), "release must close the unconsumed warm conn")

	// 释放后首句新拨。
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	require.Equal(t, 2, d.count())
}

// L5：温连接被服务端 idle 断连（写失败）时，Write 自动新拨重试一次——
// 温连接死亡不退化为首句丢失，最坏情况 = 现状延迟。
func TestTTSWarmConnDeadRetriesFresh(t *testing.T) {
	d := &countingDialer{}
	p := newPrewarmTTSProvider(d)
	p.PrewarmTTSConn(context.Background())
	require.Equal(t, 1, d.count())

	// 模拟服务端 idle 断连：温连接已关闭，写入必失败。
	_ = d.conns[0].Close()

	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	defer sess.Close()
	require.Equal(t, 2, d.count(), "dead warm conn must trigger one fresh-dial retry")
	f := mustReadFrame(t, d.conns[1])
	require.Equal(t, volcMsgFullClientRequest, f.msgType, "retry must deliver the text frame")
}
