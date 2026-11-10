package agent

// D6 L-2 脱敏闸（E 阶段 W2b，需求 §7.2/§7.3 验收 #3/#4）：云通道必经的
// BeforeModel 钩子。语义自 workbench `server/llm.go` chat 流程平移：
//
//	水位线 >= 阈值 → 强制本地（route_local_forced 审计后直接放行，本地
//	    通道无出网暴露，无需脱敏）；
//	否则一级 mask（规则驱动）→ secondCheck 二级检测；
//	    findings 非空 → mask_reject + fail-closed 拒绝上云（用原文的降级
//	    只发生在 wb 的网关双通道里；aranea 的通道选择由 L-1 selector 在
//	    选型期完成，BeforeModel 时模型已定，无法中途换本地 → 改为硬拒，
//	    未上云即安全。W4 日志登记该差异）；
//	引擎不可用 → mask_engine_unavailable + fail-closed 拒绝上云（同上）；
//	全过 → cloud_send 审计后放行（脱敏后的消息）。
//
// 通道判定（顺序）：
//  1. 本步 selector 已裁决（invocation state，见 model_classification.go
//     的 SecopsRouteState）——强制本地/已知本地直接放行；
//  2. 未裁决（水位线低于阈值等）→ 按框架已解析的 inv.Model 名查模型目录
//     的 channel_class（TTL 缓存；同名不同通道视为歧义，保守按云）；
//  3. 仍未裁决（模型不在目录/目录读失败）→ 保守按云处理（脱敏 + 二级检测）。
//
// F6（仅文本上云）在 aranea 的落地：图片/音频等非文本 part 原样保留（产品
// 支持多模态，见代码内注释与 W4 登记），只对文本 Content 与文本 part 做脱敏
// 与复扫。工具调用参数（ToolCalls）不在 V3 面内（改写 JSON 存在破坏风险，
// W4 登记为后续项）。

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// maskGateHookPriority 与 intentReorder 同带（100），在 callback_chain.go
// 中追加于其后（同层同优先级按注册序执行）——它是模型调用前最后一道
// 消息改写，必须看到行将发出的最终消息。
const maskGateHookPriority = 100

// SecopsMaskGate 是 L-2 钩子消费的脱敏门窄口（*biz.SecopsMaskUsecase 满足；
// 窄接口便于测试替身）。
type SecopsMaskGate interface {
	// Engine 返回脱敏引擎热副本；nil = 引擎不可用（调用方 fail-closed）。
	Engine(ctx context.Context) *biz.MaskEngine
	// RecordMaskAudit 严格审计写口（wb F3：失败即拒）。
	RecordMaskAudit(ctx context.Context, action, scope string, detail map[string]any, severity string) error
}

// secopsMaskGateHolder 包装脱敏门——atomic.Value 不允许 Store 未包装的
// nil 接口值，停用态也需要可存储（测试/降级均要求把门清空）。
type secopsMaskGateHolder struct{ gate SecopsMaskGate }

// secopsMaskDefaultGate 进程级脱敏门（nil 门 = 停用，钩子不注册）。
// 与 ClassificationGate 同因走进程级 atomic.Value：agent 构建被缓存且跨
// workspace 共享，装配期一次性注册（wire 副作用），构建缓存安全。
var secopsMaskDefaultGate atomic.Value // stores secopsMaskGateHolder

// SetDefaultSecopsMaskGate 注册进程级脱敏门。wire 装配期由
// provideSecopsMaskUsecase 副作用调用一次；nil = 停用（钩子不注册）。
func SetDefaultSecopsMaskGate(g SecopsMaskGate) {
	secopsMaskDefaultGate.Store(secopsMaskGateHolder{gate: g})
}

// DefaultSecopsMaskGate 读进程级脱敏门；未注册/已停用 → nil。
func DefaultSecopsMaskGate() SecopsMaskGate {
	v := secopsMaskDefaultGate.Load()
	if v == nil {
		return nil
	}
	if h, ok := v.(secopsMaskGateHolder); ok {
		return h.gate
	}
	return nil
}

// maskChannelCacheTTL 是「模型名 → 通道类别」目录缓存 TTL。
const maskChannelCacheTTL = 60 * time.Second

// maskChannelResolver 是「框架已解析模型名 → channel_class」的内存目录，
// 为 selector 未裁决的步骤（水位线低于阈值等常态路径）提供通道判定。
// 同名模型在不同 provider 下通道不一致时视为歧义，按未裁决（云保守）处理。
type maskChannelResolver struct {
	catalog biz.TeamModelCatalog
	lg      loggateway.Logger
	now     func() time.Time

	mu          sync.Mutex
	cache       map[string]string
	refreshedAt time.Time
}

func newMaskChannelResolver(catalog biz.TeamModelCatalog, lg loggateway.Logger) *maskChannelResolver {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &maskChannelResolver{catalog: catalog, lg: lg, now: time.Now}
}

