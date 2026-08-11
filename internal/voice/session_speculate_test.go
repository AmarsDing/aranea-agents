package voice

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ---- C2：投机意图（partial 稳定 500ms 触发 + final 注入复用）----

// fakeSpeculator 记录投机触发/解析调用；art 非 nil 时 resolve 注入产物。
type fakeSpeculator struct {
	mu       sync.Mutex
	specText []string
	resolves []string
	art      *intent.Artifact
}

func (f *fakeSpeculator) SpeculateIntent(_ context.Context, _ string, text string) {
	f.mu.Lock()
	f.specText = append(f.specText, text)
	f.mu.Unlock()
}

func (f *fakeSpeculator) WithSpeculativeIntent(ctx context.Context, _ string, finalText string) context.Context {
	f.mu.Lock()
	f.resolves = append(f.resolves, finalText)
	art := f.art
	f.mu.Unlock()
	if art != nil {
		return intent.WithSpeculativeArtifact(ctx, art)
	}
	return ctx
}

func (f *fakeSpeculator) specCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.specText...)
}

// ctxCapturingExecutor 记录 Turn 派发的 ctx（验证投机产物注入）。
type ctxCapturingExecutor struct {
	fakeExecutor
	mu  sync.Mutex
	ctx context.Context
}

func (e *ctxCapturingExecutor) ExecuteTurn(ctx context.Context, in ChatTurnInput) error {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
	return e.fakeExecutor.ExecuteTurn(ctx, in)
}

func (e *ctxCapturingExecutor) lastCtx() context.Context {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ctx
}

// newSpeculateFixture 与 newSessionFixture 相同，但注入 IntentSpeculator（与可选的 ctx 捕获执行器）。
func newSpeculateFixture(t *testing.T, sp IntentSpeculator, exec ChatTurnExecutor) *sessionFixture {
	t.Helper()
	asr := &fakeASRSession{events: make(chan biz.ASREvent, 8)}
	bus := newFakeBus()
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
		Bus:        bus,
		Executor:   exec,
		Canceller:  canc,
		Speculator: sp,
		LG:         loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	fx := &sessionFixture{sess: sess, asr: asr, bus: bus, cancel: canc, down: down, ttsProv: ttsProv}
	if fe, ok := exec.(*fakeExecutor); ok {
		fx.exec = fe
	}
	return fx
}

// C2：对话模式 partial 稳定 500ms 后触发投机意图。
func TestSessionPartialStableTriggersSpeculation(t *testing.T) {
	sp := &fakeSpeculator{}
	fx := newSpeculateFixture(t, sp, &fakeExecutor{})
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "今天天气"}
	require.Eventually(t, func() bool {
		calls := sp.specCalls()
		return len(calls) == 1 && calls[0] == "今天天气"
	}, 2*time.Second, 20*time.Millisecond, "stable partial must trigger speculative intent")
}

// C2：partial 文本持续变化重置稳定计时——未稳定不触发；最终稳定文本触发。
func TestSessionPartialUnstableDelaysSpeculation(t *testing.T) {
	sp := &fakeSpeculator{}
	fx := newSpeculateFixture(t, sp, &fakeExecutor{})
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "今天"}
	time.Sleep(300 * time.Millisecond) // 未到 500ms 稳定窗口
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "今天天气"}
	// 旧文本的计时器已被重置：再过一个完整窗口前不得触发。
	time.Sleep(400 * time.Millisecond)
	require.Empty(t, sp.specCalls(), "changing partial must reset the stability timer")
	require.Eventually(t, func() bool {
		calls := sp.specCalls()
		return len(calls) == 1 && calls[0] == "今天天气"
	}, 2*time.Second, 20*time.Millisecond, "latest stable partial must trigger speculation")
}

// C2：听写模式不建 Turn，不触发投机。
func TestSessionDictationSkipsSpeculation(t *testing.T) {
	sp := &fakeSpeculator{}
	fx := newSpeculateFixture(t, sp, &fakeExecutor{})
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000, Mode: ModeDictation})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "今天天气"}
	time.Sleep(700 * time.Millisecond)
	require.Empty(t, sp.specCalls(), "dictation mode must not speculate (no chat turn)")
}

// C2：final 派发 Turn 时经 resolve 注入投机产物（Turn ctx 携带 Artifact）。
func TestSessionASRFinalInjectsSpeculativeArtifact(t *testing.T) {
	art := &intent.Artifact{RefinedGoal: "查询天气", IntentKind: "question"}
	sp := &fakeSpeculator{art: art}
	exec := &ctxCapturingExecutor{}
	fx := newSpeculateFixture(t, sp, exec)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "今天天气怎么样", DurationMs: 800}
	require.Eventually(t, func() bool { return exec.callCount() == 1 }, 3*time.Second, 10*time.Millisecond)
	got := intent.SpeculativeArtifactFromContext(exec.lastCtx())
	require.Same(t, art, got, "turn ctx must carry the speculative artifact on resolve hit")
}

// C2：取消/打断停止稳定计时——残余 partial 不再触发投机。
func TestSessionCancelStopsSpeculation(t *testing.T) {
	sp := &fakeSpeculator{}
	fx := newSpeculateFixture(t, sp, &fakeExecutor{})
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "今天天气"}
	time.Sleep(200 * time.Millisecond) // 稳定窗口未到达
	fx.sess.Cancel("test")
	time.Sleep(700 * time.Millisecond)
	require.Empty(t, sp.specCalls(), "cancel must stop the stability timer")
}

// C2：终稿停止稳定计时——final 后迟到的计时器回调不再触发投机
// （同句 final 先于 500ms 窗口到达的场景，投机无消费者）。
func TestSessionFinalStopsSpeculationTimer(t *testing.T) {
	sp := &fakeSpeculator{}
	fx := newSpeculateFixture(t, sp, &fakeExecutor{})
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: "你好"}
	time.Sleep(200 * time.Millisecond)
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 300}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(700 * time.Millisecond)
	require.Empty(t, sp.specCalls(), "final must stop the stability timer")
}
