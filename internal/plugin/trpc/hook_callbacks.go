package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const hookCallbackPriorityBase = 300

// HookCallbacks converts resolved hook rules into Chain entries.
func HookCallbacks(resolved []biz.ResolvedHook, agentID, agentKey string, stats StatsRecorder, notifier *HookNotifier, lg loggateway.Logger) []callbacks.Callback {
	if len(resolved) == 0 {
		return nil
	}
	out := make([]callbacks.Callback, 0, len(resolved))
	for _, rh := range resolved {
		if rh.Rule.CallbackPoint == "on_event" {
			continue
		}
		if cb := hookToCallback(rh, agentID, agentKey, stats, notifier, lg); cb != nil {
			out = append(out, cb)
		}
	}
	return out
}

func hookToCallback(rh biz.ResolvedHook, agentID, agentKey string, stats StatsRecorder, notifier *HookNotifier, lg loggateway.Logger) callbacks.Callback {
	priority := hookCallbackPriorityBase + rh.Hook.SortOrder
	switch rh.Rule.CallbackPoint {
	case "before_agent":
		return callbacks.NewBeforeAgentHook(priority, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
			if err := executeHookAction(ctx, stats, notifier, rh, "before_agent", agentID, agentKey, "", args, lg); err != nil {
				return nil, err
			}
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		})
	case "after_agent":
		return callbacks.NewAfterAgentHook(priority, func(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
			if err := executeHookAction(ctx, stats, notifier, rh, "after_agent", agentID, agentKey, "", args, lg); err != nil {
				return nil, err
			}
			return &trpcagent.AfterAgentResult{Context: ctx}, nil
		})
	case "before_model":
		return callbacks.NewBeforeModelHook(priority, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
			if err := executeHookAction(ctx, stats, notifier, rh, "before_model", agentID, agentKey, "", args, lg); err != nil {
				return nil, err
			}
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		})
	case "after_model":
		return callbacks.NewAfterModelHook(priority, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
			if err := executeHookAction(ctx, stats, notifier, rh, "after_model", agentID, agentKey, "", args, lg); err != nil {
				return nil, err
			}
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		})
	case "before_tool":
		return callbacks.NewBeforeToolHook(priority, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
			toolName := ""
			if args != nil {
				toolName = args.ToolName
			}
			if !biz.HookAppliesToTool(rh.Rule.Condition, toolName) {
				return &trpctool.BeforeToolResult{Context: ctx}, nil
			}
			if err := executeHookAction(ctx, stats, notifier, rh, "before_tool", agentID, agentKey, toolName, args, lg); err != nil {
				return nil, err
			}
			mod := ApplyToolModifyPatch(args, rh.Rule.Action.ModifyPatch, lg)
			res := &trpctool.BeforeToolResult{Context: ctx}
			if len(mod) > 0 {
				res.ModifiedArguments = mod
			}
			return res, nil
		})
	case "after_tool":
		return &hookAfterToolCallback{
			priority: priority,
			rh:       rh,
			agentID:  agentID,
			agentKey: agentKey,
			stats:    stats,
			notifier: notifier,
			lg:       lg,
		}
	default:
		return nil
	}
}

type hookAfterToolCallback struct {
	priority int
	rh       biz.ResolvedHook
	agentID  string
	agentKey string
	stats    StatsRecorder
	notifier *HookNotifier
	lg       loggateway.Logger
}

func (h *hookAfterToolCallback) Point() callbacks.CallbackPoint { return callbacks.PointAfterTool }
func (h *hookAfterToolCallback) Priority() int                  { return h.priority }

func (h *hookAfterToolCallback) HandleAfterTool(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
	toolName := ""
	if args != nil {
		toolName = args.ToolName
	}
	if !biz.HookAppliesToTool(h.rh.Rule.Condition, toolName) {
		return &trpctool.AfterToolResult{}, nil
	}
	err := executeHookAction(ctx, h.stats, h.notifier, h.rh, "after_tool", h.agentID, h.agentKey, toolName, args, h.lg)
	return &trpctool.AfterToolResult{}, err
}

