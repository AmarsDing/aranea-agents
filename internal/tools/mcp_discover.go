package tools

// mcp_discover.go — 真实 MCP 握手的工具发现（P2）。
//
// 与连通性探测（internal/mcp/probe 的 LookPath / HTTP GET）不同，这里执行
// 完整 initialize + tools/list，拿到服务器真实暴露的工具列表。发现走
// 非池化独立连接（用完即关），不经过进程级连接池：
//   - 池化 ToolSet 的 Tools() 有 5min 缓存（DefaultMCPToolsCacheTTL），
//     发现功能恰恰要新鲜列表；
//   - 发现可能被健康 Runner 低频兜底触发，不应占用/搅动池 entry 的空闲计时。
//
// 发现结果由调用方（biz 层）持久化到 metadata_json（tool_count/tool_names/
// tools_discovered_at），发现失败不翻转健康状态。

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpdefaults "aranea-agents/internal/mcp"
	mcpconfig "aranea-agents/internal/mcp/config"
)

// DiscoverMCPToolNames performs a real MCP handshake (initialize + tools/list)
// against cfg and returns the exposed tool names. The toolset-level prefix
// ("<serverKey>_<tool>") is stripped for display readability. The applied
// tool_prefix filter (ToolFilterForPrefix) is honored, so the result matches
// what the runtime would actually mount for agents.
//
// The connection is independent of the process pool and always closed before
// return. Timeout: cfg.TimeoutSec, defaulting to DefaultDiscoveryTimeoutSec,
// capped at 60s so a hung stdio child cannot block the health runner.
func DiscoverMCPToolNames(ctx context.Context, cfg MCPServerConfig) ([]string, error) {
	timeout := mcpdefaults.DefaultDiscoveryTimeoutSec
	if cfg.TimeoutSec > 0 {
		timeout = cfg.TimeoutSec
	}
	if timeout > 60 {
		timeout = 60
	}
	dctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	ts, err := buildMCPToolSet(cfg)
	if err != nil {
		return nil, fmt.Errorf("build toolset: %w", err)
	}
	if ts == nil {
		return nil, fmt.Errorf("build toolset: empty toolset")
	}
	defer func() { _ = ts.Close() }()

	if init, ok := ts.(interface{ Init(context.Context) error }); ok {
		if err := init.Init(dctx); err != nil {
			return nil, friendlyDiscoveryError(err)
		}
	}
	exposed := ts.Tools(dctx)
	prefix := cfg.Name + "_"
	names := make([]string, 0, len(exposed))
	for _, t := range exposed {
		if t == nil || t.Declaration() == nil {
			continue
		}
		names = append(names, strings.TrimPrefix(t.Declaration().Name, prefix))
	}
	return names, nil
}

// friendlyDiscoveryError normalizes common handshake failures into operator-
// readable messages (the raw framework errors are wrapped chains).
func friendlyDiscoveryError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "context deadline"):
		return fmt.Errorf("MCP 握手超时（initialize/tools/list 未在时限内完成）: %w", err)
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("MCP 握手失败，服务器拒绝连接: %w", err)
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"):
		return fmt.Errorf("MCP 握手被服务器拒绝（鉴权失败）: %w", err)
	default:
		return fmt.Errorf("MCP 握手失败: %w", err)
	}
}

// MCPServerConfigFromServerConfig converts a stored config_json shape into a
// runtime toolset config for one-shot operations (tool discovery). headers
// must already carry resolved auth (api_key / bearer token); per-user
// credential servers are NOT supported here (no invocation context).
func MCPServerConfigFromServerConfig(name string, sc mcpconfig.ServerConfig, headers map[string]string) MCPServerConfig {
	return MCPServerConfig{
		Name:                name,
		Transport:           string(sc.Transport),
		ServerURL:           sc.URL,
		Command:             sc.Command,
		Args:                sc.Args,
		Env:                 sc.Env,
		Headers:             headers,
		TimeoutSec:          sc.TimeoutSec,
		ToolPrefix:          sc.ToolPrefix,
		SessionReconnectMax: sc.SessionReconnectMax,
	}
}
