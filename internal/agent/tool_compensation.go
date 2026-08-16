package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools/inverse"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 终态补偿跟踪器（P0-1，Cordis H3 补偿对的运行时兜底形态）。
//
// 背景：2026-08-15/16 两轮 ops 事故的根因层是「inject 后必须 clear」的补偿纪律
// 只存在于提示词层，模型可无视，只能靠 loop guard 拦截兜底。本跟踪器把补偿对
// 下沉为运行时不变量：声明了逆工具的工具调用成功后记入 pending，对应逆工具
// 调用成功后核销；pending 超过 compensateAlertAfter 未核销 → 一次性告警
// （ops.compensation_pending）。
//
// 语义说明（实现期对 report-05 P0-1 的细化）：终态校验不依赖「run 结束」的
// 精确定时（graph 节点 invocation 的父子链接在框架内不稳定），而采用
// 「补偿超时」语义——pending 挂起超时即告警。该语义等价覆盖 run 终态场景
// （run 结束后未核销的项必然超时），且额外捕获 run 卡死（HITL 长挂起）场景；
// 对正常 drill 流程（inject→clear 分钟级完成）无误报。
//
// 默认仅观测、不动作：不产生任何自动工具执行（高危工具审批铁律），
// 自动补偿（三级处置）留待后续显式预授权设计。
//
// 作用域：按 session 聚合（同会话内跨 invocation 的 inject→clear 视为有效
// 补偿）；取不到 session 时降级为 invocation。进程级单例——agent 重建不得
// 丢失 pending 状态。
//
// 已知边界：仅跟踪调用成功（AfterTool 无 error）的正向操作；「调用报错但
// 副作用已落」（网络超时等）不在覆盖范围。
const (
	compensateAlertAfter    = 30 * time.Minute // pending 超过该时长未核销 → 告警（一次性）
	compensateSweepInterval = time.Minute
	compensateJournalMax    = 1024 // 单 scope pending 上限，防异常堆积
)

// pendingCompensation 是一条待核销的副作用记录。
type pendingCompensation struct {
	forwardTool string    // 产生副作用的工具（如 gns3_fault_inject）
	inverseTool string    // 应执行的逆工具（如 gns3_fault_clear）
	argsDigest  string    // 正向参数摘要（告警载荷，截断防爆）
	mappedArgs  []byte    // 推导的逆操作参数（供后续自动补偿/人工执行参考）
	agentName   string    // 记录时的 agent 名（定位节点）
	at          time.Time // 副作用产生时间
	alerted     bool      // 是否已告警（一次性）
}

// compensationTracker 是进程级补偿跟踪器。
type compensationTracker struct {
	mu         sync.Mutex
	journals   map[string]map[string]*pendingCompensation // scopeKey → matchKey → entry
	lg         loggateway.Logger
	alertAfter time.Duration
	sweepEvery time.Duration
	startOnce  sync.Once
}

var globalCompensationTracker = newCompensationTracker(loggateway.NewNoop())

func newCompensationTracker(lg loggateway.Logger) *compensationTracker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &compensationTracker{
		journals:   map[string]map[string]*pendingCompensation{},
		lg:         lg,
		alertAfter: compensateAlertAfter,
		sweepEvery: compensateSweepInterval,
	}
}

// setLogger 注入日志出口（首次装配回调链时调用；agent 重建重复设置同值无害，
// 保证告警不因全局单例的 noop 初始化而丢失）。
func (t *compensationTracker) setLogger(lg loggateway.Logger) {
	if lg == nil {
		return
	}
	t.mu.Lock()
	t.lg = lg
	t.mu.Unlock()
}

func (t *compensationTracker) logger() loggateway.Logger {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lg
}

// compensationScopeKey 取跟踪作用域：优先 session（跨 invocation 补偿有效），
// 降级 invocation。取不到返回空串（跟踪器 fail-open，不影响工具执行）。
func compensationScopeKey(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil && inv.Session.ID != "" {
		return "sess|" + inv.Session.ID
	}
	if inv.InvocationID != "" {
		return "inv|" + inv.InvocationID
	}
	return ""
}

// compensationArgsDigest 提取参数的可读摘要（告警载荷用），截断防超大参数。
func compensationArgsDigest(args []byte) string {
	text := string(args)
	if len(text) > 512 {
		text = text[:512] + "…"
	}
	return text
}

func (t *compensationTracker) afterTool(ctx context.Context, args *trpctool.AfterToolArgs) {
	if args == nil || args.Error != nil {
		return // 失败调用的副作用治理归熔断器/重试路径，不在本跟踪范围
	}
	scope := compensationScopeKey(ctx)
	if scope == "" {
		return
	}
	if spec, ok := inverse.LookupForward(args.ToolName); ok {
		mapped := args.Arguments
		if spec.MapArgs != nil {
			m, err := spec.MapArgs(args.Arguments)
			if err != nil {
				t.logger().Warn("补偿参数映射失败，跳过跟踪",
					loggateway.StepID("ops.compensation_track"),
					loggateway.Str("tool", args.ToolName),
					loggateway.Err(err))
				return
			}
			mapped = m
		}
		var agentName string
		if inv, invOK := trpcagent.InvocationFromContext(ctx); invOK && inv != nil {
			agentName = inv.AgentName
		}
		t.addPending(scope, args.ToolName, spec.InverseTool, args.Arguments, mapped, agentName)
		return
	}
	if inverse.IsInverse(args.ToolName) {
		t.settle(scope, args.ToolName, args.Arguments)
	}
}

