package provider

// finish_observed_test.go —— Q5 批 D3 wrapper 钉板（94 三件套）：
//  1. 逐帧透传不改帧（指针级 verbatim），快照事实原样保留——供应商原文（含
//     大写拼写/错误文本）零处理，判定全在 agent emit 闭包（防 wrapper 与
//     emit 两份判序分叉污染 fork 证据，D3 评审 major 条款）。
//  2. 流关闭钩子恒恰调一次——含 silent 净关的全零快照（「是否发射」由
//     emit 侧把关，wrapper 不做闸）；同帧 error 与 finish_reason 均采集。
//  3. 观测绝缘：钩子 panic 被 recover 且不炸 turn（帧消费者无感）+ Warn
//     留证；nil 钩子/nil inner 整体 pass-through（零接线零开销）。
//  4. 刻意不实现 IterModel（shrink_retry.go:80-84 先例）。

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// finishObsCapture 收集钩子收到的快照（钩子未触发时切片恒空）。
type finishObsCapture struct {
	mu  sync.Mutex
	obs []FinishObservation
}

func (c *finishObsCapture) hook(_ context.Context, o FinishObservation) {
	c.mu.Lock()
	c.obs = append(c.obs, o)
	c.mu.Unlock()
}

func (c *finishObsCapture) all() []FinishObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]FinishObservation, len(c.obs))
	copy(out, c.obs)
	return out
}

func finishReasonFrame(reason string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Done:    true,
		Choices: []trpcmodel.Choice{{FinishReason: &reason}},
	}
}

func finishObsErrFrame(msg, typ string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Done:  true,
		Error: &trpcmodel.ResponseError{Message: msg, Type: typ},
	}
}

// 正常 stop 流：帧指针级透传 + 快照恰一条（stop 事实原样上报，判定在 emit）。
func TestFinishObserved_ForwardsFramesVerbatimAndObservesStop(t *testing.T) {
	frames := []*trpcmodel.Response{
		contentFrame("he"), contentFrame("llo"), finishReasonFrame("stop"),
	}
	inner := &shrinkStubModel{name: "inner", frames: frames}
	cap := &finishObsCapture{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{
		Hook: cap.hook, Provider: "openai", Model: "gpt-x", Origin: "chat_turn",
	})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	got := drainResponses(ch)
	if len(got) != len(frames) {
		t.Fatalf("frames forwarded = %d, want %d", len(got), len(frames))
	}
	for i := range frames {
		if got[i] != frames[i] {
			t.Fatalf("frame %d not forwarded verbatim (wrapper must not mutate)", i)
		}
	}
	obs := cap.all()
	if len(obs) != 1 {
		t.Fatalf("hook calls = %d, want exactly 1", len(obs))
	}
	o := obs[0]
	if !o.SawFinishReason || o.FinishReason != "stop" {
		t.Errorf("obs finish = (%v, %q), want (true, stop)", o.SawFinishReason, o.FinishReason)
	}
	if o.HasError {
		t.Errorf("obs.HasError = true, want false")
	}
	if o.Provider != "openai" || o.Model != "gpt-x" || o.Origin != "chat_turn" {
		t.Errorf("obs identity = (%q, %q, %q), want (openai, gpt-x, chat_turn)", o.Provider, o.Model, o.Origin)
	}
	if o.SessionID != "" {
		t.Errorf("obs.SessionID = %q, want empty (session 由 RoundTripForSession 漏斗注入)", o.SessionID)
	}
}

// 供应商原文保留：大写拼写不折叠（CONTENT_FILTER 原样上报——大小写归一与
// 映射纪律在 emit 侧，wrapper 侧任何折叠都会毁灭 fork 判定证据）。
func TestFinishObserved_FinishReasonPreservedVendorLiteral(t *testing.T) {
	inner := &shrinkStubModel{name: "inner", frames: []*trpcmodel.Response{
		contentFrame("x"), finishReasonFrame("CONTENT_FILTER"),
	}}
	cap := &finishObsCapture{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{
		Hook: cap.hook, Provider: "gemini", Model: "g-2.5",
	})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	drainResponses(ch)
	obs := cap.all()
	if len(obs) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(obs))
	}
	if obs[0].FinishReason != "CONTENT_FILTER" {
		t.Errorf("FinishReason = %q, want vendor literal CONTENT_FILTER (no case folding at wrapper)", obs[0].FinishReason)
	}
}

