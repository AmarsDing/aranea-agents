package provider

import (
	"context"
	"time"
)

// TaskType 表示 LLM 调用的任务类型，用于选择超时策略。
type TaskType string

const (
	// TaskTypeSimple 普通对话。
	TaskTypeSimple TaskType = "simple"
	// TaskTypeModerate 中等复杂度。
	TaskTypeModerate TaskType = "moderate"
	// TaskTypeComplex 深度推理。
	TaskTypeComplex TaskType = "complex"
	// TaskTypeGraphNode Graph 节点。
	TaskTypeGraphNode TaskType = "graph_node"
	// TaskTypeCodeGen 代码生成。
	TaskTypeCodeGen TaskType = "code_gen"
	// TaskTypeUnknown 未知类型（兜底）。
	TaskTypeUnknown TaskType = "unknown"
)

// 默认超时配置常量（CS-B8：禁止魔法数字，必须定义命名常量）。
const (
	// defaultSimpleTimeout 普通对话默认超时（30min）。
	defaultSimpleTimeout = 30 * time.Minute
	// defaultModerateTimeout 中等复杂度默认超时（60min）。
	defaultModerateTimeout = 60 * time.Minute
	// defaultComplexTimeout 深度推理默认超时（120min）。
	defaultComplexTimeout = 120 * time.Minute
	// defaultGraphNodeTimeout Graph 节点默认超时（60min）。
	defaultGraphNodeTimeout = 60 * time.Minute
	// defaultCodeGenTimeout 代码生成默认超时（90min）。
	defaultCodeGenTimeout = 90 * time.Minute
	// defaultUnknownTimeout 未知类型兜底超时（90min）。
	defaultUnknownTimeout = 90 * time.Minute
	// defaultFallbackTimeout 当 TaskType 未在 timeouts map 中且 Unknown 也缺失时的回退超时（30min）。
	defaultFallbackTimeout = 30 * time.Minute
	// maxAllowedTimeout 全局硬上限（120min），任何配置的超时不得超过此值。
	maxAllowedTimeout = 120 * time.Minute
)

// LLMLeakGuardTimeout 是 LLM HTTP 客户端的统一防泄漏安全网（2026-09-01
// 活性守卫治理）：业务超时由「活性守卫（livenessGuardModel）+ 整轮 runCtx」
// 治理，墙钟只负责回收 goroutine/连接双失效的死挂请求，永不干预业务。
// 取代 wire.go 既有 60min/120s/120s/90s 四处硬编码与 resolver_model.go 的
// nil-RT fallback 120s。
const LLMLeakGuardTimeout = 6 * time.Hour

// TimeoutPolicy 按任务类型动态选择 HTTP 超时。
//
// 构造后不可变（WithTimeout 应仅在构造阶段链式调用），因此并发安全：
// 多个 goroutine 可同时调用 TimeoutFor 读取超时值。
type TimeoutPolicy struct {
	timeouts       map[TaskType]time.Duration
	defaultTimeout time.Duration
	maxTimeout     time.Duration // 全局硬上限
}

// NewTimeoutPolicy 创建默认配置的 TimeoutPolicy。
//
// 默认超时：
//   - simple:     30min
//   - moderate:   60min
//   - complex:    120min
//   - graph_node: 60min
//   - code_gen:   90min
//   - unknown:    90min（兜底）
//   - default:    30min
//   - max:        120min（全局硬上限）
func NewTimeoutPolicy() *TimeoutPolicy {
	return &TimeoutPolicy{
		timeouts: map[TaskType]time.Duration{
			TaskTypeSimple:    defaultSimpleTimeout,
			TaskTypeModerate:  defaultModerateTimeout,
			TaskTypeComplex:   defaultComplexTimeout,
			TaskTypeGraphNode: defaultGraphNodeTimeout,
			TaskTypeCodeGen:   defaultCodeGenTimeout,
			TaskTypeUnknown:   defaultUnknownTimeout,
		},
		defaultTimeout: defaultFallbackTimeout,
		maxTimeout:     maxAllowedTimeout,
	}
}

// TimeoutFor 返回指定任务类型的超时时间。
//
// 行为：
//   - 已配置的 TaskType 返回其配置值
//   - 未配置的 TaskType 返回 unknown 兜底值
//   - unknown 也缺失时返回 defaultTimeout
//   - 任何超过 maxTimeout 的值被截断为 maxTimeout
//   - nil receiver 返回 defaultFallbackTimeout（防御性编程，红线 #26）
func (p *TimeoutPolicy) TimeoutFor(taskType TaskType) time.Duration {
	if p == nil {
		return defaultFallbackTimeout
	}
	d, ok := p.timeouts[taskType]
	if !ok {
		// 未知 TaskType 回退到 unknown 兜底值。
		d = p.timeouts[TaskTypeUnknown]
		if d == 0 {
			d = p.defaultTimeout
		}
	}
	if d > p.maxTimeout {
		return p.maxTimeout
	}
	return d
}

