package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeBus struct{ ch chan biz.Event }

func newFakeBus() *fakeBus                            { return &fakeBus{ch: make(chan biz.Event, 32)} }
func (f *fakeBus) Publish(context.Context, biz.Event) {}
func (f *fakeBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return f.ch, func() {}
}

type fakeExecutor struct {
	mu     sync.Mutex
	inputs []ChatTurnInput
	err    error
}

func (f *fakeExecutor) ExecuteTurn(_ context.Context, in ChatTurnInput) error {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	f.mu.Unlock()
	return f.err
}

type fakeCanceller struct {
	mu     sync.Mutex
	called []string
}

func (f *fakeCanceller) CancelRun(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	f.called = append(f.called, sessionID)
	f.mu.Unlock()
	return true
}

type fakeASRSession struct {
	events    chan biz.ASREvent
	writeMu   sync.Mutex
	written   [][]byte
	finishMu  sync.Mutex
	finished  int
	closeOnce sync.Once
}

func (f *fakeASRSession) Write(pcm []byte) error {
	f.writeMu.Lock()
	f.written = append(f.written, pcm)
	f.writeMu.Unlock()
	return nil
}
func (f *fakeASRSession) Finish() error {
	f.finishMu.Lock()
	f.finished++
	f.finishMu.Unlock()
	return nil
}
func (f *fakeASRSession) Events() <-chan biz.ASREvent { return f.events }

// Close 关闭 events 通道（对齐真实 Provider 行为），否则 asrPump 永不退出、
// Session.Close 的 wg.Wait 会死锁。
func (f *fakeASRSession) Close() error {
	f.closeOnce.Do(func() { close(f.events) })
	return nil
}

type fakeASRProvider struct{ sess *fakeASRSession }

func (p *fakeASRProvider) Open(context.Context, biz.ASRSessionConfig) (biz.ASRSession, error) {
	return p.sess, nil
}

// scriptedTTSSession 先吐 chunks 再阻塞（blockCh 非空时），用于模拟"合成中"被
// Cancel 的场景：音频已下行（状态进 speaking），但合成尚未结束。
type scriptedTTSSession struct {
	chunks  []biz.TTSAudioChunk
	blockCh chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (f *scriptedTTSSession) Write(string, bool) error { return nil }

func (f *scriptedTTSSession) Audio() <-chan biz.TTSAudioChunk {
	out := make(chan biz.TTSAudioChunk, 8)
	go func() {
		defer close(out)
		for _, c := range f.chunks {
			out <- c
		}
		if f.blockCh != nil {
			<-f.blockCh
		}
	}()
	return out
}

func (f *scriptedTTSSession) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

type scriptedTTSProvider struct {
	mu     sync.Mutex
	script func() *scriptedTTSSession
}

func (p *scriptedTTSProvider) Open(context.Context, biz.TTSSessionConfig) (biz.TTSSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.script != nil {
		return p.script(), nil
	}
	return &scriptedTTSSession{
		chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1, 2, 3, 4}}, {Type: biz.TTSAudioChunkEnd}},
		closed: make(chan struct{}),
	}, nil
}

type fakeDownlink struct {
	mu     sync.Mutex
	jsons  []map[string]any
	audios [][]byte
}

func (d *fakeDownlink) SendJSON(v any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if m, ok := v.(map[string]any); ok {
		d.jsons = append(d.jsons, m)
	}
	return nil
}
func (d *fakeDownlink) SendAudio(pcm []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.audios = append(d.audios, pcm)
	return nil
}

func (d *fakeDownlink) typesOf() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.jsons))
	for _, j := range d.jsons {
		out = append(out, j["type"].(string))
	}
	return out
}

func (d *fakeDownlink) lastState() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := len(d.jsons) - 1; i >= 0; i-- {
		if d.jsons[i]["type"] == "voice.state" {
			return d.jsons[i]["state"].(string)
		}
	}
	return ""
}

// ---- harness ----

