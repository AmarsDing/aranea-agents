package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
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

// fakeConfirmer 记录语音确认决议调用并按脚本返回 resolved。
type fakeConfirmer struct {
	mu       sync.Mutex
	calls    []bool // 每次调用的 approved 入参
	resolved bool
	err      error
}

func (f *fakeConfirmer) ResolvePendingConfirm(_ context.Context, _ string, approved bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, approved)
	return f.resolved, f.err
}

func (f *fakeConfirmer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeConfirmer) lastApproved() (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return false, false
	}
	return f.calls[len(f.calls)-1], true
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
	conf    *fakeConfirmer
	down    *fakeDownlink
	ttsProv *scriptedTTSProvider
}

func newSessionFixture(t *testing.T) *sessionFixture {
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
		Infra:     nil, // 测试不发流程日志总线
		LG:        loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	return &sessionFixture{sess: sess, asr: asr, bus: bus, exec: exec, cancel: canc, conf: conf, down: down, ttsProv: ttsProv}
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

// ---- V2-T5：语音确认拦截 ----

// 有待决议确认时，「好的」被拦截：下发 confirm.resolved、不进 Chat 管线、停留 listening。
func TestSessionVoiceConfirmApproveInterceptsTurn(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.resolved = true
	fx.sess.Start(StartParams{})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "好的", DurationMs: 300}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "confirm.resolved") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	// 决议为 approve；状态停留 listening（未进 thinking = 未创建 turn）
	approved, called := fx.conf.lastApproved()
	require.True(t, called)
	require.True(t, approved)
	require.Equal(t, "listening", fx.down.lastState())

	// 拦截路径同步执行于 asrPump：此刻 Executor 不得被调用
	fx.exec.mu.Lock()
	require.Empty(t, fx.exec.inputs)
	fx.exec.mu.Unlock()
}

// 「算了」拦截为 deny。
func TestSessionVoiceConfirmDenyInterceptsTurn(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.resolved = true
	fx.sess.Start(StartParams{})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "算了。"}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "confirm.resolved") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	approved, called := fx.conf.lastApproved()
	require.True(t, called)
	require.False(t, approved)
	fx.exec.mu.Lock()
	require.Empty(t, fx.exec.inputs)
	fx.exec.mu.Unlock()
}

// 无待决议确认（resolved=false）时，「好的」按普通语句进 Chat 管线。
func TestSessionVoiceConfirmFallsThroughWhenNoPending(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.resolved = false
	fx.sess.Start(StartParams{})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "好的"}
	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1 && fx.exec.inputs[0].Content == "好的"
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, fx.conf.callCount()) // 词表命中 → 问过 resolver
	require.Equal(t, -1, indexOf(fx.down.typesOf(), "confirm.resolved"))
}

// resolver 故障时降级：语句照常进 Chat 管线（语音失败不影响文字对话，NFR7）。
func TestSessionVoiceConfirmResolverErrorFallsThrough(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.err = errors.New("store down")
	fx.sess.Start(StartParams{})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "好的"}
	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

// 非确认词表的普通语句不询问 resolver。
func TestSessionVoiceConfirmSkipsNonVocabUtterance(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "帮我打开微信"}
	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, fx.conf.callCount())
}

// ---- V2-T6：语音留档 ----

type archiveCall struct {
	sessionID  string
	wav        []byte
	durationMs int
}

// fakeArchiver 记录留档调用并按脚本返回引用/错误。
type fakeArchiver struct {
	mu    sync.Mutex
	calls []archiveCall
	ref   artifactbiz.Ref
	err   error
}

func (f *fakeArchiver) SaveUtteranceAudio(_ context.Context, sessionID string, wav []byte, durationMs int) (artifactbiz.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, archiveCall{sessionID: sessionID, wav: append([]byte(nil), wav...), durationMs: durationMs})
	return f.ref, f.err
}

func (f *fakeArchiver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeArchiver) lastCall() (archiveCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return archiveCall{}, false
	}
	return f.calls[len(f.calls)-1], true
}

// withArchiver 将留档端口接入 fixture（默认 fixture 不接 = 留档关闭）。
func (fx *sessionFixture) withArchiver(a *fakeArchiver) {
	fx.sess.deps.Archiver = a
}

func lastExecInput(fx *sessionFixture) ChatTurnInput {
	fx.exec.mu.Lock()
	defer fx.exec.mu.Unlock()
	return fx.exec.inputs[len(fx.exec.inputs)-1]
}

