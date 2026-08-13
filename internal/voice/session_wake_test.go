package voice

import (
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	"github.com/stretchr/testify/require"
)

// ---- V10：语音唤醒与休眠（需求 §2.12 / 设计 §16）----

// companion 模式进入即待命：dormant 广播、音频门控（ASR 零占用）、事件订阅保持（G1）。
func TestSessionStartCompanionEntersDormant(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion, SampleRate: 16000, Language: "zh-CN"})
	require.Equal(t, "dormant", fx.down.lastState())
	// dormant：音频帧被门控，ASR 上游零占用
	fx.sess.WriteAudio([]byte{1, 2, 3})
	require.Empty(t, fx.asr.written)
	// G1：dormant 保持事件总线订阅（委派终态可达）
	require.Equal(t, 1, fx.bus.subscriberCount())
}

// 唤醒：dormant → listening，ASR 懒启动，自足应答「我在」（不经 Chat Turn）。
func TestSessionWakeFromDormant(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	require.Equal(t, "listening", fx.down.lastState())
	// ASR 懒启动：音频帧照常上行
	fx.sess.WriteAudio([]byte{1, 2, 3})
	require.NotEmpty(t, fx.asr.written)
	// 自足唤醒应答（不占 Chat Turn）
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "我在") }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, fx.exec.callCount())
}

// wake 幂等：仅 dormant 受理（listening/dictation 忽略）。
func TestSessionWakeNoopOutsideDormant(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeDictation}) // listening
	require.Equal(t, "listening", fx.down.lastState())
	fx.sess.Wake("kws")
	require.Equal(t, "listening", fx.down.lastState())
	require.False(t, ttsWritesContain(fx, "我在"))
}

// 静默休眠：listening 60s（测试缩短）无交互 → dormant + ASR 上游关闭。
func TestSessionSleepTimeoutReturnsToDormant(t *testing.T) {
	old := sleepTimeout
	sleepTimeout = 40 * time.Millisecond
	defer func() { sleepTimeout = old }()
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("manual")
	require.Equal(t, "listening", fx.down.lastState())
	require.Eventually(t, func() bool { return fx.down.lastState() == "dormant" }, 2*time.Second, 10*time.Millisecond)
	// 休眠后音频被门控（ASR 已关闭，零占用）
	fx.sess.WriteAudio([]byte{9, 9})
	require.Empty(t, fx.asr.written)
}

// ASR 活动重置静默计时：连续 partial 期间不休眠，静默后到期休眠。
func TestSessionSleepTimerResetOnASRPartial(t *testing.T) {
	old := sleepTimeout
	sleepTimeout = 80 * time.Millisecond
	defer func() { sleepTimeout = old }()
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	// 连续 partial（总时长 > sleepTimeout）：保持聆听
	for i := 0; i < 5; i++ {
		fx.asr.events <- biz.ASREvent{Type: biz.ASREventPartial, Text: fmt.Sprintf("部分 %d", i)}
		time.Sleep(30 * time.Millisecond)
	}
	require.Equal(t, "listening", fx.down.lastState())
	// 停止活动后到期休眠
	require.Eventually(t, func() bool { return fx.down.lastState() == "dormant" }, 2*time.Second, 10*time.Millisecond)
}

// 退出词：自足应答确认后回 dormant，不建 Chat Turn。
func TestSessionExitWordAckThenDormant(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "休息吧"}
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "好的，我先休息了") }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "dormant" }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, fx.exec.callCount())
}

// 退出词优先于确认词（「不用了」重叠，设计 §16.4①）。
func TestSessionExitWordPrecedesConfirm(t *testing.T) {
	fx := newSessionFixture(t)
	fx.conf.resolved = true // 有 pending confirm 时确认拦截本可命中
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "不用了"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "dormant" }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, fx.conf.callCount()) // 退出词优先：不查询确认决议
	require.Equal(t, 0, fx.exec.callCount())
}

// 连说形态：唤醒词剥离后净文本进 Chat 管线。
func TestSessionWakeWordStrippedBeforeTurn(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "小媛，查一下天气"}
	require.Eventually(t, func() bool { return fx.exec.callCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	fx.exec.mu.Lock()
	got := fx.exec.inputs[0].Content
	fx.exec.mu.Unlock()
	require.Equal(t, "查一下天气", got)
}

// listening 态重复单唤醒词：吞掉不建 Turn，停留聆听。
func TestSessionSingleWakeWordUtteranceSwallowed(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	fx.sess.Wake("kws")
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "小媛"}
	require.Never(t, func() bool { return fx.exec.callCount() > 0 }, 200*time.Millisecond, 20*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
}

// G1：dormant 委派终态 → 系统唤醒（无「我在」应答）→ 自足播报 → 可回待命。
func TestSessionDelegationTerminalWakesFromDormant(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	reader := &fakeDelegationStepReader{steps: []biz.Step{
		{ID: "st-1", SessionID: "ss-spirit", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted, Content: "报表已完成。"},
	}}
	fx.withDelegation(reg, reader)
	fx.sess.Start(StartParams{Mode: ModeCompanion})
	require.Equal(t, "dormant", fx.down.lastState())

	reg.Register("sess-1", "ss-spirit", "做个报表")
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-1", SessionID: "ss-spirit", UserMessage: "做个报表", Status: biz.TaskStatusRunning})
	fx.bus.ch <- biz.NewTaskCompletedEvent(biz.Task{ID: "task-1", SessionID: "ss-spirit", UserMessage: "做个报表", Status: biz.TaskStatusCompleted})

	// 系统唤醒进 listening 并播报委派结果
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "报表已完成。") }, 2*time.Second, 10*time.Millisecond)
	require.False(t, ttsWritesContain(fx, "我在")) // 系统唤醒无唤醒应答
}
