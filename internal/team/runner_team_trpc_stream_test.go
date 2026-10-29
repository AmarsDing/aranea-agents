package team

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

	v2 "aranea-agents/internal/agent/v2"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ────────────────────────────────────────────────────────────
// 2026-10-29 冤杀事故回归钉板（team 路径本源缺陷）
//
// 事故根因（output/live-44-functional-test/06-team-liveness-深度分析.md）：
// attempt 闭包内 `context.WithCancel` + `defer abortRun()` 在闭包返回时
// 【无条件】取消 lastRunCtx；RunTurnWithInvalidRetry（开关关闭 = 同步直通）
// 返回时 defer 已触发；收尾 `if lastRunCtx.Err() != nil` 恒为真 → 任何干净
// 收官的团队 run 都被误判为 "stream aborted before first byte or
// mid-stream liveness"（包 ErrFirstByteTimeout）落 failed。44 实测：两名
// 成员 step 双双 ok、事件通道正常关闭（consume_done: channel closed）后
// 504.8s 自然收官被冤杀。
//
// 修复契约：终态分支由「闭包内、defer 触发【前】捕获的取消态快照」驱动——
// 清理性 defer cancel 不构成证据；真实中止（首字节超时 / 中段活性耗尽 /
// 用户取消 / 定义超时 / token 预算）必须原样落入既有失败分类。
// ────────────────────────────────────────────────────────────

// streamStubRunner 是按脚本回放事件流的 trpcrunner.Runner 测试替身。
type streamStubRunner struct {
	runFn func(ctx context.Context) (<-chan *trpcevent.Event, error)
}

func (s streamStubRunner) Run(ctx context.Context, _, _ string, _ trpcmodel.Message, _ ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
	return s.runFn(ctx)
}

func (streamStubRunner) Close() error { return nil }

func teamStreamChunk(content string) *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object:    trpcmodel.ObjectTypeChatCompletionChunk,
		IsPartial: true,
		Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: content}}},
	}}
}

func teamStreamFinalChunk(content, finishReason string) *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletionChunk,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: content}, FinishReason: &finishReason}},
	}}
}

func teamStreamRunnerCompletion() *trpcevent.Event {
	return &trpcevent.Event{Response: &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeRunnerCompletion,
		Done:   true,
	}}
}

// cleanTeamStream 模拟事故会话的干净收官形态：首字节 → 正文 → finish_reason
// → RunnerCompletion → 通道正常关闭（consume_done: channel closed）。
func cleanTeamStream() <-chan *trpcevent.Event {
	ch := make(chan *trpcevent.Event, 3)
	ch <- teamStreamChunk("部分")
	ch <- teamStreamFinalChunk("两份交付物已完成", "stop")
	ch <- teamStreamRunnerCompletion()
	close(ch)
	return ch
}

// transitionSpy 记录 run 终态转换（finishRunErr → TransitionRunStatus）。
type transitionSpy struct {
	statuses []string
}

func (s *transitionSpy) TransitionRunStatus(_ context.Context, id, status string) (biz.TeamRunRecord, error) {
	s.statuses = append(s.statuses, status)
	return biz.TeamRunRecord{ID: id, TeamID: "team-1", SessionID: "sess-1", Status: status}, nil
}

type teamStreamTestBed struct {
	runner *Runner
	repo   *parityMemRepo
	spy    *transitionSpy
	reg    *rt.RunRegistry
}

func newTeamStreamTestBed() *teamStreamTestBed {
	repo := newParityMemRepo()
	spy := &transitionSpy{}
	reg := rt.NewRunRegistry()
	return &teamStreamTestBed{
		repo: repo,
		spy:  spy,
		reg:  reg,
		runner: &Runner{
			lg:              loggateway.NewNoop(),
			runWriter:       repo,
			runTransitioner: spy,
			cfg:             RunnerConfig{Runs: reg},
		},
	}
}

// streamAttemptInputFor 构造最小可用的流式执行输入。runCtx 由调用方按场景
// 提供（干净收官 = Background；真实取消 = 已取消 ctx；定义超时 = 已超期 ctx）。
func (b *teamStreamTestBed) streamAttemptInputFor(runCtx context.Context, run *biz.TeamRunRecord, stub streamStubRunner, turnStatus *string) teamStreamAttemptInput {
	return teamStreamAttemptInput{
		outerCtx:           context.Background(),
		runCtx:             runCtx,
		sess:               biz.Session{ID: "sess-1"},
		teamRow:            biz.Team{ID: "team-1"},
		def:                Definition{Mode: "sequential"},
		mode:               "sequential",
		run:                run,
		runner:             stub,
		uid:                "default_user",
		userTurnMsg:        trpcmodel.Message{Role: "user", Content: "请生成两份交付物"},
		projectMeta:        agent.ProjectMeta{SessionID: "sess-1", RequestID: "req-1", InvocationID: run.ID, RunID: run.ID, AgentID: "anchor"},
		v2Meta:             v2.ProjectMeta{},
		firstByteBudget:    5 * time.Second,
		stallMaxReconnects: 0,
		turnStatus:         turnStatus,
		t0:                 time.Now(),
	}
}

