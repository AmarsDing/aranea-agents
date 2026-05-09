package mcpmount

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
)

// TransportFromConfig builds an MCP client transport from platform config_json.
// When ctx is non-nil, stdio transports use exec.CommandContext so canceling ctx can stop the child.
func TransportFromConfig(ctx context.Context, cfg ServerConfig) (mcp.Transport, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Transport)) {
	case "stdio":
		cmd := strings.TrimSpace(cfg.Command)
		if cmd == "" {
			return nil, fmt.Errorf("mcp stdio transport requires command")
		}
		var c *exec.Cmd
		if ctx != nil {
			c = exec.CommandContext(ctx, cmd, cfg.Args...)
		} else {
			c = exec.Command(cmd, cfg.Args...)
		}
		if len(cfg.Env) > 0 {
			env := execEnv(cfg.Env)
			if len(env) > 0 {
				c.Env = append(c.Environ(), env...)
			}
		}
		return &mcp.CommandTransport{Command: c}, nil
	case "streamable_http":
		ep := strings.TrimSpace(cfg.URL)
		if ep == "" {
			return nil, fmt.Errorf("mcp streamable_http transport requires url")
		}
		cli := httpClient(cfg)
		return &mcp.StreamableClientTransport{Endpoint: ep, HTTPClient: cli}, nil
	case "sse":
		ep := strings.TrimSpace(cfg.URL)
		if ep == "" {
			return nil, fmt.Errorf("mcp sse transport requires url")
		}
		cli := httpClient(cfg)
		return &mcp.SSEClientTransport{Endpoint: ep, HTTPClient: cli}, nil
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", cfg.Transport)
	}
}

func httpClient(cfg ServerConfig) *http.Client {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	rt := http.DefaultTransport
	if len(cfg.Headers) > 0 {
		rt = &headerRoundTripper{base: rt, headers: cfg.Headers}
	}
	return &http.Client{Timeout: timeout, Transport: rt}
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range h.headers {
		if strings.TrimSpace(k) != "" {
			r.Header.Set(k, v)
		}
	}
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

func execEnv(m map[string]string) []string {
	var out []string
	for k, v := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

func toolPredicateForPrefix(prefix string) tool.Predicate {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	return func(_ agent.ReadonlyContext, t tool.Tool) bool {
		return strings.HasPrefix(t.Name(), prefix)
	}
}
