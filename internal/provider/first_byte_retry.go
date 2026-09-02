package provider

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// livenessGuardSlack 是消费端首字节守卫在「重试预算×尝试数 + 退避总和」
// 之外附加的宽限：覆盖 attempt 取消/排空与重建连的调度抖动，避免守卫与
// 最后一次尝试的超时同时开火导致误杀。
const livenessGuardSlack = 2 * time.Second

// 重连退避：1s/2s/4s/8s/16s，封顶 30s（与传输层 retry 退避同口径）。
const (
	stallBackoffBase = 1 * time.Second
	stallBackoffCap  = 30 * time.Second
)

// stallBackoff 返回第 n 次重连（1 基）前的退避时长。
func stallBackoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	d := stallBackoffBase << (n - 1)
	if d > stallBackoffCap {
		d = stallBackoffCap
	}
	return d
}

// stallBackoffSum 返回 maxReconnects 次重连的退避总和（守卫预算重算用）。
func stallBackoffSum(maxReconnects int) time.Duration {
	var total time.Duration
	for i := 1; i <= maxReconnects; i++ {
		total += stallBackoff(i)
	}
	return total
}

// firstByteRetryBudgetKey 携带本轮每次模型调用允许的首字节静默预算。
// 编排层在解析出 firstByteTimeout 后注入 runCtx；模型调用链（framework
// flow → Model.GenerateContent/GenerateContentIter）继承同一 ctx，
// 装饰器据此在供应商静默超时时自动重连（P6-N3，S09 t32 实证：
// deepseek 首字节 stall 30s 直接打死整轮且无自动恢复）。
type firstByteRetryBudgetKey struct{}

// stallBudgetKey 携带首字节后的活性预算：事件间隔静默上限与重连次数
// （2026-09-01 活性守卫治理）。
type stallBudgetKey struct{}

type stallPolicy struct {
	timeout       time.Duration
	maxReconnects int
}

// WithFirstByteRetryBudget attaches the per-call first-byte silence budget to
// ctx. d<=0 is a no-op (decorator passes through without retry).
func WithFirstByteRetryBudget(ctx context.Context, d time.Duration) context.Context {
	if ctx == nil || d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, firstByteRetryBudgetKey{}, d)
}

// firstByteRetryBudgetFromContext reads the budget; 0 means retry disabled.
func firstByteRetryBudgetFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	d, _ := ctx.Value(firstByteRetryBudgetKey{}).(time.Duration)
	return d
}

// WithStallBudget attaches the post-first-byte liveness policy to ctx:
// stallTimeout 是相邻事件间允许的最大静默；maxReconnects 是单次
// GenerateContent 允许的最大重连次数（总尝试 = 1 + maxReconnects）。
// 字段 <=0 时落回包默认值（DefaultStallTimeout / DefaultStallMaxReconnects）。
func WithStallBudget(ctx context.Context, stallTimeout time.Duration, maxReconnects int) context.Context {
	if ctx == nil {
		return ctx
	}
	if stallTimeout <= 0 {
		stallTimeout = DefaultStallTimeout
	}
	if maxReconnects <= 0 {
		maxReconnects = DefaultStallMaxReconnects
	}
	return context.WithValue(ctx, stallBudgetKey{}, stallPolicy{timeout: stallTimeout, maxReconnects: maxReconnects})
}

// stallPolicyFromContext reads the injected policy; absent ctx value falls
// back to package defaults — the stall guard is always armed so long-stream
// calls are protected even on paths that forget to inject (the guard only
// fires on true silence; healthy streams are unaffected).
func stallPolicyFromContext(ctx context.Context) stallPolicy {
	p := stallPolicy{timeout: DefaultStallTimeout, maxReconnects: DefaultStallMaxReconnects}
	if ctx == nil {
		return p
	}
	if v, ok := ctx.Value(stallBudgetKey{}).(stallPolicy); ok {
		if v.timeout > 0 {
			p.timeout = v.timeout
		}
		if v.maxReconnects > 0 {
			p.maxReconnects = v.maxReconnects
		}
	}
	return p
}

// FirstByteGuardWithRetry 返回消费端首字节守卫在与模型级活性重连并存时应
// 使用的总时限：每次重连都重新经历一个首字节窗口，故守卫必须等满
// (1+maxReconnects) 个首字节预算 + 全部重连退避后才可判死，否则会在重连
// 中途取消 runCtx 把恢复中的调用误杀（两机制共享同一 runCtx）。
func FirstByteGuardWithRetry(budget time.Duration, maxReconnects int) time.Duration {
	if budget <= 0 {
		return budget
	}
	if maxReconnects < 0 {
		maxReconnects = 0
	}
	return time.Duration(1+maxReconnects)*budget + stallBackoffSum(maxReconnects) + livenessGuardSlack
}