// 错误帧：帧透传 + HasError/LastError/LastErrorType 事实采集；多错误帧
// last-wins（快照语义=流内最后一次观察）。
func TestFinishObserved_ErrorFrameObservedRawLastWins(t *testing.T) {
	first := finishObsErrFrame("stream reset by peer", "stream_error")
	last := finishObsErrFrame("Post \"https://api.x\": unexpected EOF", "stream_error")
	inner := &shrinkStubModel{name: "inner", frames: []*trpcmodel.Response{
		contentFrame("partial"), first, last,
	}}
	cap := &finishObsCapture{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{Hook: cap.hook})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	got := drainResponses(ch)
	if len(got) != 3 || got[1] != first || got[2] != last {
		t.Fatalf("error frames must forward verbatim, got %d frames", len(got))
	}
	obs := cap.all()
	if len(obs) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(obs))
	}
	o := obs[0]
	if !o.HasError {
		t.Fatal("obs.HasError = false, want true")
	}
	if !strings.Contains(o.LastError, "unexpected EOF") {
		t.Errorf("LastError = %q, want last error frame text", o.LastError)
	}
	if o.LastErrorType != "stream_error" {
		t.Errorf("LastErrorType = %q, want stream_error", o.LastErrorType)
	}
}

// 纯事实纪律（D3 评审 major）：overflow 标记文本与 finish=length 同流共存
// 时两者都原样进快照——wrapper 不得做「overflow 排除」「kind 判定」等任何
// 门控（判序唯一归口 = agent 分类器，emit 侧按共享判序拿排除语义）。
func TestFinishObserved_RawFactsOnlyNoWrapperSideGating(t *testing.T) {
	inner := &shrinkStubModel{name: "inner", frames: []*trpcmodel.Response{
		finishObsErrFrame("Error 400: this model's maximum context length is 8192 tokens; request too large", trpcmodel.ErrorTypeAPIError),
		finishReasonFrame("length"),
	}}
	cap := &finishObsCapture{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{Hook: cap.hook})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	drainResponses(ch)
	obs := cap.all()
	if len(obs) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(obs))
	}
	o := obs[0]
	if !o.HasError || !strings.Contains(o.LastError, "maximum context length") {
		t.Errorf("overflow error fact lost: HasError=%v LastError=%q", o.HasError, o.LastError)
	}
	if !o.SawFinishReason || o.FinishReason != "length" {
		t.Errorf("finish fact lost: (%v, %q)", o.SawFinishReason, o.FinishReason)
	}
}

// 零接线 pass-through：nil 钩子原样返回 inner；nil inner 原样返回 nil。
func TestFinishObserved_NilHookOrNilInnerPassThrough(t *testing.T) {
	inner := &shrinkStubModel{name: "inner"}
	if got := WrapModelWithFinishObserver(inner, FinishObserverOptions{}); got != trpcmodel.Model(inner) {
		t.Fatal("nil hook must pass inner through unchanged")
	}
	noop := func(context.Context, FinishObservation) {}
	if got := WrapModelWithFinishObserver(nil, FinishObserverOptions{Hook: noop}); got != nil {
		t.Fatal("nil inner must stay nil")
	}
}

