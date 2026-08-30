package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

var dangerousCommands = []string{"rm -rf", "drop table", "format c:"}

// defaultBlockedMessage 是拦截兜底文案（P4 标记剥离，2026-08-30）：用户可见
// 文本不得暴露插件名/命中模式等内部细节（S14 曾把
// "output_policy: blocked content matching dangerous_command" 原样回给用户），
// 技术细节只进日志、决策记录与 flowlog。
const defaultBlockedMessage = "很抱歉，该内容因安全策略无法展示。如需了解相关概念或风险防范知识，可以换一种问法继续交流。"

// defensiveMarkers 是讲解/防御语境标记（P4 防御性提问白名单）：危险命令串
// 伴随这些语义出现、且不在代码块/shell 提示符行内时，视为科普/警示内容放行。
var defensiveMarkers = []string{
	"危险", "切勿", "请勿", "不要", "千万", "避免", "防范", "危害", "风险",
	"什么是", "为什么", "科普", "解释", "说明", "慎用", "注意",
	"dangerous", "never run", "do not run", "what is", "why", "avoid", "risk",
}

// defensiveContextWindow 是流式路径防御语境判定的滚动窗口长度（字符）。
// 防御标记（如「千万不要执行」）通常先于危险命令若干 chunk 到达，窗口需
// 覆盖这一间距；窗口之外的旧文本不再参与判定，自然保守回收。
const defensiveContextWindow = 240

type outputPolicyConfig struct {
	BlockedPatterns       []string `json:"blocked_patterns"`
	DangerousCommandCheck bool     `json:"dangerous_command_check"`
	BlockOnViolation      bool     `json:"block_on_violation"`
	ReplacementMessage    string   `json:"replacement_message"`
}

// streamContext 是按 invocation 维护的流式输出滚动窗口（P4）：危险命令的
// 防御语境判定需要跨 chunk 的上下文（防御标记往往先于命令串若干 chunk
// 到达），窗口在 afterModel 逐 chunk 追加、流结束由 clearStreamState 清理、
// 触发拦截后重置（已拦截内容不再影响后续判定，防同一命令在窗口内反复命中）。
type streamContext struct {
	mu  sync.Mutex
	buf string
}

func (s *streamContext) append(text string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf += text
	if len(s.buf) > defensiveContextWindow {
		s.buf = s.buf[len(s.buf)-defensiveContextWindow:]
	}
	return s.buf
}

func (s *streamContext) current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf
}

func (s *streamContext) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = ""
}

type OutputPolicyPlugin struct {
	base basePlugin
	cfg  outputPolicyConfig
	// eventCounts 按采样 key 计数（key 集合有界），用于高频 chunk 日志节流。
	eventCounts sync.Map // string -> *atomic.Int64
	// blockEmitted 是决策记录去重表（P1-④）：流式路径每个违规 chunk 都
	// 回调 emitBlockGate，同一 invocation 同一 pattern 只写一条
	// decision_records。key=invocationID|pattern，流结束由
	// clearBlockGateDedup 清理，防无界增长。
	blockEmitted sync.Map // string -> struct{}
	// streamCtxs 按 invocationID 维护流式滚动窗口（P4 防御性白名单的判定
	// 上下文），与 blockEmitted 同生命周期（流结束清理）。
	streamCtxs sync.Map // string -> *streamContext
	// decisions 是 M80 决策 collector 的 getter（P1-④）：每次发射现取，
	// 免疫注入时序。nil = 决策记录降级。monitorBus 供 flowlog 兜底 emitter
	// （插件回调 ctx 不带 turn TraceEmitter 时系统域补发）。
	decisions  func() decision.Collector
	monitorBus contract.MonitorBus
}

var _ trpcplugin.Plugin = (*OutputPolicyPlugin)(nil)

func NewOutputPolicyPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, lg loggateway.Logger, decisions func() decision.Collector) *OutputPolicyPlugin {
	var cfg outputPolicyConfig
	cfg.DangerousCommandCheck = true
	cfg.BlockOnViolation = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	return &OutputPolicyPlugin{
		base:       newBasePlugin(p.Key, stats, monitorBus, lg),
		cfg:        cfg,
		decisions:  decisions,
		monitorBus: monitorBus,
	}
}

