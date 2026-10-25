package provider

import (
	"context"
	"fmt"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// finish_observed.go —— Q5 批 D3 业务 wrap：finish_reason/EOF 结构化透传
// （评审 `docs/reports/2026-10-22-review-framework-finish-reason-eof.md` §3
// 选项 B；94-llm-abnormal-finish-observer 三件套）。各 provider adapter 早已
// 携带 finish_reason / error 帧，但业务链路不消费——截断（length）与正常结束
// （stop）在编排层无法区分。本装饰器在 channel 路径上纯观测透传，把一份纯
// 事实快照交给注入的钩子（判定/发射在 agent 侧 emit 闭包，与本包零业务判断
// ——kind 判定共享 agent 包分类器判序，防双份字符串镜像污染 fork 证据，
// D3 评审 major 纠正）。零行为变更：只转发帧不改任何内容，钩子 nil 时整体
// pass-through。
//
// 挂链点：trpc_llm.go TRPCModelForProviderModel 的 LivenessGuard **之外**——
// 单个逻辑 GenerateContent 观测一次（liveness 内部重连对用户不可见，EOF/
// finish 以 reconnect 后的终态为准）。

// FinishObservation 是 wrapper 对一次逻辑 GenerateContent 的纯观测快照：
// 全部字段为流内逐事件采集的事实，无派生语义（同 agent.StreamInvalidObs 的
// 采集纪律）。判定（是否异常、kind 标签、是否去重发射）一律由钩子消费侧
// （agent finish_observed_emit.go）负责。
type FinishObservation struct {
	// Provider / Model：模型目录解析出的供应商与模型名（日志/Extra 上下文）。
	Provider string
	Model    string
	// Origin：调用面标签（chat_turn/team_turn/background…）；空时由 emit
	// 侧按 ctx/session 推导。多副本折算与验收分组用。
	Origin string
	// SessionID：RoundTripForSession 闭包绑定的会话 id；空=后台/图路径，
	// emit 侧按 "unknown" 桶回落（防全进程非同源事件坍缩）。
	SessionID string
	// SawFinishReason / FinishReason：流内是否观察到 finish_reason 及其最后
	// 取值（保留供应商原文，大小写归一在判定侧）。
	SawFinishReason bool
	FinishReason    string
	// HasError / LastError / LastErrorType：流内最后一个 ResponseError 事件。
	HasError      bool
	LastError     string
	LastErrorType string
}

// FinishObservedHook 是观测钩子的最小形态（RoundTrip.OnAbnormalFinish 同型）。
// 由上层（agent emit 闭包）注入；消费侧自行保证幂等与异常不回流。
type FinishObservedHook func(ctx context.Context, obs FinishObservation)

// FinishObserverHooks 是 wire 装配期注入的钩子束（同 toolspill.Hooks 先例）。
// 同一份实例经 TurnDeps/graph 链共享：进程内 deduper 随实例去重，副本间
// 线性放大按 origin 折算（94.design.md §3）。
type FinishObserverHooks struct {
	OnAbnormalFinish FinishObservedHook
}

// FinishObserverOptions 是 WrapModelWithFinishObserver 的参数包。
type FinishObserverOptions struct {
	Hook     FinishObservedHook
	Provider string
	Model    string
	Origin   string
	Logger   loggateway.Logger // 构造注入；nil 时按 Noop 处理（包内构造约定）
}

// finishObservedModel 是包级装饰器实现。
type finishObservedModel struct {
	inner    trpcmodel.Model
	hook     FinishObservedHook
	provider string
	model    string
	origin   string
	lg       loggateway.Logger
}

// WrapModelWithFinishObserver 在 inner 上叠加 finish_reason/EOF 纯观测。
// inner 为 nil 或 Hook 为 nil 时原样返回 inner——不接线即零开销零行为变更。
//
// 注意：包装结果刻意不实现 IterModel（shrink_retry.go:80-84 先例）——channel
// 路径需要逐帧转发采集快照，IterModel 的 caller-goroutine pull 语义无法在
// 不缓冲/不泄漏的前提下同样观测首帧后的流；llmflow 对非 IterModel 自动退回
// GenerateContent 路径，开销与 shrink/metrics 的非 iter 分支等同。
func WrapModelWithFinishObserver(inner trpcmodel.Model, opts FinishObserverOptions) trpcmodel.Model {
	if inner == nil || opts.Hook == nil {
		return inner
	}
	lg := opts.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &finishObservedModel{
		inner:    inner,
		hook:     opts.Hook,
		provider: opts.Provider,
		model:    opts.Model,
		origin:   opts.Origin,
		lg:       lg,
	}
}

// Info 透传 inner 的 Info（观测层不申报为模型恒量变化）。
func (m *finishObservedModel) Info() trpcmodel.Info { return m.inner.Info() }

// GenerateContent 逐帧转发 inner 的流并采集快照；流关闭后对非空快照恰调
// 一次钩子。判定语义全在钩子侧——本函数只做事实采集，绝不改帧。
func (m *finishObservedModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch, err := m.inner.GenerateContent(ctx, request)
	if err != nil {
		// 同步构造失败（未开流）：不属于「流内异常收官」观测面，原样冒泡。
		return nil, err
	}
	out := make(chan *trpcmodel.Response, 16)
	go func() {
		defer close(out)
		obs := FinishObservation{Provider: m.provider, Model: m.model, Origin: m.origin}
		for resp := range ch {
			if resp != nil {
				if resp.Error != nil {
					obs.HasError = true
					obs.LastError = resp.Error.Message
					obs.LastErrorType = resp.Error.Type
				}
				for i := range resp.Choices {
					if fr := resp.Choices[i].FinishReason; fr != nil && *fr != "" {
						obs.SawFinishReason = true
						obs.FinishReason = *fr
					}
				}
			}
			select {
			case out <- resp:
			case <-ctx.Done():
				return
			}
		}
		// 流关闭：快照恒交给钩子（含正常 stop——判定与「零新增日志」纪律在
		// emit 闭包把关，防 wrapper 与 emit 两份判序分叉污染证据）。钩子
		// panic 不回流：观测链失败不炸 turn（spill_failed 先例）。
		defer func() {
			if r := recover(); r != nil {
				m.lg.Warn("llm finish observer hook panic recovered",
					loggateway.StepID("provider.finish_observed"),
					loggateway.Str("provider", m.provider),
					loggateway.Str("model", m.model),
					loggateway.Str("panic", fmt.Sprint(r)))
			}
		}()
		m.hook(ctx, obs)
	}()
	return out, nil
}