// ChannelFor 返回模型名对应的通道类别；"" = 未裁决（调用方按云保守）。
func (r *maskChannelResolver) ChannelFor(ctx context.Context, modelName string) string {
	name := strings.TrimSpace(modelName)
	if r.catalog == nil || name == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.cache == nil || now.Sub(r.refreshedAt) >= maskChannelCacheTTL {
		models, err := r.catalog.List(ctx)
		if err != nil {
			r.lg.Warn("脱敏通道解析：模型目录读取失败，沿用上次快照",
				loggateway.StepID("agent.mask_channel_catalog_fail"),
				loggateway.Bool("cold", r.cache == nil),
				loggateway.Err(err))
			if r.cache == nil {
				return ""
			}
		} else {
			cache := make(map[string]string, len(models))
			for _, pm := range models {
				model := strings.TrimSpace(pm.Model)
				if model == "" {
					continue
				}
				ch := biz.ProviderChannelClassCloud
				if biz.ProviderChannelForcesLocal(pm.ChannelClass) {
					ch = biz.ProviderChannelClassLocal
				}
				if prev, ok := cache[model]; ok && prev != ch {
					cache[model] = "" // 同名不同通道：歧义 → 未裁决
					continue
				}
				cache[model] = ch
			}
			r.cache = cache
			r.refreshedAt = now
		}
	}
	return r.cache[name]
}

// invocationModelName 取本步框架已解析模型名（llmflow 在 BeforeModel 回调
// 期间 invocation.Model = 本步所选模型，v1.11.2 runOneStep 已核实）。
func invocationModelName(inv *trpcagent.Invocation) string {
	if inv == nil || inv.Model == nil {
		return ""
	}
	return strings.TrimSpace(inv.Model.Info().Name)
}

