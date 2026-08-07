package clientbridge

import (
	"context"
	"errors"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Full runtime tool names (toolset "client" + member name, prefixed by the
// framework's NamedToolSet). The WS invoke payload carries these names so
// the desktop companion can dispatch without re-deriving the namespace.
const (
	ToolOpenApp = "client_open_app"
	ToolOpenURL = "client_open_url"
)

// ToolSetName is the clientbridge group name used in Registry and the
// confirmation-gate catalog.
const ToolSetName = "client"

// Invoker is the narrow dependency of the client ToolSet: one blocking
// invocation round-trip through the bridge. *Bridge implements it.
type Invoker interface {
	Invoke(ctx context.Context, req InvokeRequest) (InvokeResult, error)
}

// ToolSet exposes the client tools (open_app/open_url) as an ordinary
// trpc ToolSet; execution is delegated to the desktop companion via Invoker.
type ToolSet struct {
	inv Invoker
}

// NewToolSet creates the client ToolSet. inv nil is allowed at construction
// (wiring order) but Calls then fail offline-free: they error immediately.
func NewToolSet(inv Invoker) *ToolSet {
	return &ToolSet{inv: inv}
}

// Name implements trpctool.ToolSet.
func (s *ToolSet) Name() string { return ToolSetName }

// Close implements trpctool.ToolSet (the bridge outlives any toolset).
func (s *ToolSet) Close() error { return nil }

// Tools implements trpctool.ToolSet.
func (s *ToolSet) Tools(_ context.Context) []trpctool.Tool {
	return []trpctool.Tool{
		newClientTool(s.inv, "open_app", ToolOpenApp,
			"Open an application on the user's desktop companion. "+
				"The target is resolved against the client-side whitelist; execution happens on the user's machine, not the server. "+
				"Returns ok=false with error_code DESKTOP_CLIENT_OFFLINE when the desktop companion is not connected.",
			"target", "Application name or whitelisted alias to open, e.g. wechat or chrome."),
		newClientTool(s.inv, "open_url", ToolOpenURL,
			"Open a URL in the default browser of the user's desktop companion. "+
				"Only http/https URLs are allowed by the client-side whitelist. "+
				"Returns ok=false with error_code DESKTOP_CLIENT_OFFLINE when the desktop companion is not connected.",
			"url", "The http/https URL to open."),
	}
}

// clientTool is one Agent-facing tool whose execution lives on the client.
type clientTool struct {
	inv        Invoker
	memberName string
	fullName   string
	desc       string
	argName    string
	argDesc    string
}

func newClientTool(inv Invoker, memberName, fullName, desc, argName, argDesc string) *clientTool {
	return &clientTool{
		inv: inv, memberName: memberName, fullName: fullName,
		desc: desc, argName: argName, argDesc: argDesc,
	}
}

// Declaration implements trpctool.Tool.
func (t *clientTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        t.memberName,
		Description: t.desc,
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{t.argName},
			Properties: map[string]*trpctool.Schema{
				t.argName: {Type: "string", Description: t.argDesc},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Client execution envelope.",
			Required:    []string{"ok"},
			Properties: map[string]*trpctool.Schema{
				"ok":         {Type: "boolean", Description: "Whether the client executed the action successfully."},
				"output":     {Type: "string", Description: "Client-reported output on success."},
				"error":      {Type: "string", Description: "Failure detail when ok is false."},
				"error_code": {Type: "string", Description: "Machine-readable bridge failure code (DESKTOP_CLIENT_OFFLINE / CLIENT_TOOL_TIMEOUT)."},
			},
		},
	}
}

// Call implements trpctool.CallableTool. Bridge-level failures (offline /
// timeout) are returned as a structured envelope so the Agent can paraphrase
// the machine-readable code; unexpected errors propagate as Go errors.
func (t *clientTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t.inv == nil {
		return nil, errors.New("clientbridge: toolset has no invoker")
	}
	inv, ok := agent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil || inv.Session.ID == "" {
		return nil, errors.New("clientbridge: client tools require an active session")
	}
	res, err := t.inv.Invoke(ctx, InvokeRequest{
		SessionID: inv.Session.ID,
		UserID:    inv.Session.UserID,
		Tool:      t.fullName,
		Args:      jsonArgs,
	})
	if err != nil {
		var berr *Error
		if errors.As(err, &berr) {
			return map[string]any{
				"ok":         false,
				"error":      berr.Message,
				"error_code": berr.Code,
			}, nil
		}
		return nil, err
	}
	out := map[string]any{"ok": res.OK}
	if res.OK {
		out["output"] = res.Output
	} else {
		out["error"] = res.Error
	}
	return out, nil
}

// --- interface guards ---

var (
	_ trpctool.ToolSet      = (*ToolSet)(nil)
	_ trpctool.Tool         = (*clientTool)(nil)
	_ trpctool.CallableTool = (*clientTool)(nil)
)