func (t *compensationTracker) addPending(scope, forwardTool, inverseTool string, args, mapped []byte, agentName string) {
	mk := loopGuardSignature(inverseTool, mapped) // matchKey：逆工具名 + 规范化逆参数
	now := time.Now()
	overflow := false
	t.mu.Lock()
	j, ok := t.journals[scope]
	if !ok {
		j = map[string]*pendingCompensation{}
		t.journals[scope] = j
	}
	if len(j) >= compensateJournalMax {
		// 超限丢弃最旧一条并告警（异常堆积本身是运维信号）。
		var oldestKey string
		var oldestAt time.Time
		for k, p := range j {
			if oldestKey == "" || p.at.Before(oldestAt) {
				oldestKey, oldestAt = k, p.at
			}
		}
		delete(j, oldestKey)
		overflow = true
	}
	j[mk] = &pendingCompensation{
		forwardTool: forwardTool,
		inverseTool: inverseTool,
		argsDigest:  compensationArgsDigest(args),
		mappedArgs:  mapped,
		agentName:   agentName,
		at:          now,
	}
	t.mu.Unlock()
	if overflow {
		t.logger().Warn("补偿跟踪 journal 超限，丢弃最旧 pending 项",
			loggateway.StepID("ops.compensation_overflow"),
			loggateway.Str("scope", scope),
			loggateway.Int("cap", compensateJournalMax))
	}
}

// settle 核销：逆工具调用成功，移除对应 pending 项。无匹配项为 no-op
// （单独调用 fault_clear 合法——可能清除的是手工/上一轮注入的故障）。
func (t *compensationTracker) settle(scope, inverseTool string, args []byte) {
	mk := loopGuardSignature(inverseTool, args)
	t.mu.Lock()
	defer t.mu.Unlock()
	j, ok := t.journals[scope]
	if !ok {
		return
	}
	delete(j, mk)
	if len(j) == 0 {
		delete(t.journals, scope)
	}
}

// ensureSweeper 惰性启动清扫协程（进程级一次）。仅在生产装配路径
// （compensationTrackerAfterHook）调用——单测直接驱动 sweep()，不产协程，
// 避免 goleak 误报；sweepLoop 独立命名方法保证 goleak 豁免名单稳定。
func (t *compensationTracker) ensureSweeper() {
	t.startOnce.Do(func() {
		safego.Go(context.Background(), "agent.compensation_sweeper", t.sweepLoop)
	})
}

func (t *compensationTracker) sweepLoop() {
	ticker := time.NewTicker(t.sweepEvery)
	defer ticker.Stop()
	for range ticker.C {
		t.sweep()
	}
}

// sweep 扫描全部 journal，对超时未核销的 pending 项做一次性告警。
func (t *compensationTracker) sweep() {
	now := time.Now()
	type alertItem struct {
		scope string
		p     *pendingCompensation
	}
	var alerts []alertItem
	t.mu.Lock()
	for scope, j := range t.journals {
		for _, p := range j {
			if p.alerted || now.Sub(p.at) < t.alertAfter {
				continue
			}
			p.alerted = true
			alerts = append(alerts, alertItem{scope: scope, p: p})
		}
	}
	t.mu.Unlock()
	for _, a := range alerts {
		// 告警即全部处置（一级）：内容自包含，值班同学凭此载荷人工核销。
		t.logger().Warn("检测到未补偿的副作用操作（超时报未核销）",
			loggateway.StepID("ops.compensation_pending"),
			loggateway.Str("scope", a.scope),
			loggateway.Str("forward_tool", a.p.forwardTool),
			loggateway.Str("inverse_tool", a.p.inverseTool),
			loggateway.Str("forward_args", a.p.argsDigest),
			loggateway.Str("inverse_args", string(a.p.mappedArgs)),
			loggateway.Str("agent", a.p.agentName),
			loggateway.Str("pending_since", a.p.at.Format(time.RFC3339)))
	}
}

// compensationTrackerAfterHook 把进程级跟踪器接入工具回调链。
// 无条件注册（ToolsEnabled 块内）：无声明逆工具的普通工具仅两次 map 查找。
// lg 注入全局单例（首装时），避免告警落入 noop。
func compensationTrackerAfterHook(lg loggateway.Logger) callbacks.AfterToolHook {
	tracker := globalCompensationTracker
	tracker.setLogger(lg)
	tracker.ensureSweeper()
	return callbacks.NewAfterToolHook(60, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		tracker.afterTool(ctx, args)
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}
