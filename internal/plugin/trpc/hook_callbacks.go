package plugintrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const hookCallbackPriorityBase = 300

// HookCallbacks converts resolved hook rules into Chain entries.
func HookCallbacks(resolved []biz.ResolvedHook, agentID, agentKey string) []callbacks.Callback {
	if len(resolved) == 0 {
		return nil
	}
	out := make([]callbacks.Callback, 0, len(resolved))
	for _, rh := range resolved {
		if rh.Rule.CallbackPoint == "on_event" {
			continue
		}
		if cb := hookToCallback(rh, agentID, agentKey); cb != nil {
			out = append(out, cb)
		}
	}
	return out
}

func hookToCallback(rh biz.ResolvedHook, agentID, agentKey string) callbacks.Callback {
	priority := hookCallbackPriorityBase + rh.Hook.SortOrder
	switch rh.Rule.CallbackPoint {
	case "before_agent":
		return callbacks.NewBeforeAgentHook(priority, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
			if err := executeHookAction(ctx, rh, "before_agent", agentID, agentKey, "", args); err != nil {
				return nil, err
			}
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		})
	case "after_agent":
		return callbacks.NewAfterAgentHook(priority, func(ctx context.Context, args *trpcagent.AfterAgentArgs) (*trpcagent.AfterAgentResult, error) {
			if err := executeHookAction(ctx, rh, "after_agent", agentID, agentKey, "", args); err != nil {
				return nil, err
			}
			return &trpcagent.AfterAgentResult{Context: ctx}, nil
		})
	case "before_model":
		return callbacks.NewBeforeModelHook(priority, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
			if err := executeHookAction(ctx, rh, "before_model", agentID, agentKey, "", args); err != nil {
				return nil, err
			}
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		})
	case "after_model":
		return callbacks.NewAfterModelHook(priority, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
			if err := executeHookAction(ctx, rh, "after_model", agentID, agentKey, "", args); err != nil {
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
			if err := executeHookAction(ctx, rh, "before_tool", agentID, agentKey, toolName, args); err != nil {
				return nil, err
			}
			mod := ApplyToolModifyPatch(args, rh.Rule.Action.ModifyPatch)
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
	err := executeHookAction(ctx, h.rh, "after_tool", h.agentID, h.agentKey, toolName, args)
	return &trpctool.AfterToolResult{}, err
}

func executeHookAction(ctx context.Context, rh biz.ResolvedHook, point, agentID, agentKey, toolName string, hookCtx any) error {
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
		safego.Go(ctx, "hook.notify."+rh.Hook.Key, func() {
			postHookWebhook(url, payload)
		})
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
				ApplyModelModifyPatch(args.Request, rh.Rule.Action.ModifyPatch)
			}
		case "before_tool":
			// ModifiedArguments returned by caller after executeHookAction.
		default:
			slog.Debug("hook.modify skipped for point", "hook", rh.Hook.Key, "point", point)
		}
		return nil
	default:
		slog.Warn("hook: unknown action type", "hook", rh.Hook.Key, "action", action)
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
		slog.Debug(msg, attrs...)
	case "warn", "warning":
		slog.Warn(msg, attrs...)
	case "error":
		slog.Error(msg, attrs...)
	default:
		slog.Info(msg, attrs...)
	}
}

func postHookWebhook(url string, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("hook.notify: marshal failed", "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("hook.notify: request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("hook.notify: post failed", "url", url, "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("hook.notify: bad status", "url", url, "status", resp.StatusCode)
	}
}
