package voice

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// fakePrewarmer 记录 PrewarmTurn 调用（channel 传递 sessionID 供断言）。
type fakePrewarmer struct{ ch chan string }

func (f *fakePrewarmer) PrewarmTurn(_ context.Context, sessionID string) { f.ch <- sessionID }

// newPrewarmFixture 与 newSessionFixture 相同，但注入 TurnPrewarmer。
func newPrewarmFixture(t *testing.T, pw TurnPrewarmer) *sessionFixture {
	t.Helper()
	asr := &fakeASRSession{events: make(chan biz.ASREvent, 8)}
	bus := newFakeBus()
	exec := &fakeExecutor{}
	canc := &fakeCanceller{}
	conf := &fakeConfirmer{}
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
		Confirmer: conf,
		Prewarmer: pw,
		Infra:     nil,
		LG:        loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	return &sessionFixture{sess: sess, asr: asr, bus: bus, exec: exec, cancel: canc, conf: conf, down: down, ttsProv: ttsProv}
}

// C1：voice.start 成功后后台触发 Agent 构建预热，消除首个语音 Turn 冷启动。
func TestSessionStartPrewarmsAgentBuild(t *testing.T) {
	pw := &fakePrewarmer{ch: make(chan string, 1)}
	fx := newPrewarmFixture(t, pw)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	select {
	case id := <-pw.ch:
		require.Equal(t, "sess-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("voice.start must trigger agent build prewarm")
	}
}

// C1：听写模式不建 Turn，不触发预热。
func TestSessionDictationSkipsPrewarm(t *testing.T) {
	pw := &fakePrewarmer{ch: make(chan string, 1)}
	fx := newPrewarmFixture(t, pw)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000, Mode: ModeDictation})
	select {
	case <-pw.ch:
		t.Fatal("dictation mode must not prewarm (no chat turn)")
	case <-time.After(300 * time.Millisecond):
	}
}

// C1：未接线 Prewarmer（nil）时 voice.start 正常工作（默认关闭预热）。
func TestSessionStartWithoutPrewarmer(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	require.Equal(t, "listening", fx.down.lastState())
}

// E：ASR 终稿派发 Turn 时记录 T0，首帧音频下行时可测量首音频延迟。
func TestSessionFirstAudioLatencyMeasured(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "今天天气怎么样", DurationMs: 800}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 3*time.Second, 10*time.Millisecond)

	fx.sess.mu.Lock()
	t0 := fx.sess.turnT0
	fx.sess.mu.Unlock()
	require.False(t, t0.IsZero(), "ASR final dispatch must record turn T0")

	// 模拟首帧 TTS 音频到达：tts.start 下行且 T0 被消费（复位）。
	fx.sess.onTTSAudio([]byte{1, 2, 3, 4})
	require.Contains(t, fx.down.typesOf(), "tts.start")
	fx.sess.mu.Lock()
	consumed := fx.sess.turnT0.IsZero()
	fx.sess.mu.Unlock()
	require.True(t, consumed, "first audio must consume turn T0")
}

// E：取消/打断清除 T0——迟到的音频帧不产生误导性延迟测量。
func TestSessionCancelClearsTurnT0(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "查一下北京天气", DurationMs: 600}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 3*time.Second, 10*time.Millisecond)

	fx.sess.Cancel("test")
	fx.sess.mu.Lock()
	cleared := fx.sess.turnT0.IsZero()
	fx.sess.mu.Unlock()
	require.True(t, cleared, "cancel must clear turn T0")
}
