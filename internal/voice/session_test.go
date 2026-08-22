package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeBus struct {
	ch   chan biz.Event
	subs int32
}

func newFakeBus() *fakeBus                            { return &fakeBus{ch: make(chan biz.Event, 32)} }
func (f *fakeBus) Publish(context.Context, biz.Event) {}
func (f *fakeBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	atomic.AddInt32(&f.subs, 1)
	return f.ch, func() {}
}
func (f *fakeBus) subscriberCount() int { return int(atomic.LoadInt32(&f.subs)) }

type fakeExecutor struct {
	mu     sync.Mutex
	inputs []ChatTurnInput
	err    error
	errs   []error // 脚本：每次调用弹出头部错误；弹完后回退 err
}

func (f *fakeExecutor) ExecuteTurn(_ context.Context, in ChatTurnInput) error {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	} else {
		err = f.err
	}
	f.mu.Unlock()
	return err
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inputs)
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
	writeMu sync.Mutex
	writes  []string
}

func (f *scriptedTTSSession) Write(text string, _ bool) error {
	f.writeMu.Lock()
	f.writes = append(f.writes, text)
	f.writeMu.Unlock()
	return nil
}

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
	mu       sync.Mutex
	script   func() *scriptedTTSSession
	sessions []*scriptedTTSSession
}

func (p *scriptedTTSProvider) Open(context.Context, biz.TTSSessionConfig) (biz.TTSSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var s *scriptedTTSSession
	if p.script != nil {
		s = p.script()
	} else {
		s = &scriptedTTSSession{
			chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1, 2, 3, 4}}, {Type: biz.TTSAudioChunkEnd}},
			closed: make(chan struct{}),
		}
	}
	p.sessions = append(p.sessions, s)
	return s, nil
}

// allWrites 汇总全部已开 TTS 会话的合成文本（按开序）。
func (p *scriptedTTSProvider) allWrites() []string {
	p.mu.Lock()
	sessions := make([]*scriptedTTSSession, len(p.sessions))
	copy(sessions, p.sessions)
	p.mu.Unlock()
	var out []string
	for _, s := range sessions {
		s.writeMu.Lock()
		out = append(out, s.writes...)
		s.writeMu.Unlock()
	}
	return out
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

func voiceBusyErr() error {
	return apierror.Conflict("CHAT_TURN_BUSY", "session turn is starting; retry in a moment or use enqueue")
}

func TestIsTurnBusyError(t *testing.T) {
	require.False(t, isTurnBusyError(nil))
	require.False(t, isTurnBusyError(errors.New("boom")))
	require.False(t, isTurnBusyError(apierror.Conflict("OTHER_DOMAIN", "nope")))
	require.False(t, isTurnBusyError(apierror.Internal("CHAT_TURN_BUSY", "wrong code")))
	require.True(t, isTurnBusyError(voiceBusyErr()))
	require.True(t, isTurnBusyError(fmt.Errorf("wrap: %w", voiceBusyErr())))
}

func TestSessionTurnBusyRetryThenSuccess(t *testing.T) {
	fx := newSessionFixture(t)
	// 前两次撞 CHAT_TURN_BUSY（准入 TOCTOU 竞态），第三次放行。
	fx.exec.errs = []error{voiceBusyErr(), voiceBusyErr()}
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 3 }, 3*time.Second, 10*time.Millisecond)
	// 重试成功：整轮不失败（无 voice.error 下行）
	for _, ty := range fx.down.typesOf() {
		require.NotEqual(t, "voice.error", ty)
	}
}

func TestSessionTurnBusyExhaustedFails(t *testing.T) {
	fx := newSessionFixture(t)
	fx.exec.err = voiceBusyErr() // 恒 busy：重试耗尽后走失败路径
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 3 }, 3*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		for _, ty := range fx.down.typesOf() {
			if ty == "voice.error" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
	// 恰好 3 次尝试（1 + 2 次重试），不多不少
	require.Equal(t, 3, fx.exec.callCount())
}