func executeHookAction(ctx context.Context, stats StatsRecorder, notifier *HookNotifier, rh biz.ResolvedHook, point, agentID, agentKey, toolName string, hookCtx any, lg loggateway.Logger) error {
	start := time.Now()
	action := strings.ToLower(strings.TrimSpace(rh.Rule.Action.Type))
	if action == "" {
		action = "log"
	}
	status := "ok"
	var err error
	defer func() {
		st := status
		if err != nil {
			st = "error"
		}
		if st == "blocked" {
			metrics.PluginBlockTotal.WithLabelValues("hook:"+rh.Hook.Key, point).Inc()
		}
		metrics.PluginInvokeTotal.WithLabelValues("hook:"+rh.Hook.Key, point, st).Inc()
		metrics.ObserveCallback("hook", point, start, err)
		durationMS := time.Since(start).Milliseconds()
		getHookLogger().Info("hook.execute",
			"hook_key", rh.Hook.Key,
			"point", point,
			"action", action,
			"status", st,
			"agent_id", agentID,
			"agent_key", agentKey,
			"tool", toolName,
			"duration_ms", durationMS,
		)
		recordHookAudit(stats, ctx, rh, point, st, agentID, durationMS)
	}()

	switch action {
	case "log":
		logHookAction(rh, point, agentID, agentKey, toolName, action)
		return nil
	case "notify":
		url := strings.TrimSpace(rh.Rule.Action.WebhookURL)
		if url == "" {
			err = kerrors.BadRequest("HOOK", "webhook_url required for notify action")
			status = "error"
			return err
		}
		payload := map[string]any{
			"hook_key":  rh.Hook.Key,
			"point":     point,
			"agent_id":  agentID,
			"agent_key": agentKey,
			"tool_name": toolName,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		n := notifier
		if n == nil {
			n = NewHookNotifier(nil, nil, lg)
		}
		if enqueueErr := n.EnqueueNotify(ctx, rh, payload); enqueueErr != nil {
			err = enqueueErr
			status = "error"
			return err
		}
		status = "queued"
		return nil
	case "block":
		msg := strings.TrimSpace(rh.Rule.Action.Message)
		if msg == "" {
			msg = fmt.Sprintf("blocked by hook %s", rh.Hook.Key)
		}
		err = kerrors.Forbidden("HOOK_BLOCKED", msg)
		status = "blocked"
		return err
	case "modify":
		if len(rh.Rule.Action.ModifyPatch) == 0 {
			return nil
		}
		switch point {
		case "before_model":
			args, _ := hookCtx.(*trpcmodel.BeforeModelArgs)
			if args != nil && args.Request != nil {
				ApplyModelModifyPatch(args.Request, rh.Rule.Action.ModifyPatch, lg)
			}
		case "before_tool":
			// ModifiedArguments returned by caller after executeHookAction.
		default:
			getHookLogger().Debug("hook.modify skipped for point", "hook", rh.Hook.Key, "point", point)
		}
		return nil
	default:
		getHookLogger().Warn("hook: unknown action type", "hook", rh.Hook.Key, "action", action)
		return nil
	}
}

func logHookAction(rh biz.ResolvedHook, point, agentID, agentKey, toolName, action string) {
	level := strings.ToLower(strings.TrimSpace(rh.Rule.Action.LogLevel))
	if level == "" {
		level = "info"
	}
	msg := strings.TrimSpace(rh.Rule.Action.Message)
	if msg == "" {
		msg = fmt.Sprintf("hook %s at %s", rh.Hook.Key, point)
	}
	attrs := []any{
		"hook", rh.Hook.Key,
		"point", point,
		"agent_id", agentID,
		"agent_key", agentKey,
		"tool", toolName,
		"action", action,
	}
	switch level {
	case "debug":
		getHookLogger().Debug(msg, attrs...)
	case "warn", "warning":
		getHookLogger().Warn(msg, attrs...)
	case "error":
		getHookLogger().Error(msg, attrs...)
	default:
		getHookLogger().Info(msg, attrs...)
	}
}

