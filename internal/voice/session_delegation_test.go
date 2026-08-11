package voice

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	"github.com/stretchr/testify/require"
)

// ---- M74 V9-T3：eventLoop 三路分流 + task 绑定 + 委派播报 ----

// fakeDelegationStepReader 记录读取调用并按脚本返回精灵会话 steps。
type fakeDelegationStepReader struct {
	mu     sync.Mutex
	steps  []biz.Step
	err    error
	gotSID string
	calls  int
}

func (f *fakeDelegationStepReader) ListStepsBySessionID(_ context.Context, sid string) ([]biz.Step, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.gotSID = sid
	return f.steps, f.err
}

func (f *fakeDelegationStepReader) lastSID() (string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotSID, f.calls
}

// withDelegation 将委派登记表 + 终稿读取端口接入 fixture（须在 Start 前调用）。
func (fx *sessionFixture) withDelegation(reg *DelegationRegistry, steps DelegationStepReader) {
	fx.sess.deps.Delegation = reg
	fx.sess.deps.DelegationSteps = steps
}

func ttsWritesContain(fx *sessionFixture, sub string) bool {
	return strings.Contains(strings.Join(fx.ttsProv.allWrites(), ""), sub)
}

// 第三路：外来会话事件一律丢弃（防串话）——精灵后台流式 delta 不得进本会话 TTS。
func TestSessionDelegationForeignEventsDiscarded(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	reader := &fakeDelegationStepReader{}
	fx.withDelegation(reg, reader)
	fx.sess.Start(StartParams{})

	// 本会话 turn 进行中（thinking）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	// 外来会话（精灵后台执行）的流式 delta / turn 终态：全部丢弃
	fx.bus.ch <- biz.NewStepStreamingEvent("ss-spirit", "task-x", "step-x", "content", "精灵的后台输出不应串话。")
	fx.bus.ch <- biz.NewTurnCompletedEvent(biz.Turn{SpiritSessionID: "ss-spirit"})
	require.Never(t, func() bool { return ttsWritesContain(fx, "串话") }, 200*time.Millisecond, 20*time.Millisecond)

	// 本会话 delta 照常处理（第一路不回归）
	fx.bus.ch <- biz.NewStepStreamingEvent("sess-1", "task-1", "step-1", "content", "本会话回复。")
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "本会话回复") }, 2*time.Second, 10*time.Millisecond)
}

// 委派全链路：注册 → TaskCreated 内容匹配绑定 → TaskCompleted 取终稿全文播报。
func TestSessionDelegationBindAndBroadcast(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	reader := &fakeDelegationStepReader{steps: []biz.Step{
		{ID: "st-1", SessionID: "ss-spirit", Kind: biz.StepKindThinking, Status: biz.StepStatusCompleted, Content: "思考"},
		{ID: "st-2", SessionID: "ss-spirit", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted, Content: "表格已做好，共三列。"},
	}}
	fx.withDelegation(reg, reader)
	fx.sess.Start(StartParams{})

	// 工具侧先注册（先注册后提交，无漏绑窗口）
	reg.Register("sess-1", "ss-spirit", "做个表")

	// 精灵后台建 task：内容精确匹配绑定 taskID
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-9", SessionID: "ss-spirit", UserMessage: "做个表", Status: biz.TaskStatusRunning})
	// task 终态 → 取终稿播报
	fx.bus.ch <- biz.NewTaskCompletedEvent(biz.Task{ID: "task-9", SessionID: "ss-spirit", UserMessage: "做个表", Status: biz.TaskStatusCompleted})

	require.Eventually(t, func() bool { return ttsWritesContain(fx, "表格已做好，共三列。") }, 2*time.Second, 10*time.Millisecond)
	require.True(t, ttsWritesContain(fx, "精灵助手来回复了"))
	// 终稿读取目标 = 精灵会话
	require.Eventually(t, func() bool {
		sid, calls := reader.lastSID()
		return calls == 1 && sid == "ss-spirit"
	}, 2*time.Second, 10*time.Millisecond)
	// 自足 flush：tts.end 下行，状态停留 listening（listening 无 FSM 出口事件）
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "tts.end") >= 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
	// 条目已消费移除
	_, ok := reg.OwnerOf("ss-spirit")
	require.False(t, ok)
}

// 正忙排队（§15.3）：委派终态到达时 voice 在 thinking → 入 FIFO；本 turn
// 结束回 listening 后串联排空播报。
func TestSessionDelegationBroadcastQueuesWhileBusy(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	reader := &fakeDelegationStepReader{steps: []biz.Step{
		{ID: "st-1", SessionID: "ss-spirit", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted, Content: "委派结果全文。"},
	}}
	fx.withDelegation(reg, reader)
	fx.sess.Start(StartParams{})

	// 本会话 turn 进行中（无文本 turn：thinking 停留）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你在吗"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	// 委派终态到达：正忙 → 排队，不得立即播报
	reg.Register("sess-1", "ss-spirit", "查资料")
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-7", SessionID: "ss-spirit", UserMessage: "查资料"})
	fx.bus.ch <- biz.NewTaskCompletedEvent(biz.Task{ID: "task-7", SessionID: "ss-spirit", Status: biz.TaskStatusCompleted})
	require.Never(t, func() bool { return ttsWritesContain(fx, "委派结果全文") }, 200*time.Millisecond, 20*time.Millisecond)

	// 本 turn 结束（无文本）→ 回 listening → 排空播报
	fx.bus.ch <- biz.NewTurnCompletedEvent(biz.Turn{SpiritSessionID: "sess-1"})
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "委派结果全文") }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
}

