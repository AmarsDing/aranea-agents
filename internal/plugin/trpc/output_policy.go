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

type outputPolicyConfig struct {
	BlockedPatterns       []string `json:"blocked_patterns"`
	DangerousCommandCheck bool     `json:"dangerous_command_check"`
	BlockOnViolation      bool     `json:"block_on_violation"`
	ReplacementMessage    string   `json:"replacement_message"`
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
	if viol, pat := o.violation(text); viol {
		o.base.logger.Info("plugin.output_policy.after_model", "status", "blocked", "pattern", pat, "block_on_violation", o.cfg.BlockOnViolation)
		o.base.recordEvent(ctx, "after_model", "blocked",
			fmt.Sprintf("模型输出命中阻断策略（pattern=%s）", pat))
		o.emitBlockGate(ctx, "after_model", pat)
		if o.cfg.BlockOnViolation {
			msg := strings.TrimSpace(o.cfg.ReplacementMessage)
			if msg == "" {
				msg = "output_policy: blocked content matching " + pat
			}
			return &trpcmodel.AfterModelResult{
				Context:        ctx,
				CustomResponse: blockedModelResponse(msg),
			}, nil
		}
	} else if args.Response.Object == trpcmodel.ObjectTypeChatCompletionChunk {
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
	defer o.clearBlockGateDedup(ctx, e.Response)
	text := eventText(e)
	if text == "" {
		return e, nil
	}
	viol, pat := o.violation(text)
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

func (o *OutputPolicyPlugin) violation(text string) (bool, string) {
	if containsAny(text, o.cfg.BlockedPatterns) {
		for _, p := range o.cfg.BlockedPatterns {
			if strings.TrimSpace(p) != "" && strings.Contains(strings.ToLower(text), strings.ToLower(p)) {
				return true, p
			}
		}
		return true, "blocked_patterns"
	}
	if o.cfg.DangerousCommandCheck {
		if containsAny(text, dangerousCommands) {
			return true, "dangerous_command"
		}
	}
	return false, ""
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

// clearBlockGateDedup 在流结束（任一 choice 带 FinishReason）时清理该
// invocation 的去重键，防 blockEmitted 随调用数无界增长。
func (o *OutputPolicyPlugin) clearBlockGateDedup(ctx context.Context, resp *trpcmodel.Response) {
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