func TestSessionTurnNonBusyErrorNoRetry(t *testing.T) {
	fx := newSessionFixture(t)
	fx.exec.err = errors.New("boom")
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	// 非 busy 错误不重试：快速失败
	time.Sleep(600 * time.Millisecond)
	require.Equal(t, 1, fx.exec.callCount())
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
	fx.bus.ch <- biz.NewStepStreamingEvent("sess-1", "task-1", "step-1", "content", "你好呀，我是助手。")
	require.Eventually(t, func() bool {
		fx.down.mu.Lock()
		defer fx.down.mu.Unlock()
		return len(fx.down.audios) > 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "speaking" }, 2*time.Second, 10*time.Millisecond)

	// Turn 结束 → flush 残余 → tts.end → 回 listening
	fx.bus.ch <- biz.NewTurnCompletedEvent(biz.Turn{SpiritSessionID: "sess-1"})
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

// reopenASRProvider 每次 Open 返回新会话（模拟火山「每句一连接」真机行为）。
type reopenASRProvider struct {
	mu       sync.Mutex
	sessions []*fakeASRSession
}

func (p *reopenASRProvider) Open(context.Context, biz.ASRSessionConfig) (biz.ASRSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := &fakeASRSession{events: make(chan biz.ASREvent, 8)}
	p.sessions = append(p.sessions, s)
	return s, nil
}

func (p *reopenASRProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sessions)
}

func (p *reopenASRProvider) at(i int) *fakeASRSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions[i]
}

// 真机回归（2026-08-09）：火山在末帧应答后以 close 1000 关闭连接、事件流终结。
// asrPump 必须摘掉已终结的 ASR 会话，否则第二句话写已关闭连接报 ASR_WRITE。
func TestWriteAudioReopensASRAfterUpstreamClose(t *testing.T) {
	prov := &reopenASRProvider{}
	down := &fakeDownlink{}
	deps := SessionDeps{
		NewASR: func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return prov, biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000}, nil
		},
		NewTTS: func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return &scriptedTTSProvider{}, biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000}, nil
		},
		Bus:       newFakeBus(),
		Executor:  &fakeExecutor{},
		Canceller: &fakeCanceller{},
		Infra:     nil,
		LG:        loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	require.Equal(t, 1, prov.count())

	// 第一句音频写入会话1
	sess.WriteAudio([]byte{0x01})
	first := prov.at(0)
	require.Eventually(t, func() bool {
		first.writeMu.Lock()
		defer first.writeMu.Unlock()
		return len(first.written) == 1
	}, 2*time.Second, 10*time.Millisecond)

	// 上游终结事件流（火山末帧后 close 1000：readPump 退出并关闭 events）
	_ = first.Close()
	require.Eventually(t, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.asr == nil
	}, 2*time.Second, 10*time.Millisecond)

	// 第二句音频：必须懒重开会话2 并写入，不再写已终结的会话1
	sess.WriteAudio([]byte{0x02})
	require.Equal(t, 2, prov.count())
	second := prov.at(1)
	require.Eventually(t, func() bool {
		second.writeMu.Lock()
		defer second.writeMu.Unlock()
		return len(second.written) == 1 && second.written[0][0] == 0x02
	}, 2*time.Second, 10*time.Millisecond)
	first.writeMu.Lock()
	defer first.writeMu.Unlock()
	require.Len(t, first.written, 1)
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
	fx.bus.ch <- biz.NewStepStreamingEvent("sess-1", "task-1", "step-1", "content", "很长的句子正在合成中。")
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

// ---- V11-T3：噪声终稿过滤（设计 §17.4）----

// 语气词终稿不建 Turn、不进 Chat 管线、状态停留 listening；正常语句不受影响。
func TestSessionNoiseFinalFiltered(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "嗯", DurationMs: 900}
	// asr.final 仍下行（字幕），但不得出现 turn.accepted、不得派发执行
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "asr.final") >= 0
	}, 2*time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond) // 留窗口防异步漏派
	require.Equal(t, 0, fx.exec.callCount())
	require.Equal(t, -1, indexOf(fx.down.typesOf(), "turn.accepted"))
	require.Equal(t, "listening", fx.down.lastState())

	// 正常语句照常建 Turn（回归）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "帮我打开微信", DurationMs: 1200}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
}