// silent 净关（无 finish_reason、无错误帧）：钩子仍恒恰调一次、快照全零——
// 发射与否的判定单一归口在 emit 闭包（wrapper 若自判会让两处判序分叉）。
func TestFinishObserved_HookFiresExactlyOnceOnSilentCleanClose(t *testing.T) {
	inner := &shrinkStubModel{name: "inner", frames: []*trpcmodel.Response{
		contentFrame("hello"), contentFrame(" world"),
	}}
	cap := &finishObsCapture{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{Hook: cap.hook})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if got := drainResponses(ch); len(got) != 2 {
		t.Fatalf("frames = %d, want 2", len(got))
	}
	obs := cap.all()
	if len(obs) != 1 {
		t.Fatalf("hook calls = %d, want exactly 1 even for silent clean close", len(obs))
	}
	o := obs[0]
	if o.SawFinishReason || o.HasError {
		t.Errorf("silent close snapshot must be all-zero facts, got finish=(%v,%q) err=(%v,%q)",
			o.SawFinishReason, o.FinishReason, o.HasError, o.LastError)
	}
}

// 观测绝缘：钩子 panic 被 recover——帧消费者完整收到全部帧，turn 不被
// 炸断；Warn 留证（spill_failed 先例：观测链失败不回流主链）。
func TestFinishObserved_HookPanicRecoveredTurnUnbroken(t *testing.T) {
	frames := []*trpcmodel.Response{contentFrame("a"), finishReasonFrame("length")}
	inner := &shrinkStubModel{name: "inner", frames: frames}
	lg := &captureWarnLogger{}
	m := WrapModelWithFinishObserver(inner, FinishObserverOptions{
		Hook:   func(context.Context, FinishObservation) { panic("boom") },
		Logger: lg,
	})
	ch, err := m.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	got := drainResponses(ch)
	if len(got) != len(frames) {
		t.Fatalf("consumer must receive all frames despite hook panic, got %d want %d", len(got), len(frames))
	}
	if len(lg.warns) != 1 || !strings.Contains(lg.warns[0], "panic recovered") {
		t.Fatalf("panic must be Warn-evidenced, warns=%v", lg.warns)
	}
}

// D3-f 对抗评审 C3 修复批钉板：经生产入口 TRPCModelForProviderModel 的
// 观测层安装断言。旧的 8 钉全部直构 WrapModelWithFinishObserver——删掉
// trpc_llm.go 的 WrapModelWithFinishObserver 接线块（或
// finishObservedHookFromRT 取值）后编译与全部测试仍绿、生产观测静默全灭。
// 本钉经真生产构造链（stub 目录 + 离线最小 openai config，无 base_url →
// 不触 SSRF 校验/网络）驱动，断言返回模型最外层即 finishObservedModel 且
// provider/model 事实取自 cfg；改动方向固定：
//   - 删挂链块 / hook 取值 → 类型断言红；
//   - cfg 事实位漂移（Provider/Model 传错源）→ 型内字段断言红。
func TestTRPCModelForProviderModel_InstallsFinishObserver(t *testing.T) {
	ctx := context.Background()
	cat := stubTeamModelCatalog{pm: biz.ProviderModel{
		Model:      "m-api",
		ConfigJSON: `{"provider_type":"openai","api_key":"k"}`,
	}}
	cap := &finishObsCapture{}

	withHook, err := TRPCModelForProviderModel(ctx, cat, &RoundTrip{OnAbnormalFinish: cap.hook}, "openai", "m-api", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("build with hook: %v", err)
	}
	obsWrap, ok := withHook.(*finishObservedModel)
	if !ok {
		t.Fatalf("outermost wrap must be *finishObservedModel when rt carries hook, got %T（挂链块被删/挪序?）", withHook)
	}
	if obsWrap.provider != "openai" || obsWrap.model != "m-api" {
		t.Errorf("observed facts must come from cfg: provider=%q model=%q", obsWrap.provider, obsWrap.model)
	}
	if obsWrap.hook == nil {
		t.Error("hook must be installed from rt.OnAbnormalFinish（finishObservedHookFromRT 取值缺失?）")
	}

	// 未注钩子：观测层按设计 pass-through——最外层不得是本装饰器。
	noHook, err := TRPCModelForProviderModel(ctx, cat, &RoundTrip{}, "openai", "m-api", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("build without hook: %v", err)
	}
	if _, isObs := noHook.(*finishObservedModel); isObs {
		t.Error("nil hook must keep the observer wrap out of the chain（pass-through 契约）")
	}
}