// 留档开启：终稿语句 PCM 封装为 WAV 送留档端口，附件引用随 Turn 透传。
func TestSessionArchivesUtteranceOnFinal(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{ref: artifactbiz.Ref{ID: "art-1", Name: "voice-x.wav", MimeType: "audio/wav", Size: 3264}}
	fx.withArchiver(arch)
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{1, 2, 3, 4})
	fx.sess.WriteAudio([]byte{5, 6, 7, 8})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "打开微信", DurationMs: 1200}

	require.Eventually(t, func() bool { return arch.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	call, _ := arch.lastCall()
	require.Equal(t, "sess-1", call.sessionID)
	require.Equal(t, 1200, call.durationMs)
	// WAV 封装：RIFF 头 + 全部上行 PCM
	require.Equal(t, "RIFF", string(call.wav[0:4]))
	require.Equal(t, 44+8, len(call.wav))
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, call.wav[44:])

	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
	in := lastExecInput(fx)
	require.NotNil(t, in.Voice, "语音 Turn 必须携带溯源元数据")
	require.Equal(t, 1200, in.Voice.DurationMs)
	require.NotNil(t, in.Voice.Archive)
	require.Equal(t, "art-1", in.Voice.Archive.ID)
	require.Equal(t, "audio/wav", in.Voice.Archive.MimeType)
}

// 留档端口未接线（nil）：Turn 照常派发，Voice 元数据在但无附件引用。
func TestSessionArchiveDisabledWhenArchiverNil(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{1, 2, 3, 4})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 500}

	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
	in := lastExecInput(fx)
	require.NotNil(t, in.Voice)
	require.Equal(t, 500, in.Voice.DurationMs)
	require.Nil(t, in.Voice.Archive)
}

// 开关关闭（端口返回零值 Ref）：Turn 照常派发，无附件引用。
func TestSessionArchiveSwitchOffSkipsAttachment(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{ref: artifactbiz.Ref{}} // 开关关闭契约：零值 Ref + nil 错误
	fx.withArchiver(arch)
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{9, 9})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 300}

	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
	in := lastExecInput(fx)
	require.NotNil(t, in.Voice)
	require.Nil(t, in.Voice.Archive)
}

// 留档失败降级（K3）：Turn 照常派发，无附件引用。
func TestSessionArchiveFailureDegrades(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{err: errors.New("disk full")}
	fx.withArchiver(arch)
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{1, 2})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 300}

	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1
	}, 2*time.Second, 10*time.Millisecond)
	in := lastExecInput(fx)
	require.NotNil(t, in.Voice)
	require.Nil(t, in.Voice.Archive)
}

// 语句缓冲按终稿切分：两次终稿各自仅含本语句的 PCM。
func TestSessionArchiveBufferResetsBetweenUtterances(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{ref: artifactbiz.Ref{ID: "art-x", MimeType: "audio/wav"}}
	fx.withArchiver(arch)
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{1, 1})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "第一句", DurationMs: 100}
	require.Eventually(t, func() bool { return arch.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	first, _ := arch.lastCall()
	require.Equal(t, []byte{1, 1}, first.wav[44:])

	// 第一句终稿后状态进 thinking；Cancel 在 thinking 态经 EvBargeIn 直接回
	// listening（状态机转换表），第二句音频方可被接收。
	fx.sess.Cancel("test")
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)

	fx.sess.WriteAudio([]byte{2, 2, 2})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "第二句", DurationMs: 150}
	require.Eventually(t, func() bool { return arch.callCount() == 2 }, 2*time.Second, 10*time.Millisecond)
	second, _ := arch.lastCall()
	require.Equal(t, []byte{2, 2, 2}, second.wav[44:], "第二句不得混入第一句的 PCM")
}

// 语音确认拦截的语句不留档（不创建消息，无挂载点）。
func TestSessionVoiceConfirmNotArchived(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{ref: artifactbiz.Ref{ID: "art-c", MimeType: "audio/wav"}}
	fx.withArchiver(arch)
	fx.conf.resolved = true
	fx.sess.Start(StartParams{SampleRate: 16000})

	fx.sess.WriteAudio([]byte{7, 7})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "好的", DurationMs: 200}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "confirm.resolved") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	// 拦截路径同步执行于 asrPump：此刻留档端口不得被调用，且缓冲已清空。
	require.Equal(t, 0, arch.callCount())

	// 下一句正常语句的留档不得混入确认词的 PCM。
	fx.sess.WriteAudio([]byte{8, 8, 8})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "打开微信", DurationMs: 900}
	require.Eventually(t, func() bool { return arch.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	call, _ := arch.lastCall()
	require.Equal(t, []byte{8, 8, 8}, call.wav[44:])
}