// WithTimeout 链式配置指定任务类型的超时时间。
//
// 应在构造阶段使用（NewTimeoutPolicy 之后、并发读取之前），
// 构造完成后不再调用以保证并发安全。
//
// 返回 receiver 自身以支持链式调用：
//
//	p := NewTimeoutPolicy().
//	    WithTimeout(TaskTypeSimple, 10*time.Minute).
//	    WithTimeout(TaskTypeComplex, 90*time.Minute)
func (p *TimeoutPolicy) WithTimeout(taskType TaskType, d time.Duration) *TimeoutPolicy {
	if p == nil {
		return nil
	}
	if p.timeouts == nil {
		p.timeouts = make(map[TaskType]time.Duration)
	}
	p.timeouts[taskType] = d
	return p
}

// MaxTimeout 返回全局硬上限。
func (p *TimeoutPolicy) MaxTimeout() time.Duration {
	if p == nil {
		return maxAllowedTimeout
	}
	return p.maxTimeout
}

// DefaultTimeout 返回兜底默认超时（当 TaskType 未配置且 unknown 也缺失时使用）。
func (p *TimeoutPolicy) DefaultTimeout() time.Duration {
	if p == nil {
		return defaultFallbackTimeout
	}
	return p.defaultTimeout
}

// --- Context-based TaskType propagation ---

// taskTypeKey 是 context 中传递 TaskType 的键类型（未导出，避免冲突）。
type taskTypeKey struct{}

// WithTaskType 将 TaskType 存入 context，返回新的 context。
//
// 调用方在发起 LLM 调用前通过此函数标记任务类型，LLM 调用路径
// 可读取该值并应用对应的超时策略：
//
//	ctx = provider.WithTaskType(ctx, provider.TaskTypeCodeGen)
//	resp, err := model.GenerateContent(ctx, req)
func WithTaskType(ctx context.Context, taskType TaskType) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskTypeKey{}, taskType)
}

// TaskTypeFromCtx 从 context 读取 TaskType。
// 如果 context 中未设置 TaskType，返回 TaskTypeUnknown。
func TaskTypeFromCtx(ctx context.Context) TaskType {
	if ctx == nil {
		return TaskTypeUnknown
	}
	v, ok := ctx.Value(taskTypeKey{}).(TaskType)
	if !ok || v == "" {
		return TaskTypeUnknown
	}
	return v
}

// ApplyTimeoutFromCtx 根据 context 中的 TaskType 和 TimeoutPolicy 创建带超时的 context。
//
// 行为：
//   - 如果 ctx 中有 TaskType（非 Unknown），返回 context.WithTimeout(ctx, policy.TimeoutFor(taskType))
//   - 如果 ctx 中没有 TaskType（TaskTypeUnknown），返回原 ctx 和 nil cancel（不修改）
//   - 如果 policy 为 nil，返回原 ctx 和 nil cancel（不修改）
//
// 返回值：
//   - 新的 context（可能带超时）
//   - cancel 函数（非 nil 时调用方必须 defer 调用，否则 context 泄漏）
//   - 应用的超时时长（0 表示未应用超时）
//
// 调用方应在发起 LLM 请求前调用此函数：
//
//	ctx, cancel, timeout := policy.ApplyTimeoutFromCtx(ctx)
//	if cancel != nil {
//	    defer cancel()
//	}
//	resp, err := model.GenerateContent(ctx, req)
func (p *TimeoutPolicy) ApplyTimeoutFromCtx(ctx context.Context) (context.Context, context.CancelFunc, time.Duration) {
	if p == nil || ctx == nil {
		return ctx, nil, 0
	}
	taskType := TaskTypeFromCtx(ctx)
	if taskType == TaskTypeUnknown {
		// 未显式标记 TaskType 时不覆盖 http.Client 的默认超时。
		return ctx, nil, 0
	}
	timeout := p.TimeoutFor(taskType)
	if timeout <= 0 {
		return ctx, nil, 0
	}
	newCtx, cancel := context.WithTimeout(ctx, timeout)
	return newCtx, cancel, timeout
}