func (o *OutputPolicyPlugin) Name() string { return o.base.Name() }

func (o *OutputPolicyPlugin) Register(r *trpcplugin.Registry) {
	r.AfterModel(o.afterModel)
	r.OnEvent(o.onEvent)
}

func (o *OutputPolicyPlugin) afterModel(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
	if args == nil || args.Response == nil {
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	}
	text := responseText(args.Response)
	// P4：流式 chunk 的防御语境判定需要跨 chunk 上下文——把 chunk 追加进
	// 本 invocation 的滚动窗口，危险命令白名单基于窗口判定（严格
	// blocked_patterns 仍只看当前 chunk，维持原有语义）。
	invID := invocationIDFromContext(ctx)
	isChunk := args.Response.Object == trpcmodel.ObjectTypeChatCompletionChunk
	ctxText := text
	if isChunk && invID != "" && text != "" {
		ctxText = o.streamWindow(invID).append(text)
	}
	if viol, pat := o.violationWithContext(text, ctxText); viol {
		o.base.logger.Info("plugin.output_policy.after_model", "status", "blocked", "pattern", pat, "block_on_violation", o.cfg.BlockOnViolation)
		o.base.recordEvent(ctx, "after_model", "blocked",
			fmt.Sprintf("模型输出命中阻断策略（pattern=%s）", pat))
		o.emitBlockGate(ctx, "after_model", pat)
		if o.cfg.BlockOnViolation {
			if invID != "" {
				if sc := o.peekStreamContext(invID); sc != nil {
					sc.reset()
				}
			}
			return &trpcmodel.AfterModelResult{
				Context:        ctx,
				CustomResponse: blockedModelResponse(o.replacementMessage()),
			}, nil
		}
	} else if isChunk {
		// 干净 chunk 是高频洪泛点（框架对每个 chunk 回调一次，实测 8287
		// 条/4min）：采样节流。违规安全检查本身不受节流影响（每个 chunk
		// 仍做 violation 匹配）；blocked 走上方分支逐条记录。
		v, _ := o.eventCounts.LoadOrStore("after_model:chunk_ok", &atomic.Int64{})
		n := v.(*atomic.Int64).Add(1)
		if n == 1 || n%auditEventSampleInterval == 0 {
			o.base.logger.Info("plugin.output_policy.after_model", "status", "ok", "count", n, "sampled", true)
		}
	} else {
		o.base.logger.Info("plugin.output_policy.after_model", "status", "ok")
	}
	o.base.record(ctx, "after_model", "ok")
	return &trpcmodel.AfterModelResult{Context: ctx}, nil
}

func (o *OutputPolicyPlugin) onEvent(
	ctx context.Context,
	inv *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	if e == nil || e.Response == nil {
		return e, nil
	}
	defer o.clearStreamState(ctx, e.Response)
	text := eventText(e)
	if text == "" {
		return e, nil
	}
	// P4：危险命令白名单基于滚动窗口判定（窗口由 afterModel 维护，通常已
	// 含当前 chunk；未过 afterModel 的事件兜底拼上当前文本）。
	ctxText := text
	if inv != nil && inv.InvocationID != "" {
		if sc := o.peekStreamContext(inv.InvocationID); sc != nil {
			ctxText = sc.current() + text
		}
	}
	viol, pat := o.violationWithContext(text, ctxText)
	if !viol {
		return e, nil
	}
	o.base.recordEvent(ctx, "on_event", "blocked",
		fmt.Sprintf("流式输出命中阻断策略（pattern=%s）", pat))
	o.base.logger.Warn("output_policy.event_blocked", "plugin", o.base.name, "pattern", pat, "block_on_violation", o.cfg.BlockOnViolation)
	// TPM-P1-04: actually enforce block_on_violation in streaming path. Previously
	// the event passed through unchanged — admin's block_on_violation=true was a no-op
	// for OnEvent (only afterModel honored it). Now we splice the chunk content with
	// the replacement message so the violating fragment never reaches the client.
	if !o.cfg.BlockOnViolation {
		return e, nil
	}
	msg := strings.TrimSpace(o.cfg.ReplacementMessage)
	if msg == "" {
		msg = "output_policy: blocked content matching " + pat
	}
	for i := range e.Response.Choices {
		ch := &e.Response.Choices[i]
		ch.Message.Content = msg
		ch.Delta.Content = msg
		if ch.FinishReason == nil {
			reason := "content_filter"
			ch.FinishReason = &reason
		}
	}
	return e, nil
}