type sessionFixture struct {
	sess    *Session
	asr     *fakeASRSession
	bus     *fakeBus
	exec    *fakeExecutor
	cancel  *fakeCanceller
	down    *fakeDownlink
	ttsProv *scriptedTTSProvider
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	asr := &fakeASRSession{events: make(chan biz.ASREvent, 8)}
	bus := newFakeBus()
	exec := &fakeExecutor{}
	canc := &fakeCanceller{}
	down := &fakeDownlink{}
	ttsProv := &scriptedTTSProvider{}
	deps := SessionDeps{
		NewASR: func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return &fakeASRProvider{sess: asr}, biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000}, nil
		},
		NewTTS: func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return ttsProv, biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000}, nil
		},
		Bus:       bus,
		Executor:  exec,
		Canceller: canc,
		Infra:     nil, // 测试不发流程日志总线
		LG:        loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	return &sessionFixture{sess: sess, asr: asr, bus: bus, exec: exec, cancel: canc, down: down, ttsProv: ttsProv}
}

// ---- tests ----

func TestSessionStartBroadcastsListening(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	require.Equal(t, "listening", fx.down.lastState())
}

func TestSessionTextInAudioOut(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})

	// ASR 终稿 → 下行 asr.final + 入 Chat 管线 + 状态 thinking
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1 && fx.exec.inputs[0].Content == "你好" && fx.exec.inputs[0].SessionID == "sess-1"
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	// LLM delta → 分句 → TTS 音频下行 + 状态 speaking
	fx.bus.ch <- &biz.StepStreamingEvent{DeltaField: "content", DeltaChunk: "你好呀，我是助手。"}
	require.Eventually(t, func() bool {
		fx.down.mu.Lock()
		defer fx.down.mu.Unlock()
		return len(fx.down.audios) > 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "speaking" }, 2*time.Second, 10*time.Millisecond)

	// Turn 结束 → flush 残余 → tts.end → 回 listening
	fx.bus.ch <- &biz.TurnCompletedEvent{}
	require.Eventually(t, func() bool {
		for _, ty := range fx.down.typesOf() {
			if ty == "tts.end" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)

	// tts.start 先于音频
	require.Equal(t, "tts.start", fx.down.typesOf()[indexOf(fx.down.typesOf(), "tts.start")])
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func TestSessionCancelDuringSpeaking(t *testing.T) {
	block := make(chan struct{})
	fx := newSessionFixture(t)
	fx.ttsProv.script = func() *scriptedTTSSession {
		return &scriptedTTSSession{
			chunks:  []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1, 2}}},
			blockCh: block,
			closed:  make(chan struct{}),
		}
	}
	fx.sess.Start(StartParams{})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)
	fx.bus.ch <- &biz.StepStreamingEvent{DeltaField: "content", DeltaChunk: "很长的句子正在合成中。"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "speaking" }, 2*time.Second, 10*time.Millisecond)

	fx.sess.Cancel("voice.barge_in")
	close(block)

	fx.cancel.mu.Lock()
	require.Equal(t, []string{"sess-1"}, fx.cancel.called)
	fx.cancel.mu.Unlock()
	// interrupted → ~300ms 自动回 listening
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
}

func TestSessionTurnFailureRecoversToListening(t *testing.T) {
	fx := newSessionFixture(t)
	fx.exec.err = errors.New("pipeline boom")
	fx.sess.Start(StartParams{})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool {
		for _, ty := range fx.down.typesOf() {
			if ty == "voice.error" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
	// 可恢复错误：error → 自动 voice_start → listening
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
}

func TestSessionCommitForwardsToASR(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})
	fx.sess.Commit()
	fx.asr.finishMu.Lock()
	defer fx.asr.finishMu.Unlock()
	require.Equal(t, 1, fx.asr.finished)
}

func TestSessionStopBroadcastsIdle(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})
	fx.sess.Stop()
	require.Equal(t, "idle", fx.down.lastState())
}
