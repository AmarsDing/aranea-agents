package speech

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	nethttp "net/http"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

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

func TestTTSSubmitRequestFields(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v1", SpeedRatio: 1.2, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("你好世界", true))
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgFullClientRequest, f.msgType)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	audio := body["audio"].(map[string]any)
	require.Equal(t, "v1", audio["voice_type"])
	require.Equal(t, "pcm", audio["encoding"])
	require.Equal(t, 1.2, audio["speed_ratio"])
	req := body["request"].(map[string]any)
	require.Equal(t, "你好世界", req["text"])
	require.Equal(t, "submit", req["operation"])
}

func TestTTSAudioChunksUntilLastPackage(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	pcm16 := []byte{0x01, 0x00, 0x02, 0x00} // 2 samples s16le
	audioFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyResponse, flags: volcFlagNone, payload: pcm16}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: audioFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkData, chunk.Type)
	require.Len(t, chunk.PCM, 8) // 2 samples × 4B f32

	lastFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyResponse, flags: volcFlagLastPackage, payload: []byte{}}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: lastFrame}

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

func TestTTSErrorFrame(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	errFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgError, flags: volcFlagNone, json: true, payload: []byte(`{"code":3001}`)}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
	require.Error(t, chunk.Err)
}

func TestTTSDialFailureEmitsErrorChunk(t *testing.T) {
	p := &volcTTSProvider{
		cfg:  biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", Voice: "v", SpeedRatio: 1},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return nil, errors.New("boom") },
		lg:   loggateway.NewNoop(),
	}
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
}