// WrapModelWithLivenessGuard wraps inner with a full-stream liveness guard
// （2026-09-01 治理由「首字节单发重试」升级）：
//
//   - 首事件前：首字节预算（ctx 注入，无注入则该维度不监控——保持旧语义）。
//   - 首事件后：相邻事件间隔 stall 预算（ctx 注入 > 包默认 90s）。
//     任何事件到达即重置计时器——有响应就不超时，流可合法跑任意久。
//   - 计时器触发 / 中段 stream_error：取消本 attempt（断开 HTTP）→ 指数
//     退避 → 重发同一 Request，最多重连 maxReconnects 次（默认 5）。
//   - 重连穷尽：流尾产出终态错误响应（ErrorTypeStreamError +
//     ObjectTypeError），走正常失败链，不再重试。
//   - 语义错误（非 stream_error）直接透传，不触发重连。
//   - 错误响应归一化：转发时 Error != nil 且 Object 为空则置
//     ObjectTypeError（框架 isTerminalAgentErrorEvent 的 Object 门，
//     团队 WithPropagateChildAgentErrors 生效的必要条件）。
//
// 为什么安全：工具调用在完整响应组装后才执行，stall 发生时该次响应的
// 工具尚未进入执行阶段，半成品零副作用；重连 = 重新生成，最终消息采用
// 替换语义（openai accumulator 每调用独立），落库 = 成功 attempt 的完整
// 聚合。代价：事件流可见重连前的半截内容（可观测性噪音，不改结果）。
// 消费端首字节守卫总时限已按 FirstByteGuardWithRetry 重算，不会误杀重连。
func WrapModelWithLivenessGuard(inner trpcmodel.Model, providerType, modelName string, lg loggateway.Logger) trpcmodel.Model {
	if inner == nil {
		return nil
	}
	base := &livenessGuardModel{
		inner:    inner,
		provider: providerType,
		model:    modelName,
		lg:       lg,
	}
	if _, ok := inner.(trpcmodel.IterModel); ok {
		return &livenessGuardIterModel{livenessGuardModel: base}
	}
	return base
}

type livenessGuardModel struct {
	inner    trpcmodel.Model
	provider string
	model    string
	lg       loggateway.Logger
}

type livenessGuardIterModel struct {
	*livenessGuardModel
}

func (m *livenessGuardModel) Info() trpcmodel.Info {
	return m.inner.Info()
}

func (m *livenessGuardModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return m.generateWithGuard(ctx, request)
}

func (m *livenessGuardIterModel) GenerateContentIter(ctx context.Context, request *trpcmodel.Request) (trpcmodel.Seq[*trpcmodel.Response], error) {
	ch, err := m.generateWithGuard(ctx, request)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, ErrModelNilChannel
	}
	return func(yield func(*trpcmodel.Response) bool) {
		for resp := range ch {
			if !yield(resp) {
				// 消费者提前停止（框架出错/取消）：排空 ch，避免
				// generateWithGuard 的转发 goroutine 永远阻塞在 out<-。
				safego.Go(ctx, "liveness-guard-drain-abandoned", func() { drainResponseChannel(ch) })
				return
			}
		}
	}, nil
}

// callOnce performs one attempt, normalizing both model flavors into a
// channel so the deadline-select logic lives in exactly one place. When the
// inner model only offers the iterator form, a pump goroutine pulls the seq
// (the seq executes the HTTP request lazily on first pull, so the pump must
// start immediately — not after the deadline fires).
func (m *livenessGuardModel) callOnce(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if iter, ok := m.inner.(trpcmodel.IterModel); ok {
		seq, err := iter.GenerateContentIter(ctx, request)
		if err != nil {
			return nil, err
		}
		if seq == nil {
			return nil, ErrModelNilChannel
		}
		ch := make(chan *trpcmodel.Response, 16)
		safego.Go(ctx, "liveness-guard-iter-pump", func() {
			defer close(ch)
			seq(func(resp *trpcmodel.Response) bool {
				select {
				case ch <- resp:
					return true
				case <-ctx.Done():
					return false
				}
			})
		})
		return ch, nil
	}
	return m.inner.GenerateContent(ctx, request)
}

