package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type voiceTestASRSession struct {
	writeMu sync.Mutex
	written [][]byte
	events  chan biz.ASREvent
}

func (f *voiceTestASRSession) Write(pcm []byte) error {
	f.writeMu.Lock()
	f.written = append(f.written, pcm)
	f.writeMu.Unlock()
	return nil
}
func (f *voiceTestASRSession) Finish() error               { return nil }
func (f *voiceTestASRSession) Events() <-chan biz.ASREvent { return f.events }
func (f *voiceTestASRSession) Close() error                { return nil }

type voiceTestASRProvider struct{ sess *voiceTestASRSession }

func (p *voiceTestASRProvider) Open(context.Context, biz.ASRSessionConfig) (biz.ASRSession, error) {
	return p.sess, nil
}

type voiceTestBus struct{ ch chan biz.Event }

func (b *voiceTestBus) Publish(context.Context, biz.Event) {}
func (b *voiceTestBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return b.ch, func() {}
}

type voiceTestExecutor struct{}

func (voiceTestExecutor) ExecuteTurn(context.Context, WSTurnInput) error { return nil }

func newVoiceTestServer(asr *voiceTestASRSession) *VoiceWSServer {
	return NewVoiceWSServer(
		nil, // sessionAuth：bypass 下 admin 免 ownership
		voiceTestExecutor{},
		nil, // canceller 测试不需要（Cancel 路径在 voice 包单测覆盖）
		func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return &voiceTestASRProvider{sess: asr}, biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000}, nil
		},
		func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return nil, biz.TTSSessionConfig{}, nil // 本组测试不触发 TTS
		},
		&voiceTestBus{ch: make(chan biz.Event, 8)},
		nil,
		loggateway.NewNoop(),
	)
}

func voiceDial(t *testing.T, srv *httptest.Server, sessionID string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/voice?session_id=" + sessionID
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func readVoiceJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	mt, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

func TestVoiceWSRejectsMissingSessionID(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	s := newVoiceTestServer(&voiceTestASRSession{events: make(chan biz.ASREvent, 1)})
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/voice", nil)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestVoiceWSUnauthorizedWithoutBypass(t *testing.T) {
	s := newVoiceTestServer(&voiceTestASRSession{events: make(chan biz.ASREvent, 1)})
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/voice?session_id=s1", nil)
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestVoiceWSStartAndBinaryFrame(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn := voiceDial(t, srv, "s1")
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"voice.start","language":"zh-CN","sample_rate":16000}`)))
	msg := readVoiceJSON(t, conn)
	require.Equal(t, "voice.state", msg["type"])
	require.Equal(t, "listening", msg["state"])

	pcm := []byte{1, 2, 3, 4}
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, pcm))
	require.Eventually(t, func() bool {
		asr.writeMu.Lock()
		defer asr.writeMu.Unlock()
		return len(asr.written) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestVoiceWSSecondConnectionReplacesFirst(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn1 := voiceDial(t, srv, "s1")
	conn2 := voiceDial(t, srv, "s1")
	_ = conn2
	msg := readVoiceJSON(t, conn1)
	require.Equal(t, "voice.replaced", msg["type"])
}

func TestVoiceWSPingPong(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn := voiceDial(t, srv, "s1")
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))
	msg := readVoiceJSON(t, conn)
	require.Equal(t, "pong", msg["type"])
}