// violation 是严格判定（无上下文），供非流式/无窗口场景使用。
func (o *OutputPolicyPlugin) violation(text string) (bool, string) {
	return o.violationWithContext(text, text)
}

// violationWithContext 判定违规：text 是当前输出（chunk），ctxText 是判定
// 上下文（流式路径为滚动窗口，含当前 chunk；非流式即 text 本身）。
// blocked_patterns 是管理员的显式严格清单，只看当前 text 不做豁免；
// 内建危险命令检查适用防御性白名单（讲解/警示语境放行）。
func (o *OutputPolicyPlugin) violationWithContext(text, ctxText string) (bool, string) {
	if containsAny(text, o.cfg.BlockedPatterns) {
		for _, p := range o.cfg.BlockedPatterns {
			if strings.TrimSpace(p) != "" && strings.Contains(strings.ToLower(text), strings.ToLower(p)) {
				return true, p
			}
		}
		return true, "blocked_patterns"
	}
	if o.cfg.DangerousCommandCheck {
		for _, cmd := range dangerousCommands {
			if dangerousCommandActionable(ctxText, cmd) {
				return true, "dangerous_command"
			}
		}
	}
	return false, ""
}

// dangerousCommandActionable 判定危险命令是否应拦截（P4 白名单）：
// 任一命中为可执行形态（代码块/shell 提示符行）=> 拦截；
// 全部命中均为散文提及且上下文带防御/讲解标记 => 放行；
// 无防御标记 => 拦截（保守方向）。
// 必须遍历全部命中：只查首个命中会让「讲解 + 代码块示范」的混合内容逃逸。
// S14 h3 实证：模型回答「rm -rf 为什么危险」类防御性提问被连续误拦 5 次。
func dangerousCommandActionable(ctxText, cmd string) bool {
	lower := strings.ToLower(ctxText)
	hasDefensive := false
	for _, m := range defensiveMarkers {
		if strings.Contains(lower, m) {
			hasDefensive = true
			break
		}
	}
	found := false
	searchFrom := 0
	for {
		rel := strings.Index(lower[searchFrom:], cmd)
		if rel < 0 {
			break
		}
		idx := searchFrom + rel
		found = true
		if insideCodeFence(ctxText, idx) || onPromptLine(ctxText, idx) {
			return true
		}
		searchFrom = idx + len(cmd)
	}
	if !found {
		return false
	}
	return !hasDefensive
}

// insideCodeFence 以 ``` 围栏奇偶判定命中位置是否在代码块内（滚动窗口上的
// 近似判定：窗口起点可能切在块中，奇数即按块内处理，方向保守）。
func insideCodeFence(text string, idx int) bool {
	return strings.Count(text[:idx], "```")%2 == 1
}

// onPromptLine 判定命中位置所在行是否以 shell 提示符（$/#/>）开头。
func onPromptLine(text string, idx int) bool {
	lineStart := strings.LastIndexByte(text[:idx], '\n') + 1 // 无换行时为 0
	line := strings.TrimLeft(text[lineStart:idx], " \t")
	return strings.HasPrefix(line, "$") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">")
}

func (o *OutputPolicyPlugin) replacementMessage() string {
	if msg := strings.TrimSpace(o.cfg.ReplacementMessage); msg != "" {
		return msg
	}
	return defaultBlockedMessage
}