func newRunningTeamRun() *biz.TeamRunRecord {
	return &biz.TeamRunRecord{
		ID:        "run-1",
		TeamID:    "team-1",
		SessionID: "sess-1",
		Mode:      "sequential",
		Status:    biz.TeamRunStatusRunning,
		StartedAt: agent.RFC3339Now(),
	}
}

// TestExecuteTeamStreamAttempts_CleanCompletionReachesSuccessPath 钉死主修复：
// 干净收官（成员事件流正常结束、通道正常关闭）必须返回 nil 错误、不触发任何
// 失败终态化——即到达 runTeamTRPCFromInput 的 finalize 成功路径。
//
// 对修复前语义（闭包返回后读 defer 污染的 ctx.Err()）本用例必败：
// lastRunCtx 恒被清理性 cancel → 误判 "stream aborted…" 落 failed。
func TestExecuteTeamStreamAttempts_CleanCompletionReachesSuccessPath(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanTeamStream(), nil
	}}

	result, err := bed.runner.executeTeamStreamAttempts(bed.streamAttemptInputFor(context.Background(), run, stub, &turnStatus))
	if err != nil {
		t.Fatalf("干净收官不得被判失败: err=%v（2026-10-29 冤杀事故回归：defer 清理性 cancel 不构成流中止证据）", err)
	}
	if !result.HasContent {
		t.Fatal("干净脚本流必须产出内容")
	}
	if len(bed.spy.statuses) != 0 {
		t.Fatalf("干净收官不得触发失败终态化, TransitionRunStatus 被调用 %d 次: %v", len(bed.spy.statuses), bed.spy.statuses)
	}
	if turnStatus != biz.TeamMemberStepStatusOK {
		t.Fatalf("干净收官 turnStatus 必须保持 ok, got %s", turnStatus)
	}
	if run.Status != biz.TeamRunStatusRunning {
		t.Fatalf("run 状态不得被改动, got %s", run.Status)
	}
}

// TestExecuteTeamStreamAttempts_InfraAbortStillClassified 钉死「不掩盖真中止」
// （infra 分支）：run ctx 在流期间被真实取消且【无】用户取消原因登记时
// （首字节守卫 AbortOnStall / 中段活性中止等同构场景），必须仍落入既有
// aborted 分类：错误包装 ErrFirstByteTimeout、文案
// "stream aborted before first byte or mid-stream liveness"、run 落 failed。
func TestExecuteTeamStreamAttempts_InfraAbortStillClassified(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanTeamStream(), nil
	}}

	// 运行期间真实取消（无用户取消原因登记 → infra 中止语义）。
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := bed.runner.executeTeamStreamAttempts(bed.streamAttemptInputFor(runCtx, run, stub, &turnStatus))
	if err == nil {
		t.Fatal("真实中止必须仍判失败")
	}
	if !errors.Is(err, agent.ErrFirstByteTimeout) {
		t.Fatalf("infra 中止必须包装 ErrFirstByteTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "stream aborted before first byte or mid-stream liveness") {
		t.Fatalf("infra 中止文案丢失, got %v", err)
	}
	if len(bed.spy.statuses) != 1 || bed.spy.statuses[0] != biz.TeamRunStatusFailed {
		t.Fatalf("run 必须落 failed 一次, got %v", bed.spy.statuses)
	}
	if got := bed.repo.runs[run.ID].ErrorMessage; !strings.Contains(got, "stream aborted before first byte or mid-stream liveness") {
		t.Fatalf("failed 原因必须落库 aborted 文案, got %q", got)
	}
	if turnStatus != biz.TeamMemberStepStatusError {
		t.Fatalf("turnStatus 必须置 error, got %s", turnStatus)
	}
}

// TestExecuteTeamStreamAttempts_UserCancelKeepsBareCanceled 钉死「不掩盖真中
// 止」（用户取消分支）：取消原因登记为 user_cancel 时，错误必须保持裸
// context.Canceled（StartTeamTurn 的 isUserCancelledErr 据此映射
// cancelled），不得被包装成 ErrFirstByteTimeout 落 failed。
func TestExecuteTeamStreamAttempts_UserCancelKeepsBareCanceled(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanTeamStream(), nil
	}}

	// 用户取消：registry 登记 cancelled + user_cancel 原因（Cancel 时写入）。
	bed.reg.SetStatus("sess-1", run.ID, biz.SessionRunPhaseCancelled, "user_cancel")
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := bed.runner.executeTeamStreamAttempts(bed.streamAttemptInputFor(runCtx, run, stub, &turnStatus))
	if err == nil {
		t.Fatal("用户取消必须仍以取消终态返回")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("用户取消必须保持裸 context.Canceled, got %v", err)
	}
	if errors.Is(err, agent.ErrFirstByteTimeout) {
		t.Fatalf("用户取消不得被包装成 infra aborted, got %v", err)
	}
	if got := bed.repo.runs[run.ID].ErrorMessage; !strings.Contains(got, "cancel_reason=user_cancel") {
		t.Fatalf("失败原因必须携带 cancel_reason=user_cancel, got %q", got)
	}
}

