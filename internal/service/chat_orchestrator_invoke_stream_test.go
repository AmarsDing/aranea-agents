package service

import (
	"context"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/tools/security"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ────────────────────────────────────────────────────────────
// 2026-10-29 defer 污染冤杀事故回归钉板（chat 路径次生缺陷）
//
// 事故根因：invokeTurnLLMAndStream 的 attempt 闭包内
// `context.WithCancel` + `defer abortRun()` 在闭包返回时【无条件】取消
// lastRunCtx；闭包返回后的 `errors.Is(lastRunCtx.Err(), context.Canceled)`
// 恒为真 → 干净收官的 turn 被误判 cancelled，session_runs.phase 恒落
// 'cancelled'（六轮遗留 P3 怪癖根因，见
// output/live-44-functional-test/06-team-liveness-深度分析.md §2.3）。
//
// 修复契约：终态分支必须由「闭包内、defer 触发【前】的取消态快照」驱动——
// 只有运行期间的真实取消（用户取消 / 显式 abort）才置 cancelled；
// 清理性 defer cancel 不构成证据。
// ────────────────────────────────────────────────────────────

// invokeStreamStubRunner 是按脚本回放事件流的 trpcrunner.Runner 测试替身。
type invokeStreamStubRunner struct {
	runFn func(ctx context.Context) (<-chan *trpcevent.Event, error)
}

func (s invokeStreamStubRunner) Run(ctx context.Context, _, _ string, _ trpcmodel.Message, _ ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	return s.runFn(ctx)
}

func (invokeStreamStubRunner) Close() error { return nil }

func invokeStreamChunk(content string) *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object:    trpcmodel.ObjectTypeChatCompletionChunk,
		IsPartial: true,
		Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: content}}},
	}}
}

func invokeStreamFinalChunk(content, finishReason string) *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletionChunk,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: content}, FinishReason: &finishReason}},
	}}
}

func invokeStreamRunnerCompletion() *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeRunnerCompletion,
		Done:   true,
	}}
}

// cleanInvokeStream 模拟一次干净收官的事件流：首字节 → 正文 → finish_reason
// → RunnerCompletion → 通道正常关闭（与 44 事故会话的 consume_done:
// channel closed 形态一致）。
func cleanInvokeStream() <-chan *trpcevent.Event {
	ch := make(chan *trpcevent.Event, 3)
	ch <- invokeStreamChunk("你")
	ch <- invokeStreamFinalChunk("你好，有什么可以帮你？", "stop")
	ch <- invokeStreamRunnerCompletion()
	close(ch)
	return ch
}

func newInvokeStreamTestOrch() *ChatOrchestrator {
	return &ChatOrchestrator{
		core:             chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
		turnLC:           newNoopChatTurnLifecycle(),
		runMgr:           newNoopChatRunManager(),
		infraDeps:        ChatInfraDeps{LG: loggateway.NewNoop()},
		cmdSafetyChecker: security.NewCommandSafetyPermissionChecker(loggateway.NewNoop()),
	}
}

// TestInvokeTurnLLMAndStream_CleanTurnNotMarkedCancelled 钉死主修复：
// 干净收官（通道正常关闭、有内容、无错误）的 turn 不得因 attempt 闭包
// defer 的清理性 cancel 被误判 cancelled。
//
// 对修复前代码本用例必败（lastRunCtx.Err() 被 defer 污染恒为 Canceled →
// execResult.cancelled=true）。
func TestInvokeTurnLLMAndStream_CleanTurnNotMarkedCancelled(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	orch := newInvokeStreamTestOrch()
	runner := invokeStreamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanInvokeStream(), nil
	}}
	emitter := event.NewTraceEmitter(nil, event.TraceContext{SessionID: "sess-1", RunID: "run-1"}, loggateway.NewNoop())

	execResult, err := orch.invokeTurnLLMAndStream(
		context.Background(),
		biz.Session{ID: "sess-1"},
		biz.TurnInput{SessionID: "sess-1", Content: "你好"},
		biz.Agent{ID: "agent-1"},
		turnAdmissionResult{runID: "run-1", dialogMode: "default"},
		emitter,
		nil, // traceBridge：nil-safe（turntrace.Bridge.StartChild 对 nil 接收者直通）
		chatagent.TRPCBuilderDeps{},
		runner,
		biz.ChatMessage{ID: "msg-1", SessionID: "sess-1"},
		true,
		"{}",
		nil,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("clean turn must succeed, got err=%v", err)
	}
	if !execResult.result.HasContent {
		t.Fatal("clean scripted stream must produce content")
	}
	if execResult.cancelled {
		t.Fatal("干净收官不得标记 cancelled——attempt 闭包 defer 的清理性 cancel 不构成流中止证据（2026-10-29 冤杀事故回归）")
	}
}

// TestInvokeTurnLLMAndStream_RealCancelStillMarkedCancelled 钉死「不掩盖真
// 中止」：run ctx 在流期间被真实取消（用户取消语义）时，消费端观测到取消并
// 排干收官，execResult.cancelled 必须仍为 true——修复只剔除 defer 污染，
// 不得屏蔽真实取消传播（turns_v2.status=cancelled 与 session_runs.phase
// 对齐依赖此信号）。
func TestInvokeTurnLLMAndStream_RealCancelStillMarkedCancelled(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	orch := newInvokeStreamTestOrch()
	runner := invokeStreamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanInvokeStream(), nil
	}}
	emitter := event.NewTraceEmitter(nil, event.TraceContext{SessionID: "sess-1", RunID: "run-1"}, loggateway.NewNoop())

	// 用户取消在 turn 开始前已生效（等价于 cancelActiveRun 已取消 run ctx）：
	// 消费端在首个事件即观测到 turnCtx 取消 → canceled=true → 排干收官。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	execResult, err := orch.invokeTurnLLMAndStream(
		ctx,
		biz.Session{ID: "sess-1"},
		biz.TurnInput{SessionID: "sess-1", Content: "你好"},
		biz.Agent{ID: "agent-1"},
		turnAdmissionResult{runID: "run-1", dialogMode: "default"},
		emitter,
		nil,
		chatagent.TRPCBuilderDeps{},
		runner,
		biz.ChatMessage{ID: "msg-1", SessionID: "sess-1"},
		true,
		"{}",
		nil,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("cancelled turn consume must not hard-error, got err=%v", err)
	}
	if !execResult.cancelled {
		t.Fatal("真实取消必须仍标记 cancelled——修复不得屏蔽运行期间的真实取消传播")
	}
}