// 提交同步失败（永无 TaskCreated）：watcher 带外通知口播失败原因（R12）。
func TestSessionDelegationSubmitFailedNoticeBroadcast(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	fx.withDelegation(reg, &fakeDelegationStepReader{})
	fx.sess.Start(StartParams{})

	regID := reg.Register("sess-1", "ss-spirit", "做个表")
	reg.MarkSubmitFailed(regID, "交给精灵助手的任务提交失败了，请在聊天窗口里直接告诉我重试")

	require.Eventually(t, func() bool { return ttsWritesContain(fx, "提交失败") }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, "listening", fx.down.lastState())
}

// 委派任务失败终态：口播失败简报，防用户空等（§15.6）。
func TestSessionDelegationFailedTaskBroadcast(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	fx.withDelegation(reg, &fakeDelegationStepReader{})
	fx.sess.Start(StartParams{})

	reg.Register("sess-1", "ss-spirit", "做个表")
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-8", SessionID: "ss-spirit", UserMessage: "做个表"})
	fx.bus.ch <- biz.NewTaskFailedEvent(biz.Task{ID: "task-8", SessionID: "ss-spirit", Status: biz.TaskStatusFailed})

	require.Eventually(t, func() bool { return ttsWritesContain(fx, "未能完成") }, 2*time.Second, 10*time.Millisecond)
	_, ok := reg.OwnerOf("ss-spirit")
	require.False(t, ok)
}

// 终稿读取失败降级（K3）：播简报而非全文。
func TestSessionDelegationReplyReadFailureFallsBackToBrief(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	fx.withDelegation(reg, &fakeDelegationStepReader{err: errors.New("db down")})
	fx.sess.Start(StartParams{})

	reg.Register("sess-1", "ss-spirit", "做个表")
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-6", SessionID: "ss-spirit", UserMessage: "做个表"})
	fx.bus.ch <- biz.NewTaskCompletedEvent(biz.Task{ID: "task-6", SessionID: "ss-spirit", Status: biz.TaskStatusCompleted})

	require.Eventually(t, func() bool { return ttsWritesContain(fx, "任务已完成") }, 2*time.Second, 10*time.Millisecond)
}

// 会话拆除即清委派条目与 watcher（进程内委派跟随会话生命周期）。
func TestSessionDelegationClearedOnClose(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	fx.withDelegation(reg, &fakeDelegationStepReader{})
	fx.sess.Start(StartParams{})
	reg.Register("sess-1", "ss-spirit", "做个表")

	fx.sess.Close()
	_, ok := reg.OwnerOf("ss-spirit")
	require.False(t, ok, "Close 必须清委派条目")
	// watcher 已清：MarkSubmitFailed 不再回调（无 panic 即通过）
	reg.MarkSubmitFailed(1, "x")
}

// 委派自足播报在途时用户开口（ASR final 开新 turn）：残余播报必须按
// barge-in 语义取消（M74 V9 评审竞态修复）——否则其 flush 哨兵 OnDrained
// 会把 thinking 经 EvTTSEnd（无文本 Turn 合法出口）提前拍回 listening，
// 新 turn delta 全丢（feedDelta active=false）+ tts.end 缺失。
func TestSessionDelegationBroadcastCancelledByNewTurn(t *testing.T) {
	fx := newSessionFixture(t)
	reg := NewDelegationRegistry(nil)
	fx.withDelegation(reg, &fakeDelegationStepReader{})
	// 委派播报合成挂起（模拟播报在途）：吐一帧后阻塞
	release := make(chan struct{})
	fx.ttsProv.script = func() *scriptedTTSSession {
		return &scriptedTTSSession{
			chunks:  []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1}}},
			blockCh: release,
			closed:  make(chan struct{}),
		}
	}
	fx.sess.Start(StartParams{})

	reg.Register("sess-1", "ss-spirit", "做个表")
	fx.bus.ch <- biz.NewTaskCreatedEvent(biz.Task{ID: "task-5", SessionID: "ss-spirit", UserMessage: "做个表"})
	fx.bus.ch <- biz.NewTaskCompletedEvent(biz.Task{ID: "task-5", SessionID: "ss-spirit", Status: biz.TaskStatusCompleted})
	// 委派播报开始：第一个 TTS session 已开且合成挂起
	require.Eventually(t, func() bool {
		fx.ttsProv.mu.Lock()
		defer fx.ttsProv.mu.Unlock()
		return len(fx.ttsProv.sessions) == 1
	}, 2*time.Second, 10*time.Millisecond)
	fx.ttsProv.mu.Lock()
	delegSess := fx.ttsProv.sessions[0]
	fx.ttsProv.mu.Unlock()

	// 用户开口 → 新 turn：在途委派播报必须被取消（session Close）
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "新问题"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)
	select {
	case <-delegSess.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight delegation broadcast must be cancelled on new turn (barge-in)")
	}
	close(release) // 解除残余阻塞（防御；Cancelled session 已被 Close）

	// 新 turn delta 正常合成（新 TTS session），不丢
	fx.bus.ch <- biz.NewStepStreamingEvent("sess-1", "task-1", "step-1", "content", "新问题的回复。")
	require.Eventually(t, func() bool { return ttsWritesContain(fx, "新问题的回复") }, 2*time.Second, 10*time.Millisecond)
	// TurnCompleted → tts.end 必达 + 状态回 listening
	fx.bus.ch <- biz.NewTurnCompletedEvent(biz.Turn{SpiritSessionID: "sess-1"})
	require.Eventually(t, func() bool {
		return indexOf(fx.down.typesOf(), "tts.end") >= 0 && fx.down.lastState() == "listening"
	}, 2*time.Second, 10*time.Millisecond)
}