// 极短含混音（<300ms 且 ≤2 字）同样被过滤；时长未知（0）时宁留不杀。
func TestSessionNoiseFinalTooShortFiltered(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 200}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "asr.final") >= 0
	}, 2*time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0, fx.exec.callCount())

	// 同文本但时长未知：照建 Turn（durationMs=0 不启用 F3）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
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

// ---- 听写模式（voice.start mode=dictation）----

// 听写终稿仅下行 asr.final 文本：不建 Chat Turn、不订阅 TTS 事件流、状态停留 listening。
func TestSessionDictationFinalDownlinksTextOnly(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeDictation, SampleRate: 16000})
	require.Equal(t, "listening", fx.down.lastState())
	require.Equal(t, 0, fx.bus.subscriberCount(), "听写模式无 TTS，不得订阅事件总线")

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "asr.final") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	// 不建 Turn（异步派发也不会发生）、状态停留 listening、无 turn.accepted。
	require.Never(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) > 0
	}, 200*time.Millisecond, 10*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
	require.Equal(t, -1, indexOf(fx.down.typesOf(), "turn.accepted"))
}

// 听写内容原样下行：确认词表不拦截（「好的」是文本内容而非确认决议）。
func TestSessionDictationSkipsConfirmInterception(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.resolved = true
	fx.sess.Start(StartParams{Mode: ModeDictation})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "好的"}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "asr.final") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, 0, fx.conf.callCount(), "听写模式不得询问确认 resolver")
	require.Equal(t, -1, indexOf(fx.down.typesOf(), "confirm.resolved"))
	require.Equal(t, "listening", fx.down.lastState())
}

// 听写不创建消息，无留档挂载点：语句 PCM 不缓冲、留档端口不调用。
func TestSessionDictationSkipsArchive(t *testing.T) {
	fx := newSessionFixture(t)
	arch := &fakeArchiver{ref: artifactbiz.Ref{ID: "a", MimeType: "audio/wav"}}
	fx.withArchiver(arch)
	fx.sess.Start(StartParams{Mode: ModeDictation, SampleRate: 16000})

	fx.sess.WriteAudio([]byte{1, 2, 3})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 300}
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "asr.final") >= 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, 0, arch.callCount())
	fx.sess.mu.Lock()
	require.Empty(t, fx.sess.utterBuf, "听写模式不得累积留档缓冲")
	fx.sess.mu.Unlock()
}

// 连续听写：多句终稿均下行，状态始终停留 listening（可一直听写直到用户停止）。
func TestSessionDictationConsecutiveFinals(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeDictation})

	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "第一句"}
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "第二句"}
	require.Eventually(t, func() bool {
		count := 0
		for _, ty := range fx.down.typesOf() {
			if ty == "asr.final" {
				count++
			}
		}
		return count == 2
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
	require.Never(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) > 0
	}, 200*time.Millisecond, 10*time.Millisecond)
}

// ---- F2：澄清问题语音播报 ----