func (o *OutputPolicyPlugin) streamWindow(invID string) *streamContext {
	v, _ := o.streamCtxs.LoadOrStore(invID, &streamContext{})
	return v.(*streamContext)
}

func (o *OutputPolicyPlugin) peekStreamContext(invID string) *streamContext {
	if v, ok := o.streamCtxs.Load(invID); ok {
		return v.(*streamContext)
	}
	return nil
}

func invocationIDFromContext(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
		return inv.InvocationID
	}
	return ""
}

// emitBlockGate 把一次输出安全拦截双写为 system_guard 决策记录 + flowlog
// （P1-④，2026-08-30）：此前仅 plugin_runs 统计 + 进程日志，run 结束后
// decision_records 零行、三方互证断链（S14 实测）。流式路径每个违规 chunk
// 都回调，决策记录按 invocation|pattern 去重；flowlog 优先用 ctx 自带
// turn emitter，缺失时经 monitorBus 建系统域兜底（run 外 ctx 不断链）。
func (o *OutputPolicyPlugin) emitBlockGate(ctx context.Context, phase, pat string) {
	var inv *trpcagent.Invocation
	if i, ok := trpcagent.InvocationFromContext(ctx); ok {
		inv = i
	}
	if inv != nil && inv.InvocationID != "" {
		if _, loaded := o.blockEmitted.LoadOrStore(inv.InvocationID+"|"+pat, struct{}{}); loaded {
			return
		}
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		sessionID, _ = sessionAgentKey(ctx, inv)
	}
	gd := decision.GateDecision{
		TriggerRule: decision.TriggerOutputPolicyBlocked,
		Outcome:     "blocked",
		Scenario:    "模型输出命中输出安全策略",
		Reasoning:   fmt.Sprintf("phase=%s pattern=%s block_on_violation=%v", phase, pat, o.cfg.BlockOnViolation),
		GuardName:   "output_policy",
		RunID:       decision.GateRunIDFromContext(ctx),
		SessionID:   sessionID,
		Action:      "block",
		Extra:       map[string]any{"phase": phase, "pattern": pat},
	}
	var collector decision.Collector
	if o.decisions != nil {
		collector = o.decisions()
	}
	if collector != nil {
		decision.EmitGate(ctx, collector, gd)
	}
	if event.TraceEmitterFromContext(ctx) != nil {
		event.LogGateFlow(ctx, gd.TriggerRule, gd.Outcome, gd.Scenario, gd.Reasoning)
		return
	}
	if o.monitorBus == nil {
		return
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainSystem,
		LG:        o.base.lg,
		Infra:     event.NewInfraFromBus(o.monitorBus),
	})
	event.LogGateFlow(event.WithTraceEmitter(ctx, flow), gd.TriggerRule, gd.Outcome, gd.Scenario, gd.Reasoning)
}

// clearStreamState 在流结束（任一 choice 带 FinishReason）时清理该
// invocation 的决策去重键与滚动窗口，防 blockEmitted/streamCtxs 随调用数
// 无界增长。
func (o *OutputPolicyPlugin) clearStreamState(ctx context.Context, resp *trpcmodel.Response) {
	if resp == nil {
		return
	}
	finished := false
	for _, ch := range resp.Choices {
		if ch.FinishReason != nil {
			finished = true
			break
		}
	}
	if !finished {
		return
	}
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.InvocationID != "" {
		o.streamCtxs.Delete(inv.InvocationID)
		o.blockEmitted.Range(func(k, _ any) bool {
			if ks, ok := k.(string); ok && strings.HasPrefix(ks, inv.InvocationID+"|") {
				o.blockEmitted.Delete(k)
			}
			return true
		})
	}
}

func eventText(e *trpcevent.Event) string {
	if e == nil || e.Response == nil {
		return ""
	}
	var b strings.Builder
	for _, ch := range e.Response.Choices {
		if ch.Delta.Content != "" {
			b.WriteString(ch.Delta.Content)
		} else if ch.Message.Content != "" {
			b.WriteString(ch.Message.Content)
		}
	}
	return b.String()
}