// generateWithGuard 首发 attempt 同步发起（保持 GenerateContent 的错误契约），
// 随后的首字节等待、stall 监控、重连循环全部在后台 goroutine 执行。
// 返回的 out 在任何终局（成功流尽 / 重连穷尽终态错误 / ctx 取消）都会关闭。
func (m *livenessGuardModel) generateWithGuard(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if ctx.Err() != nil {
		return m.callOnce(ctx, request)
	}
	fbBudget := firstByteRetryBudgetFromContext(ctx)
	policy := stallPolicyFromContext(ctx)

	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	ch1, err := m.callOnce(attemptCtx, request)
	if err != nil {
		cancelAttempt()
		return nil, err
	}
	if ch1 == nil {
		cancelAttempt()
		return nil, ErrModelNilChannel
	}

	out := make(chan *trpcmodel.Response, 16)
	safego.Go(ctx, "liveness-guard-forward", func() {
		defer close(out)
		m.forwardLoop(ctx, request, ch1, cancelAttempt, fbBudget, policy, out)
	})
	return out, nil
}

// forwardLoop 是活性守卫主循环：转发当前 attempt 的事件流，监控活性；
// stall / stream_error 触发重连，直到成功流尽或重连穷尽。
// ch 与 cancelAttempt 属于当前 attempt（重连时整体替换）。
func (m *livenessGuardModel) forwardLoop(
	ctx context.Context,
	request *trpcmodel.Request,
	ch <-chan *trpcmodel.Response,
	cancelAttempt context.CancelFunc,
	fbBudget time.Duration,
	policy stallPolicy,
	out chan<- *trpcmodel.Response,
) {
	// 闭包捕获变量而非 defer 时求值：重连替换 cancelAttempt 后，
	// 退出时必须取消的是最新 attempt 的 ctx。
	defer func() { cancelAttempt() }()
	reconnects := 0
	gotFirstEvent := false
	for {
		// 活性计时器：首事件前用首字节预算（0 = 该维度不监控，仅靠 stall
		// 预算兜底首事件），首事件后用 stall 预算。nil channel 永不触发。
		var timer *time.Timer
		var timerC <-chan time.Time
		if d := m.activeBudget(fbBudget, policy, gotFirstEvent); d > 0 {
			timer = time.NewTimer(d)
			timerC = timer.C
		}
		select {
		case resp, ok := <-ch:
			if timer != nil {
				timer.Stop()
			}
			if !ok {
				// 本 attempt 流尽：正常收官。
				return
			}
			gotFirstEvent = true
			if isReconnectableStreamError(resp) && reconnects < policy.maxReconnects {
				// 中段 stream_error（连接健康类故障）：不转发半截错误，
				// 直接重连重新生成。
				if next, nextCancel, ok2 := m.reconnect(ctx, request, ch, cancelAttempt, reconnects, policy, "stream_error"); ok2 {
					ch, cancelAttempt = next, nextCancel
					reconnects++
					gotFirstEvent = false
					continue
				}
				// ctx 已取消或重连发起失败：reconnect 内已处理终局。
				return
			}
			if !forwardResponse(ctx, out, normalizeErrorObject(resp)) {
				safego.Go(context.Background(), "liveness-guard-drain", func() { drainResponseChannel(ch) })
				return
			}
		case <-timerC:
			// 活性超时：取消本 attempt（断开 HTTP），退避后重连。
			phase := "first_byte"
			if gotFirstEvent {
				phase = "stall"
			}
			if reconnects >= policy.maxReconnects {
				m.emitTerminalStallError(ctx, out, phase, policy, reconnects)
				safego.Go(context.Background(), "liveness-guard-drain", func() { drainResponseChannel(ch) })
				return
			}
			if next, nextCancel, ok := m.reconnect(ctx, request, ch, cancelAttempt, reconnects, policy, phase); ok {
				ch, cancelAttempt = next, nextCancel
				reconnects++
				gotFirstEvent = false
				continue
			}
			return
		case <-ctx.Done():
			// 用户取消/整轮超时优先于重连决策。
			safego.Go(context.Background(), "liveness-guard-drain", func() { drainResponseChannel(ch) })
			return
		}
	}
}

// activeBudget 返回当前阶段的活性预算：首事件前优先首字节预算（0 = 该
// 维度不监控，由 stall 预算兜底首事件）；首事件后一律 stall 预算。
func (m *livenessGuardModel) activeBudget(fbBudget time.Duration, policy stallPolicy, gotFirstEvent bool) time.Duration {
	if gotFirstEvent {
		return policy.timeout
	}
	if fbBudget > 0 {
		return fbBudget
	}
	return policy.timeout
}

