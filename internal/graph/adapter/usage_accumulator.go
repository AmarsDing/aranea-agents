package adapter

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// usage_accumulator.go — 独立图运行（含 twinmonitor 门面触发）的 LLM 用量
// 事件泵累计器。
//
// 计量语义与 agent.accumulateStreamUsage / turn_helpers.accumulateRound 的
// 框架既有口径对齐（2026-09-12 复查修正：原 first-non-zero 实现在提供方按
// 块累计上报 usage 时会漏计 completion 增量）：
//   - 调用内累计取 MAX：同一次 LLM 调用（round）的流式事件 usage 按块累计
//     上报——prompt 在调用内恒定，completion 单调增长，故每个计费键取观测
//     最大值（等价于 accumulateRound 的"同 prompt 取 completion 最大"）。
//   - 计费键：优先 Response.ID（与计费调用一一对应，框架 runner/
//     stream_consumer 同用该 ID 做持久化去重）；无 ID 时退化为
//     author|model|prompt 三元组——prompt 变化即新计费调用（工具循环中
//     prompt 增长、压缩后收缩，与 accumulateRound 的 round 边界探测同
//     启发式）。
//   - 分桶落账：按 Response.Model 聚合（多模型节点并存时分别落账，
//     handleGetRun 的 model_used 取聚合量最大的模型）。
//   - 对象门禁：仅统计 chat.completion / chunk 对象（与 stream_consumer
//     的 isChatCompletionStreamObject 同口径），避免非 LLM 响应对象
//     携带 usage 时误计。
//   - 零 token 跳过：无 Response.Usage 或全零不累计。
//
// 红线约束：
//   - best-effort：落账失败仅 Warn，绝不阻断图执行主链路。
//   - 线程安全：forwardEvents 单 goroutine 调用 observe，但 flush 在流结束
//     后同步调用；加锁防御未来并发（与 SpanCollector 互斥量模式一致）。
//   - 团队运行不重复计费：消费方（biz.GraphUsageRecorder 实现）负责按
//     会话归属过滤；本累计器忠实上报全部观测到的用量。

// graphUsageAccumulator 累计一次图运行的 LLM 用量。
type graphUsageAccumulator struct {
	sessionID string
	execID    string
	graphID   string
	rec       biz.GraphUsageRecorder
	lg        loggateway.Logger

	mu sync.Mutex
	// byCall 计费键 → 调用内最大观测（prompt 恒定取 MAX，completion 累计
	// 取 MAX）。流结束时按 model 聚合各计费键落账。
	byCall map[string]*callUsage
}

// callUsage 一次计费调用的最终 token（调用内各事件取最大值）。
type callUsage struct {
	model      string
	prompt     int
	completion int
}

func newGraphUsageAccumulator(sessionID, execID, graphID string, rec biz.GraphUsageRecorder, lg loggateway.Logger) *graphUsageAccumulator {
	if rec == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &graphUsageAccumulator{
		sessionID: sessionID,
		execID:    execID,
		graphID:   graphID,
		rec:       rec,
		lg:        lg,
		byCall:    make(map[string]*callUsage),
	}
}

// usageObjectGate 与 agent.isChatCompletionStreamObject 同口径（该函数为
// internal/agent 包私有，此处按契约复刻：空对象与两类 chat.completion）。
func usageObjectGate(objType string) bool {
	switch objType {
	case "", "chat.completion.chunk", "chat.completion":
		return true
	default:
		return false
	}
}

// billingKey 计费键：Response.ID 优先；无 ID 时以 author|model|prompt 定位
// round（prompt 变化即新调用，同 accumulateRound 启发式）。
func billingKey(e *trpcevent.Event) string {
	if id := strings.TrimSpace(e.Response.ID); id != "" {
		return "id:" + id
	}
	author := strings.TrimSpace(e.Author)
	model := strings.TrimSpace(e.Response.Model)
	return "rnd:" + author + "|" + model + "|" + strconv.Itoa(e.Response.Usage.PromptTokens)
}

// observe 消费一条框架事件；无 usage、非 chat.completion 对象或零 token 早退。
func (a *graphUsageAccumulator) observe(e *trpcevent.Event) {
	if a == nil || e == nil || e.Response == nil || e.Response.Usage == nil {
		return
	}
	rsp := e.Response
	if !usageObjectGate(rsp.Object) {
		return
	}
	prompt, completion := rsp.Usage.PromptTokens, rsp.Usage.CompletionTokens
	if prompt <= 0 && completion <= 0 {
		return
	}
	key := billingKey(e)
	a.mu.Lock()
	cu := a.byCall[key]
	if cu == nil {
		cu = &callUsage{model: strings.TrimSpace(rsp.Model)}
		a.byCall[key] = cu
	}
	// 调用内累计取 MAX（流式块 usage 累计上报，prompt 恒定、completion 增长）。
	if prompt > cu.prompt {
		cu.prompt = prompt
	}
	if completion > cu.completion {
		cu.completion = completion
	}
	a.mu.Unlock()
}

// flush 流结束时按模型聚合并落账。status 为运行终态
// （success/failed/cancelled/interrupted）。best-effort：每个模型独立落账，
// 单条失败不影响其余；失败仅 Warn 日志。
func (a *graphUsageAccumulator) flush(ctx context.Context, status string) {
	if a == nil || a.rec == nil {
		return
	}
	a.mu.Lock()
	// 聚合：计费键 → model 桶。
	byModel := map[string]*modelTokenSum{}
	for _, cu := range a.byCall {
		if cu.prompt <= 0 && cu.completion <= 0 {
			continue
		}
		sum := byModel[cu.model]
		if sum == nil {
			sum = &modelTokenSum{}
			byModel[cu.model] = sum
		}
		sum.prompt += cu.prompt
		sum.completion += cu.completion
		sum.calls++
	}
	a.mu.Unlock()
	if len(byModel) == 0 {
		return
	}
	// 落账用独立 deadline：执行 ctx 可能在长运行后临近取消（与既有
	// recordChatIngressUsage / team 落账同一模式）。
	for model, sum := range byModel {
		if sum.prompt <= 0 && sum.completion <= 0 {
			continue
		}
		in := biz.GraphRunUsageInput{
			SessionID:     a.sessionID,
			RunID:         a.execID,
			GraphID:       a.graphID,
			Model:         model,
			PromptTok:     sum.prompt,
			CompletionTok: sum.completion,
			Status:        status,
			LLMCalls:      sum.calls,
			UsageSource:   "streaming",
		}
		recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		err := a.rec.RecordGraphRunUsage(recCtx, in)
		cancel()
		if err != nil {
			a.lg.Warn("图运行用量落账失败",
				loggateway.StepID("graph.usage_record_fail"),
				loggateway.Err(err),
				loggateway.Str("session_id", a.sessionID),
				loggateway.Str("execution_id", a.execID),
				loggateway.Str("graph_id", a.graphID),
				loggateway.Str("model", model),
			)
		}
	}
}

// modelTokenSum 一个模型的聚合落账数据。
type modelTokenSum struct {
	prompt     int
	completion int
	calls      int
}
