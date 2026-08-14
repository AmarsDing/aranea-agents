package team

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// evalProfileRunOptions builds the run-level options from the definition's
// eval_profile (P3-4, ADR-E D2): pinned model + pinned generation fields +
// tool visibility allowlist. The options ride the root RunOptions, which the
// graph runtime propagates to every member invocation — one install, whole
// chain covered, same kernel as production (DSH preset principle).
//
// Returns nil when the profile is absent or fully empty (zero-cost production
// path). Sub-fields are independent: a profile may pin only tools, only the
// model, or only generation fields.
func (r *Runner) evalProfileRunOptions(ctx context.Context, def Definition, sessID string) []trpcagent.RunOption {
	ep := def.EvalProfile
	if ep == nil {
		return nil
	}
	var opts []trpcagent.RunOption
	if prov, mod := strings.TrimSpace(ep.Provider), strings.TrimSpace(ep.Model); prov != "" && mod != "" {
		opts = append(opts, trpcagent.WithModelSelector(
			agent.PinnedModelSelector(prov, mod, r.td.ReadDeps.LLM, r.td.RoundTripForSession(sessID), r.lg),
		))
	}
	if len(ep.ExtraModelFields) > 0 {
		opts = append(opts, trpcagent.WithModelRequestExtraFields(ep.ExtraModelFields))
	}
	if len(ep.ToolAllowlist) > 0 {
		opts = append(opts, trpcagent.WithToolFilter(trpctool.NewIncludeToolNamesFilter(ep.ToolAllowlist...)))
	}
	if len(opts) == 0 {
		return nil
	}
	// D3: 评测审计轨迹——哪个 profile 产出哪个 run 必须可从流程日志回答（K1）。
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogDone("team.eval_profile.applied", "评测态 profile 生效",
			event.P("provider", strings.TrimSpace(ep.Provider)),
			event.P("model", strings.TrimSpace(ep.Model)),
			event.P("tool_allowlist_size", len(ep.ToolAllowlist)),
			event.P("extra_model_fields", sortedKeys(ep.ExtraModelFields)))
	}
	r.lg.Info("评测态 profile 生效",
		loggateway.StepID("team.eval_profile.applied"),
		loggateway.Str("provider", strings.TrimSpace(ep.Provider)),
		loggateway.Str("model", strings.TrimSpace(ep.Model)),
		loggateway.Int("tool_allowlist_size", len(ep.ToolAllowlist)))
	return opts
}

// modelGovernanceRunOptions returns the run-level model governance options:
// eval_profile pin (P3-4) wins over model_cascade (P2-1) when both are
// configured — the two are mutually exclusive goals (reproducibility vs cost
// optimization), and silently stacking them would route members to the
// cascade tier while the leader is pinned, producing an unintended mix.
func (r *Runner) modelGovernanceRunOptions(ctx context.Context, def Definition, sessID string) []trpcagent.RunOption {
	if def.EvalProfile != nil {
		if def.ModelCascade != nil {
			r.lg.Warn("eval_profile 与 model_cascade 同时配置，eval_profile 胜出（可复现性优先）",
				loggateway.StepID("team.eval_profile.applied"),
				loggateway.Str("member_model", def.ModelCascade.MemberModel),
				loggateway.Str("pinned_model", def.EvalProfile.Model))
		}
		if opts := r.evalProfileRunOptions(ctx, def, sessID); len(opts) > 0 {
			return opts
		}
		// 全空 profile 不阻断 cascade——等于未配置 eval_profile。
	}
	if opt := r.cascadeRunOption(ctx, def, sessID); opt != nil {
		return []trpcagent.RunOption{opt}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