// reconnect 取消当前 attempt、排空其 channel、按第 n+1 次重连退避，然后
// 发起新 attempt。返回 ok=false 表示终局已定（ctx 取消或重连发起失败，
// 失败已记日志，out 由 forwardLoop  defer 关闭，消费端守卫按首字节超时
// 判死整轮），调用方直接结束循环。
func (m *livenessGuardModel) reconnect(
	ctx context.Context,
	request *trpcmodel.Request,
	ch <-chan *trpcmodel.Response,
	cancelAttempt context.CancelFunc,
	reconnects int,
	policy stallPolicy,
	reason string,
) (<-chan *trpcmodel.Response, context.CancelFunc, bool) {
	cancelAttempt()
	safego.Go(context.Background(), "liveness-guard-drain", func() { drainResponseChannel(ch) })
	// LLMFirstByteRetryTotal 语义随治理扩展为「活性重连总数」（首字节
	// 静默 + 流中段 stall + 中段 stream_error），metric 名保留以兼容
	// 既有 dashboard；区分维度看 provider.liveness_reconnect 日志 reason。
	metrics.LLMFirstByteRetryTotal.WithLabelValues(m.provider, m.model).Inc()
	if m.lg != nil {
		m.lg.Warn("LLM 流活性缺失，取消本次调用并重连",
			loggateway.StepID("provider.liveness_reconnect"),
			loggateway.Str("provider", m.provider),
			loggateway.Str("model", m.model),
			loggateway.Str("reason", reason),
			loggateway.Int("reconnect", reconnects+1),
			loggateway.Int("max_reconnects", policy.maxReconnects),
			loggateway.Str("stall_timeout", policy.timeout.String()))
	}
	backoff := stallBackoff(reconnects + 1)
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return nil, nil, false
	}
	attemptCtx, cancel := context.WithCancel(ctx)
	next, err := m.callOnce(attemptCtx, request)
	if err != nil {
		cancel()
		m.logReconnectFailure(err)
		return nil, nil, false
	}
	if next == nil {
		cancel()
		m.logReconnectFailure(ErrModelNilChannel)
		return nil, nil, false
	}
	return next, cancel, true
}

// emitTerminalStallError 在重连穷尽时向流尾产出终态错误响应（走正常失败链）。
func (m *livenessGuardModel) emitTerminalStallError(ctx context.Context, out chan<- *trpcmodel.Response, phase string, policy stallPolicy, reconnects int) {
	msg := fmt.Sprintf("llm stream liveness exhausted: phase=%s reconnects=%d/%d stall_timeout=%s",
		phase, reconnects, policy.maxReconnects, policy.timeout)
	if m.lg != nil {
		m.lg.Warn("LLM 流活性重连穷尽，判死",
			loggateway.StepID("provider.liveness_exhausted"),
			loggateway.Str("provider", m.provider),
			loggateway.Str("model", m.model),
			loggateway.Str("reason", msg))
	}
	resp := &trpcmodel.Response{
		Object: trpcmodel.ObjectTypeError,
		Done:   true,
		Error: &trpcmodel.ResponseError{
			Type:    trpcmodel.ErrorTypeStreamError,
			Message: msg,
		},
	}
	forwardResponse(ctx, out, resp)
}

// logReconnectFailure 记录重连发起失败；out 由 forwardLoop 的 defer close
// 关闭，消费端首字节守卫会按首字节超时判死整轮。
func (m *livenessGuardModel) logReconnectFailure(err error) {
	if m.lg != nil {
		m.lg.Warn("LLM 重连发起失败，放弃",
			loggateway.StepID("provider.liveness_reconnect_fail"),
			loggateway.Str("provider", m.provider),
			loggateway.Str("model", m.model),
			loggateway.Err(err))
	}
}

// isReconnectableStreamError 判定中段连接健康类错误（可重连）；语义错误
// （api_error/flow_error/billing 等）不可重连，直接透传。
func isReconnectableStreamError(resp *trpcmodel.Response) bool {
	return resp != nil && resp.Error != nil && resp.Error.Type == trpcmodel.ErrorTypeStreamError
}

// normalizeErrorObject 归一化错误响应：Error 非空而 Object 为空时置
// ObjectTypeError——框架 isTerminalAgentErrorEvent 的 Object 门要求
// Object == "error"，openai 适配器错误响应原生 Object 为空，不归一化则
// 团队 WithPropagateChildAgentErrors 对 stream_error 不生效。
func normalizeErrorObject(resp *trpcmodel.Response) *trpcmodel.Response {
	if resp != nil && resp.Error != nil && resp.Object == "" {
		resp.Object = trpcmodel.ObjectTypeError
	}
	return resp
}

// forwardResponse forwards one response, returning false when ctx is done.
func forwardResponse(ctx context.Context, out chan<- *trpcmodel.Response, resp *trpcmodel.Response) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- resp:
		return true
	}
}