// newMaskGateBeforeHook 返回 D6 L-2 脱敏闸 BeforeModel 钩子。门未装配
// （进程级 nil）时返回 nil 不注册——与其余条件装配 hook 同口径。
func newMaskGateBeforeHook(deps TRPCBuilderDeps) callbacks.Callback {
	gate := DefaultSecopsMaskGate()
	if gate == nil {
		return nil
	}
	resolver := newMaskChannelResolver(deps.ModelCatalog, deps.Logger())
	lg := deps.Logger()
	return callbacks.NewBeforeModelHook(maskGateHookPriority, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		pass := &trpcmodel.BeforeModelResult{Context: ctx}
		if args == nil || args.Request == nil {
			return pass, nil
		}
		inv, _ := trpcagent.InvocationFromContext(ctx)
		state := SecopsRouteStateFromInvocation(inv)

		sessionID := decision.GateSessionIDFromContext(ctx)
		if sessionID == "" && inv != nil && inv.Session != nil {
			sessionID = strings.TrimSpace(inv.Session.ID)
		}
		runID := decision.GateRunIDFromContext(ctx)
		scope := firstNonEmptyString(sessionID, runID)
		if scope == "" && inv != nil {
			scope = strings.TrimSpace(inv.InvocationID)
		}

		// 1. 本地通道：强制本地时只落 route_local_forced 审计后放行。
		if state.Channel == biz.ProviderChannelClassLocal {
			if !state.Forced {
				return pass, nil
			}
			metrics.SecopsMaskEventTotal.WithLabelValues(decision.TriggerRouteLocalForced).Inc()
			err := emitSecopsMaskDecision(ctx, deps, gate,
				decision.GateDecision{
					TriggerRule:   decision.TriggerRouteLocalForced,
					Outcome:       "proceeded",
					Scenario:      "密级路由强制本地通道",
					Reasoning:     "会话水位线达到本地阈值，本步走本地通道（无出网暴露，跳过脱敏）",
					GuardName:     "secops_mask",
					RunID:         runID,
					SessionID:     sessionID,
					ObservedValue: state.Ceiling,
					Threshold:     state.Threshold,
					Action:        "route_local",
					Extra:         map[string]any{"channel": biz.ProviderChannelClassLocal},
				},
				biz.SecopsMaskActionRouteLocalForced, scope,
				map[string]any{
					"ceiling":   state.Ceiling,
					"threshold": state.Threshold,
					"channel":   biz.ProviderChannelClassLocal,
				}, "info")
			if err != nil {
				return nil, err
			}
			return pass, nil
		}

		// 2. 通道未裁决（selector 保守 nil 路径）：按模型目录解析。
		if state.Channel == "" {
			if resolver.ChannelFor(ctx, invocationModelName(inv)) == biz.ProviderChannelClassLocal {
				return pass, nil
			}
		}

		// 3. 云 / 未知（保守按云）：一级脱敏 + 二级检测。
		eng := gate.Engine(ctx)
		if eng == nil {
			metrics.SecopsMaskEventTotal.WithLabelValues(decision.TriggerMaskEngineUnavailable).Inc()
			lg.Error("脱敏引擎不可用，fail-closed 拒绝上云",
				loggateway.StepID("agent.secops_mask_engine_unavailable"),
				loggateway.Str("session_id", sessionID))
			emitErr := emitSecopsMaskDecision(ctx, deps, gate,
				decision.GateDecision{
					TriggerRule: decision.TriggerMaskEngineUnavailable,
					Outcome:     "blocked",
					Scenario:    "脱敏引擎不可用",
					Reasoning:   "脱敏规则读取/编译失败且无可用快照，禁止上云（wb: 降级本地；aranea 无网关双通道 → fail-closed）",
					GuardName:   "secops_mask",
					RunID:       runID,
					SessionID:   sessionID,
					Action:      "block_cloud_send",
				},
				biz.SecopsMaskActionEngineUnavailable, scope,
				map[string]any{"ceiling": state.Ceiling, "reason": "engine_unavailable"}, "warning")
			if emitErr != nil {
				return nil, emitErr
			}
			return nil, fmt.Errorf("secops mask gate: mask engine unavailable, cloud send blocked")
		}

		totalHits := 0
		findings := make([]string, 0, 4)
		messages := args.Request.Messages
		for i := range messages {
			msg := &messages[i]
			if msg.Content != "" {
				masked, hits := eng.Mask(scope, msg.Content)
				totalHits += hits
				findings = append(findings, eng.SecondCheck(masked)...)
				msg.Content = masked
			}
			for j := range msg.ContentParts {
				part := &msg.ContentParts[j]
				if part.Type != trpcmodel.ContentTypeText || part.Text == nil || *part.Text == "" {
					continue
				}
				masked, hits := eng.Mask(scope, *part.Text)
				totalHits += hits
				findings = append(findings, eng.SecondCheck(masked)...)
				text := masked
				part.Text = &text
			}
		}

		if len(findings) > 0 {
			kinds := biz.SecondCheckKinds(findings)
			metrics.SecopsMaskEventTotal.WithLabelValues(decision.TriggerMaskReject).Inc()
			lg.Warn("二级检测命中未脱敏敏感信息，fail-closed 拒绝上云",
				loggateway.StepID("agent.secops_mask_reject"),
				loggateway.Str("session_id", sessionID),
				loggateway.Int("findings", len(findings)),
				loggateway.Str("kinds", strings.Join(kinds, ",")))
			emitErr := emitSecopsMaskDecision(ctx, deps, gate,
				decision.GateDecision{
					TriggerRule:   decision.TriggerMaskReject,
					Outcome:       "blocked",
					Scenario:      "二级检测命中未脱敏敏感信息",
					Reasoning:     "一级规则未覆盖的 PII 被 secondCheck 检出，禁止上云（v2.0：二次检测 + 失败即拒绝上云）",
					GuardName:     "secops_mask",
					RunID:         runID,
					SessionID:     sessionID,
					ObservedValue: len(findings),
					Action:        "block_cloud_send",
					Extra:         map[string]any{"kinds": kinds, "messages": len(messages), "ceiling": state.Ceiling},
				},
				biz.SecopsMaskActionReject, scope,
				// 只落类别与计数，绝不携带 findings 原文（明文三不进）。
				map[string]any{"findings": len(findings), "kinds": kinds, "messages": len(messages)}, "warning")
			if emitErr != nil {
				return nil, emitErr
			}
			return nil, fmt.Errorf("secops mask gate: outbound text contains unmasked sensitive data (kinds=%s), cloud send blocked",
				strings.Join(kinds, ","))
		}

		metrics.SecopsMaskEventTotal.WithLabelValues(decision.TriggerCloudSend).Inc()
		emitErr := emitSecopsMaskDecision(ctx, deps, gate,
			decision.GateDecision{
				TriggerRule:   decision.TriggerCloudSend,
				Outcome:       "proceeded",
				Scenario:      "脱敏通过放行上云",
				Reasoning:     "一级脱敏 + 二级检测均通过，脱敏后消息发往云通道",
				GuardName:     "secops_mask",
				RunID:         runID,
				SessionID:     sessionID,
				ObservedValue: totalHits,
				Action:        "send_cloud",
				Extra:         map[string]any{"masked_entities": totalHits, "messages": len(messages)},
			},
			biz.SecopsMaskActionCloudSend, scope,
			map[string]any{"masked_entities": totalHits, "messages": len(messages)}, "info")
		if emitErr != nil {
			return nil, emitErr
		}
		return pass, nil
	})
}

// emitSecopsMaskDecision 双写单条脱敏事件：decision_records（EmitGate，
// 异步 outbox）+ audit_logs（严格写，失败向调用方上抛 = wb F3 审计失败即拒）。
func emitSecopsMaskDecision(ctx context.Context, deps TRPCBuilderDeps, gate SecopsMaskGate, gd decision.GateDecision, action, scope string, detail map[string]any, severity string) error {
	decision.EmitGate(ctx, deps.DecisionCollector, gd)
	if err := gate.RecordMaskAudit(ctx, action, scope, detail, severity); err != nil {
		deps.Logger().Error("脱敏审计落库失败，按 F3 fail-closed 拦截",
			loggateway.StepID("agent.secops_mask_audit_fail"),
			loggateway.Str("action", action),
			loggateway.Err(err))
		return fmt.Errorf("secops mask gate: audit persist failed: %w", err)
	}
	return nil
}

// firstNonEmptyString 返回首个非空（去空白后）值。
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
