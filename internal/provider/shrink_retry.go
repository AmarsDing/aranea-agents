package provider

// shrink_retry.go — A7 SHRINK_STEPS 装饰器（P0-3 设计 §4.4② / §12.2 P0-3②
// 审批裁定）。ContextOverflow/400（marker 分类，见 retry_classifier.go 的
// IsContextOverflow）不再是"重发同尺寸请求必败"的死局：装饰器按
// ShrinkSteps [0.7, 0.5, 0.35] 逐级收缩目标输入预算（MaxInputTokens），
// 恰好 3 次递减；用尽后把"原始第一错"原样上浮（用尽回原错）。
//
// 边界与裁定：
//   - 只包 LLM 调用层；不触碰 event/telemetry/metrics 通路（审批硬约束）。
//   - 收缩落在 MaxInputTokens 目标预算（经 Rebuild 回调重建模型），
//     不直接改 messages——裁剪由 token tailoring 既有机能执行
//     （openai.Model.applyTokenTailoring），避免双重裁剪。
//   - 429/5xx/瞬时类永不触发（IsContextOverflow 恒 false），与
//     retry_transport 的 ClassifyRetry 触发集互斥——不双重重试。
//   - 默认零行为变更：Rebuild 为 nil 或 BaselineInputTokens ≤ 0 时
//     WrapModelWithShrinkRetry 直接返回原模型（pass-through），wire 接线
//     在批 B（trpc_llm.go 本批不动）。建议接线位（批 B 参考）：wrapHA 之
//     后、WrapModelWithMetrics 内侧之外的 provider-具体模型上，使收缩针对
//     具体 provider 的预算生效。
//   - AS-EVT-01：零新增事件——每步收缩一条 Warn 进程日志（反作用缓解#2：
//     不刷 Info 降噪），失败终态仍走既有 critical 路径。
//
// 安全性：每次尝试在请求副本上执行（cloneRequestPerAttempt）。框架的
// token tailoring 会 request.Messages = tailored 就地改写（modeltailoring.
// ApplyResult 实测）——若重试共用同一指针，tailoring 会基于"已被裁剪"的历史
// 二次裁剪，双重失水。副本语义意味着：enabled 态下调用方的 request 不再被
// tailoring 就地改写。

