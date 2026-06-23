package browser

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// navigateToolSuffixes lists tool-name suffixes that accept a URL argument
// and must be SSRF-validated before the call is forwarded to the browser.
// Matching is suffix-based so MCP ToolPrefix prefixes (e.g. "bw_browser_navigate")
// are handled transparently.
var navigateToolSuffixes = []string{
	"browser_navigate",
}

// requiresURLValidation reports whether the tool with the given declaration
// name should have its URL argument validated against the navigation policy.
func requiresURLValidation(toolName string) bool {
	if toolName == "" {
		return false
	}
	for _, suffix := range navigateToolSuffixes {
		if toolName == suffix || strings.HasSuffix(toolName, "_"+suffix) {
			return true
		}
	}
	return false
}

// urlArgNames are the JSON field names that may carry the navigation URL.
// The Playwright MCP Server uses "url"; alternates are listed for robustness.
var urlArgNames = []string{"url", "URL", "Url"}

// extractURL reads the URL argument from a JSON-encoded tool call payload.
// Returns "" when no URL field is present (the caller treats "" as a no-op
// navigation which the policy allows).
func extractURL(jsonArgs []byte) string {
	if len(jsonArgs) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(jsonArgs, &raw); err != nil {
		return ""
	}
	for _, key := range urlArgNames {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// NavigationGuardedToolSet wraps a trpctool.ToolSet so that navigation tools
// returned by Tools(ctx) have their URL argument validated against the
// embedded NavigationPolicy before the call is forwarded to the inner ToolSet.
//
// Non-navigation tools are passed through unchanged. The guard is applied
// fresh on each Tools(ctx) call (consistent with the framework ToolSet contract).
//
// Compile-time interface assertion guarantees conformance to trpctool.ToolSet.
var _ trpctool.ToolSet = (*NavigationGuardedToolSet)(nil)

// NavigationGuardedToolSet wraps inner with a navigation URL validator.
type NavigationGuardedToolSet struct {
	inner  trpctool.ToolSet
	policy NavigationPolicy
}

// NewNavigationGuardedToolSet wraps inner so navigation tool calls are
// validated against policy. A nil inner returns nil.
func NewNavigationGuardedToolSet(inner trpctool.ToolSet, policy NavigationPolicy) *NavigationGuardedToolSet {
	if inner == nil {
		return nil
	}
	return &NavigationGuardedToolSet{inner: inner, policy: policy}
}

// Name returns the wrapped ToolSet's name.
func (s *NavigationGuardedToolSet) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

// Close releases resources held by the wrapped ToolSet.
func (s *NavigationGuardedToolSet) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

// Tools returns the wrapped ToolSet's tools. Navigation tools are wrapped
// with a urlValidatingCallable that enforces the SSRF policy; other tools
// are returned unchanged.
func (s *NavigationGuardedToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s == nil || s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		if t == nil {
			continue
		}
		decl := t.Declaration()
		if decl != nil && requiresURLValidation(decl.Name) {
			if ct, ok := t.(trpctool.CallableTool); ok {
				out[i] = &urlValidatingCallable{
					inner:  ct,
					policy: s.policy,
				}
				continue
			}
		}
		out[i] = t
	}
	return out
}

// urlValidatingCallable wraps a CallableTool so that its URL argument is
// validated against the navigation policy before the call is forwarded.
//
// Declaration() returns the inner tool's declaration unchanged so the model
// sees the original schema. Call() extracts the URL, validates it, and only
// forwards to the inner tool when validation passes.
type urlValidatingCallable struct {
	inner  trpctool.CallableTool
	policy NavigationPolicy
}

var _ trpctool.CallableTool = (*urlValidatingCallable)(nil)

// Declaration returns the inner tool's declaration.
func (c *urlValidatingCallable) Declaration() *trpctool.Declaration {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Declaration()
}

// Call validates the URL argument against the navigation policy and, if
// validation passes, forwards the call to the inner CallableTool.
// Validation failure returns an apierror.BadRequest without invoking the
// inner tool so the model sees a clear "url blocked" reason.
func (c *urlValidatingCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if c == nil || c.inner == nil {
		return nil, apierror.Internal(apierror.DomainTool, "browser guard unavailable")
	}
	url := extractURL(jsonArgs)
	if err := c.policy.Validate(url); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, err.Error())
	}
	return c.inner.Call(ctx, jsonArgs)
}
