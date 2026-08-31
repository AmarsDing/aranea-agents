package provider

import (
	"context"
	"time"

	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// firstByteRetryGuardSlack 是消费端首字节守卫在「重试预算×2」之外附加的
// 宽限：覆盖 attempt1 取消/排空与 attempt2 建连的调度抖动，避免守卫与
// 第二次尝试的超时同时开火导致误杀。
const firstByteRetryGuardSlack = 2 * time.Second

// firstByteRetryBudgetKey 携带本轮每次模型调用允许的首字节静默预算。
// 编排层在解析出 firstByteTimeout 后注入 runCtx；模型调用链（framework
// flow → Model.GenerateContent/GenerateContentIter）继承同一 ctx，
// 装饰器据此在供应商静默超时时自动重试一次（P6-N3，S09 t32 实证：
// deepseek 首字节 stall 30s 直接打死整轮且无自动恢复）。
type firstByteRetryBudgetKey struct{}

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

// FirstByteGuardWithRetry 返回消费端首字节守卫在与模型级单发重试并存时应
// 使用的总时限：重试需要 attempt1(预算) + attempt2(预算) 两段窗口，守卫
// 必须等第二次尝试也静默后才可判死，否则会在重试中途取消 runCtx 把恢复
// 中的调用误杀（两机制共享同一 runCtx）。
func FirstByteGuardWithRetry(budget time.Duration) time.Duration {
	if budget <= 0 {
		return budget
	}
	return 2*budget + firstByteRetryGuardSlack
}

// WrapModelWithFirstByteRetry wraps inner with a single automatic same-model
// retry on first-byte silence. Silence = no response event within the budget
// carried by ctx (WithFirstByteRetryBudget); without a budget the wrapper is
// a pure pass-through. An error event as first event is NOT a stall and is
// forwarded immediately without retry. When inner implements IterModel the
// wrapper preserves it so the framework keeps its preferred iterator path.
//
// 为什么安全：首字节静默意味着该次调用零产出（无 token、无工具调用、无
// session 事件），取消后重试等价于传输层重连，不产生重复副作用；且编排层
// 的首字节守卫总时限已按 FirstByteGuardWithRetry 放宽，不会误杀重试。
// 不设 per-turn 次数上限：每次 GenerateContent 独立决策，供应商彻底死亡时
// 由消费端守卫兜底终止整轮，重试的取消请求本身不计费（未产出 token）。
func WrapModelWithFirstByteRetry(inner trpcmodel.Model, providerType, modelName string, lg loggateway.Logger) trpcmodel.Model {
	if inner == nil {
		return nil
	}
	base := &firstByteRetryModel{
		inner:    inner,
		provider: providerType,
		model:    modelName,
		lg:       lg,
	}
	if _, ok := inner.(trpcmodel.IterModel); ok {
		return &firstByteRetryIterModel{firstByteRetryModel: base}
	}
	return base
}

type firstByteRetryModel struct {
	inner    trpcmodel.Model
	provider string
	model    string
	lg       loggateway.Logger
}

type firstByteRetryIterModel struct {
	*firstByteRetryModel
}

func (m *firstByteRetryModel) Info() trpcmodel.Info {
	return m.inner.Info()
}

func (m *firstByteRetryModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return m.generateWithRetry(ctx, request)
}

func (m *firstByteRetryIterModel) GenerateContentIter(ctx context.Context, request *trpcmodel.Request) (trpcmodel.Seq[*trpcmodel.Response], error) {
	ch, err := m.generateWithRetry(ctx, request)
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
				// generateWithRetry 的转发 goroutine 永远阻塞在 out<-。
				safego.Go(ctx, "first-byte-retry-drain-abandoned", func() { drainResponseChannel(ch) })
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
func (m *firstByteRetryModel) callOnce(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if iter, ok := m.inner.(trpcmodel.IterModel); ok {
		seq, err := iter.GenerateContentIter(ctx, request)
		if err != nil {
			return nil, err
		}
		if seq == nil {
			return nil, ErrModelNilChannel
		}
		ch := make(chan *trpcmodel.Response, 16)
		safego.Go(ctx, "first-byte-retry-iter-pump", func() {
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

// generateWithRetry runs attempt 1 under a cancellable child ctx and watches
// for the first event. On silence within the ctx budget it cancels attempt 1
// (kills the in-flight HTTP request) and retries exactly once under the
// parent ctx. The second attempt is passed through without a deadline — if it
// also stalls, the consume-level guard (FirstByteGuardWithRetry) terminates
// the turn.
func (m *firstByteRetryModel) generateWithRetry(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	budget := firstByteRetryBudgetFromContext(ctx)
	if budget <= 0 || ctx.Err() != nil {
		return m.callOnce(ctx, request)
	}

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
	timer := time.NewTimer(budget)
	select {
	case resp, ok := <-ch1:
		// 首事件在预算内到达（含 channel 关闭=空流）：原样转发，不重试。
		timer.Stop()
		safego.Go(ctx, "first-byte-retry-forward", func() {
			defer close(out)
			defer cancelAttempt()
			if ok && !forwardResponse(ctx, out, resp) {
				safego.Go(ctx, "first-byte-retry-drain", func() { drainResponseChannel(ch1) })
				return
			}
			forwardResponseStream(ctx, out, ch1)
		})
		return out, nil
	case <-timer.C:
		// 首字节静默：取消 attempt1（未产出任何内容，无副作用），
		// 后台排空其 channel，同模型重试一次。
		cancelAttempt()
		safego.Go(ctx, "first-byte-retry-drain", func() { drainResponseChannel(ch1) })
		metrics.LLMFirstByteRetryTotal.WithLabelValues(m.provider, m.model).Inc()
		if m.lg != nil {
			m.lg.Warn("供应商首字节静默超时，同模型自动重试一次",
				loggateway.StepID("provider.first_byte_retry"),
				loggateway.Str("provider", m.provider),
				loggateway.Str("model", m.model),
				loggateway.Str("budget", budget.String()))
		}
		ch2, err2 := m.callOnce(ctx, request)
		if err2 != nil {
			return nil, err2
		}
		if ch2 == nil {
			return nil, ErrModelNilChannel
		}
		safego.Go(ctx, "first-byte-retry-forward", func() {
			defer close(out)
			forwardResponseStream(ctx, out, ch2)
		})
		return out, nil
	case <-ctx.Done():
		// 用户取消/整轮超时优先于重试决策。
		timer.Stop()
		cancelAttempt()
		safego.Go(context.Background(), "first-byte-retry-drain", func() { drainResponseChannel(ch1) })
		return nil, ctx.Err()
	}
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

// forwardResponseStream forwards the remainder of ch into out until ch closes
// or ctx is cancelled (then ch is drained in the background to unblock the
// producer goroutine).
func forwardResponseStream(ctx context.Context, out chan<- *trpcmodel.Response, ch <-chan *trpcmodel.Response) {
	for {
		select {
		case <-ctx.Done():
			safego.Go(ctx, "first-byte-retry-drain", func() { drainResponseChannel(ch) })
			return
		case resp, ok := <-ch:
			if !ok {
				return
			}
			if !forwardResponse(ctx, out, resp) {
				safego.Go(ctx, "first-byte-retry-drain", func() { drainResponseChannel(ch) })
				return
			}
		}
	}
}