import (
	"context"
	"math"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ShrinkSteps 是 overflow 收缩比例阶梯（OpenCode 同构物）：第 i 次收缩的
// 目标预算 = round(BaselineInputTokens * ShrinkSteps[i])。恰好 3 级递减；
// 用尽收眼底回原错。包级变量而非散落魔法数（后续结构化字段接入只改表）。
var ShrinkSteps = []float64{0.7, 0.5, 0.35}

// ShrinkRetryOptions 配置 WrapModelWithShrinkRetry。零值即 pass-through
// （Rebuild=nil）。Go Options 结构体 + 默认 OFF 模式：批 A 内不经任何
// conf 字段注入，行为与现状字节级一致。
type ShrinkRetryOptions struct {
	// BaselineInputTokens 是收缩前的有效输入预算（通常取模型目录的
	// MaxInputTokens / ContextWindow 预算）；≤0 时装饰器直接失效。
	BaselineInputTokens int
	// Rebuild 以新的 MaxInputTokens 预算重建模型；nil 时装饰器直接失效
	// （默认零行为变更，接线在批 B）。返回的模型必须已把 budget 落进
	// token tailoring（WithMaxInputTokens 类构造参数）。
	Rebuild func(maxInputTokens int) (trpcmodel.Model, error)
	// Provider / Model 仅用于日志上下文字段。
	Provider string
	Model    string
	// Logger 构造注入；nil 时按 Noop 处理（不强制非空，与包内既有构造约定一致）。
	Logger loggateway.Logger
	// Steps 覆盖 ShrinkSteps（仅测试用；生产留空走包级阶梯）。
	Steps []float64
}

// shrinkRetryModel 是包级装饰器实现。
type shrinkRetryModel struct {
	inner    trpcmodel.Model
	baseline int
	rebuild  func(maxInputTokens int) (trpcmodel.Model, error)
	steps    []float64
	provider string
	model    string
	lg       loggateway.Logger
}

// WrapModelWithShrinkRetry 在 inner 上叠加 SHRINK_STEPS 语义。options 为
// 零值（Rebuild==nil 或 BaselineInputTokens<=0）时原样返回 inner——不开关
// 即零开销零行为变更。
//
// 注意：包装结果刻意不实现 IterModel——channel 路径需要"首帧窥视"来透明
// 重试流式 overflow（见 GenerateContent），IterModel 的 caller-goroutine
// pull 语义无法在不缓冲/不泄漏的前提下重放首帧后的流，该精确修复归框架期
// （批 C second-pass）。llmflow 对非 IterModel 自动退回 GenerateContent
// 路径，开销与 WrapModelWithMetrics 的非 iter 分支等同。
func WrapModelWithShrinkRetry(inner trpcmodel.Model, opts ShrinkRetryOptions) trpcmodel.Model {
	if inner == nil || opts.Rebuild == nil || opts.BaselineInputTokens <= 0 {
		return inner
	}
	lg := opts.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	steps := opts.Steps
	if len(steps) == 0 {
		steps = ShrinkSteps
	}
	return &shrinkRetryModel{
		inner:    inner,
		baseline: opts.BaselineInputTokens,
		rebuild:  opts.Rebuild,
		steps:    steps,
		provider: opts.Provider,
		model:    opts.Model,
		lg:       lg,
	}
}

// Info 透传 inner 的 Info（预算收缩不申报为模型恒量变化）。
func (m *shrinkRetryModel) Info() trpcmodel.Info { return m.inner.Info() }

// cloneRequestPerAttempt 返回一次尝试专用的请求副本：值拷贝 + Messages
// 切片头拷贝，隔离 tailoring 的就地改写（见文件头"安全性"段）。
func cloneRequestPerAttempt(request *trpcmodel.Request) *trpcmodel.Request {
	if request == nil {
		return nil
	}
	cp := *request
	if request.Messages != nil {
		cp.Messages = append([]trpcmodel.Message(nil), request.Messages...)
	}
	return &cp
}

// shrinkBudget computes the target input budget for shrink step i.
func (m *shrinkRetryModel) shrinkBudget(step int) int {
	budget := int(math.Round(float64(m.baseline) * m.steps[step]))
	if budget < 1 {
		budget = 1
	}
	return budget
}

// GenerateContent 实现透明收缩重试。两类 overflow 表面均被接管：
//  1. GenerateContent 的 error 返回（连接前/构造期失败）；
//  2. 流式首帧的 Response.Error（provider 拒绝 oversize 请求的常见形态，
//     openai 框架路径到 ErrorTypeAPIError 的单帧终态）。
//
// 首帧窥视语义：只缓存第一个响应；一旦它不是 overflow 错误（正常流开始）
// 即原样回放且不再干预——绝不在已产出内容后重试（重复副作用防线）。
// 用尽（3 级用尽 / Rebuild 失败 / ctx 取消）一律上浮"原始第一错"。
func (m *shrinkRetryModel) GenerateContent(ctx context.Context, request *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	current := m.inner
	step := 0
	var firstErr error
	var firstResp *trpcmodel.Response
	for {
		if step > 0 {
			m.lg.Warn("llm context overflow: shrinking input budget for retry",
				loggateway.StepID("provider.llm.shrink_retry"),
				loggateway.Phase("shrinking"),
				loggateway.Str("provider", m.provider),
				loggateway.Str("model", m.model),
				loggateway.Int("shrink_step", step),
				loggateway.Float64("shrink_ratio", m.steps[step-1]),
				loggateway.Int("max_input_tokens", m.shrinkBudget(step-1)))
		}
		attemptReq := cloneRequestPerAttempt(request)
		ch, err := current.GenerateContent(ctx, attemptReq)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if !m.canShrinkRetry(ctx, step) || !IsContextOverflowError(err) {
				return nil, firstErr
			}
			next, rerr := m.rebuild(m.shrinkBudget(step))
			if rerr != nil {
				return nil, firstErr
			}
			current, step = next, step+1
			continue
		}
		if ch == nil {
			// 与 WrapModelWithMetrics 同款防御：框架契约禁止 (nil, nil)。
			return nil, ErrModelNilChannel
		}
		// 首帧窥视。
		resp, ok := <-ch
		if !ok {
			// 空流：原样（异常但非 overflow，交上层既有失败路径处置）。
			if firstErr != nil {
				return nil, firstErr
			}
			empty := make(chan *trpcmodel.Response)
			close(empty)
			return empty, nil
		}
		if resp != nil && resp.Error != nil && IsContextOverflow(resp.Error.Message) {
			// 流形态的"原错"就是首个 overflow 响应帧本身（用尽回原错）。
			if firstResp == nil {
				firstResp = resp
				if firstErr == nil {
					firstErr = resp.Error
				}
			}
			if m.canShrinkRetry(ctx, step) {
				drainResponseChannel(ch) // 释放 inner 流，防 goroutine 泄漏。
				if next, rerr := m.rebuild(m.shrinkBudget(step)); rerr == nil {
					current, step = next, step+1
					continue
				}
			}
			// 用尽（阶梯耗尽 / Rebuild 失败 / ctx 取消）：排空当前尝试流，
			// 回放"原始第一帧"——与未加装饰器时调用端所见的失败逐项一致。
			drainResponseChannel(ch)
			return closedStreamWith(firstResp), nil
		}
		// 非 overflow：首帧+余流原样回放。
		return concatFirstResponse(ctx, resp, ch), nil
	}
}

// closedStreamWith replays a single terminal response then closes — the exact
// wire shape an un-decorated overflow failure stream has (单帧终态).
func closedStreamWith(resp *trpcmodel.Response) <-chan *trpcmodel.Response {
	out := make(chan *trpcmodel.Response, 1)
	if resp != nil {
		out <- resp
	}
	close(out)
	return out
}

// canShrinkRetry 报告是否还有收缩余量且 ctx 未取消。
func (m *shrinkRetryModel) canShrinkRetry(ctx context.Context, step int) bool {
	return step < len(m.steps) && ctx.Err() == nil
}

// concatFirstResponse 回放"窥视到的首帧 + 剩余流"，保持原有流形契约。
// ctx 取消时排空 rest 以释放上游 goroutine（与 metrics_model 同守则）。
func concatFirstResponse(ctx context.Context, first *trpcmodel.Response, rest <-chan *trpcmodel.Response) <-chan *trpcmodel.Response {
	out := make(chan *trpcmodel.Response, 16)
	safego.Go(ctx, "shrink-retry-replay", func() {
		defer close(out)
		select {
		case <-ctx.Done():
			safego.Go(ctx, "shrink-retry-drain", func() { drainResponseChannel(rest) })
			return
		case out <- first:
		}
		for {
			select {
			case <-ctx.Done():
				safego.Go(ctx, "shrink-retry-drain", func() { drainResponseChannel(rest) })
				return
			case resp, ok := <-rest:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					safego.Go(ctx, "shrink-retry-drain", func() { drainResponseChannel(rest) })
					return
				case out <- resp:
				}
			}
		}
	})
	return out
}