// 澄清门触发（step.created kind=clarify）时，问题必须经 TTS 口播——语音会话
// 看不到澄清卡片（UI 组件），turn 挂起后若无播报用户将面对静默。作答走既有
// 自由文本澄清路径（service.resolveClarificationFreeText，与文字消息同入口）。
func TestSessionClarificationBroadcast(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})

	// 语音提问 → turn 派发（thinking）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "帮我做个管理系统", DurationMs: 1200}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	// 澄清门触发：step.created(kind=clarify) 事件经事件总线到达语音会话
	env := biz.ClarificationEnvelope{
		Version: 1, Kind: biz.ClarificationEnvelopeKind,
		Questions: []biz.ClarificationQuestion{
			{Question: "目标用户是谁", Mode: biz.ClarificationModeSingle, Options: []string{"内部员工", "外部客户"}},
			{Question: "预期上线时间？"},
		},
	}
	raw, err := json.Marshal(env)
	require.NoError(t, err)
	fx.bus.ch <- biz.NewStepCreatedEvent(biz.Step{
		ID: "task-1-clarify", TaskID: "task-1", SessionID: "sess-1", SpiritSessionID: "sess-1",
		Kind: biz.StepKindClarify, Content: string(raw), Status: biz.StepStatusAwaitingInput,
	})
	// turn 挂起返回空回复 → turn.completed 驱动 flush/drain 收尾
	fx.bus.ch <- biz.NewTurnCompletedEvent(biz.Turn{SpiritSessionID: "sess-1"})

	// TTS 收到口播文本：两题题干 + 选项 + 引导语
	require.Eventually(t, func() bool {
		joined := strings.Join(fx.ttsProv.allWrites(), "")
		return strings.Contains(joined, "目标用户是谁") &&
			strings.Contains(joined, "内部员工") && strings.Contains(joined, "外部客户") &&
			strings.Contains(joined, "预期上线时间")
	}, 2*time.Second, 10*time.Millisecond)

	// 播报完成：tts.end 下行 + 状态回 listening（等待用户语音作答）
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "tts.end") >= 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
}

// 非 clarify 的 step.created（reply/thinking 等）不触发口播。
func TestSessionClarificationBroadcastSkipsNonClarify(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 500}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	fx.bus.ch <- biz.NewStepCreatedEvent(biz.Step{
		ID: "s-1", TaskID: "task-1", SessionID: "sess-1", SpiritSessionID: "sess-1",
		Kind: biz.StepKindReply, Content: "普通回复",
	})
	require.Never(t, func() bool { return len(fx.ttsProv.allWrites()) > 0 }, 200*time.Millisecond, 20*time.Millisecond)
}

// 口播文本渲染：空题/单题/多题/选项/补标点。
func TestClarificationSpeech(t *testing.T) {
	require.Empty(t, clarificationSpeech(&biz.ClarificationEnvelope{}))
	require.Empty(t, clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{{Question: "  "}}}))

	single := clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{
		{Question: "平台？", Options: []string{"Web", "iOS"}},
	}})
	require.Contains(t, single, "一个问题")
	require.Contains(t, single, "平台？")
	require.Contains(t, single, "Web")
	require.Contains(t, single, "iOS")
	require.NotContains(t, single, "第一")
	require.Contains(t, single, "回答")

	multi := clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{
		{Question: "平台？"},
		{Question: "风格"},
	}})
	require.Contains(t, multi, "两个问题")
	require.Contains(t, multi, "第一")
	require.Contains(t, multi, "第二")
	require.Contains(t, multi, "风格。") // 无终止标点补句号
}

func TestRouteEvent_SpeaksCodingTaskApproval(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.sess.mu.Lock()
	fx.sess.pendingTurns = 1 // busy: enqueue instead of live TTS
	fx.sess.mu.Unlock()

	fx.sess.routeEvent(biz.NewSystemNoticeEvent("sess-1", noticeCodingTaskApproval, "编码助手请求确认：运行测试", map[string]any{
		"speak": true,
		"title": "运行测试",
	}))
	fx.sess.mu.Lock()
	got := append([]string(nil), fx.sess.delegationOutbox...)
	fx.sess.mu.Unlock()
	require.Equal(t, []string{"编码助手请求确认：运行测试"}, got)
}

func TestRouteEvent_IgnoresCodingApprovalWithoutSpeak(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.sess.mu.Lock()
	fx.sess.pendingTurns = 1
	fx.sess.mu.Unlock()

	fx.sess.routeEvent(biz.NewSystemNoticeEvent("sess-1", noticeCodingTaskApproval, "不应播报", map[string]any{
		"speak": false,
	}))
	fx.sess.mu.Lock()
	n := len(fx.sess.delegationOutbox)
	fx.sess.mu.Unlock()
	require.Zero(t, n)
}