// TestExecuteTeamStreamAttempts_DefinitionTimeoutStillFails 钉死定义超时分支：
// runCtx 超期（DeadlineExceeded）必须原样透传为失败，不得被误判为
// 用户取消或 infra aborted。
func TestExecuteTeamStreamAttempts_DefinitionTimeoutStillFails(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return cleanTeamStream(), nil
	}}
	in := bed.streamAttemptInputFor(context.Background(), run, stub, &turnStatus)
	in.def.TimeoutSeconds = 300

	runCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // 确保 deadline 已触发
	in.runCtx = runCtx

	_, err := bed.runner.executeTeamStreamAttempts(in)
	if err == nil {
		t.Fatal("定义超时必须判失败")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("定义超时必须透传 context.DeadlineExceeded, got %v", err)
	}
	if errors.Is(err, agent.ErrFirstByteTimeout) {
		t.Fatalf("定义超时不得被包装成 infra aborted, got %v", err)
	}
	if len(bed.spy.statuses) != 1 || bed.spy.statuses[0] != biz.TeamRunStatusFailed {
		t.Fatalf("run 必须落 failed 一次, got %v", bed.spy.statuses)
	}
}

// TestExecuteTeamStreamAttempts_FirstByteTimeoutStillFails 钉死首字节守卫路径：
// 静默流（首字节预算内零产出）必须仍经 streamErr 分支硬失败
// （ErrFirstByteTimeout）——该路径本就先于缺陷点返回，修复不得改变其行为。
func TestExecuteTeamStreamAttempts_FirstByteTimeoutStillFails(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return make(chan *trpcevent.Event), nil // 静默流：永不产出
	}}
	in := bed.streamAttemptInputFor(context.Background(), run, stub, &turnStatus)
	in.firstByteBudget = 20 * time.Millisecond
	in.stallMaxReconnects = 0 // 守卫总时限 = 1×预算 + slack，避免按重连池放大

	_, err := bed.runner.executeTeamStreamAttempts(in)
	if err == nil {
		t.Fatal("首字节超时必须硬失败")
	}
	if !errors.Is(err, agent.ErrFirstByteTimeout) {
		t.Fatalf("首字节超时必须包装 ErrFirstByteTimeout, got %v", err)
	}
	if len(bed.spy.statuses) != 1 || bed.spy.statuses[0] != biz.TeamRunStatusFailed {
		t.Fatalf("run 必须落 failed 一次, got %v", bed.spy.statuses)
	}
	if got := bed.repo.runs[run.ID].ErrorMessage; !strings.Contains(got, "first byte timeout") {
		t.Fatalf("失败原因必须为首字节超时文案, got %q", got)
	}
}

// TestExecuteTeamStreamAttempts_InvokeFailureRollsBack 钉死 invoke 硬失败
// 分支：RunTRPCUserTurnMsg 发起失败走 finishTeamRunWithError（含一次性
// 回滚），错误原样透传。
func TestExecuteTeamStreamAttempts_InvokeFailureRollsBack(t *testing.T) {
	t.Setenv("ARANEA_INVALID_RETRY_ENABLED", "false")
	bed := newTeamStreamTestBed()
	run := newRunningTeamRun()
	turnStatus := biz.TeamMemberStepStatusOK
	invokeErr := errors.New("runner: session locked")
	stub := streamStubRunner{runFn: func(context.Context) (<-chan *trpcevent.Event, error) {
		return nil, invokeErr
	}}
	in := bed.streamAttemptInputFor(context.Background(), run, stub, &turnStatus)
	rollbackCalled := false
	in.rollbackRunnerSession = func() { rollbackCalled = true }

	_, err := bed.runner.executeTeamStreamAttempts(in)
	if !errors.Is(err, invokeErr) {
		t.Fatalf("invoke 硬失败必须原样透传, got %v", err)
	}
	if !rollbackCalled {
		t.Fatal("invoke 硬失败必须触发一次性回滚")
	}
	if len(bed.spy.statuses) != 1 || bed.spy.statuses[0] != biz.TeamRunStatusFailed {
		t.Fatalf("run 必须落 failed 一次, got %v", bed.spy.statuses)
	}
}
