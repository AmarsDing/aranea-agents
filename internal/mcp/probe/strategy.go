package probe

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/mcp/config"
	"aranea-agents/pkg/outboundguard"
)

type ProbeMode string

const (
	ProbeModeConnectivity  ProbeMode = "connectivity"
	ProbeModeAuthAware     ProbeMode = "auth_aware"
	ProbeModeFullHandshake ProbeMode = "full_handshake"
)

type TokenResolver func(ctx context.Context, auth config.AuthConfig) (string, error)

type ProbeStrategy interface {
	Name() string
	Probe(ctx context.Context, cfg config.ServerConfig) TestResult
}

type ConnectivityProbe struct{}

func (ConnectivityProbe) Name() string { return string(ProbeModeConnectivity) }

func (ConnectivityProbe) Probe(_ context.Context, cfg config.ServerConfig) TestResult {
	switch cfg.Transport {
	case config.TransportStdio:
		return evaluateStdio(cfg)
	case config.TransportSSE, config.TransportStreamable:
		return evaluateHTTP(cfg)
	default:
		return TestResult{
			OK:      false,
			Status:  "error",
			Message: fmt.Sprintf("transport 必须是 %v（收到 %q）", config.KnownTransports(), cfg.Transport),
		}
	}
}

type AuthAwareProbe struct {
	resolveToken TokenResolver
	inner        ProbeStrategy
}

func NewAuthAwareProbe(resolver TokenResolver) *AuthAwareProbe {
	if resolver == nil {
		resolver = noopTokenResolver
	}
	return &AuthAwareProbe{
		resolveToken: resolver,
		inner:        ConnectivityProbe{},
	}
}

func noopTokenResolver(_ context.Context, _ config.AuthConfig) (string, error) {
	return "", fmt.Errorf("no token resolver configured")
}

func (a *AuthAwareProbe) Name() string { return string(ProbeModeAuthAware) }

func (a *AuthAwareProbe) Probe(ctx context.Context, cfg config.ServerConfig) TestResult {
	if cfg.Transport == config.TransportStdio {
		return a.inner.Probe(ctx, cfg)
	}

	if cfg.Transport != config.TransportSSE && cfg.Transport != config.TransportStreamable {
		return TestResult{
			OK:      false,
			Status:  "error",
			Message: fmt.Sprintf("auth_aware 探针仅支持 HTTP 传输（收到 %q）", cfg.Transport),
		}
	}

	rawURL, timeout, errResult := validateHTTPConfig(cfg)
	if errResult != nil {
		return *errResult
	}

	hasAuth := cfg.Auth.Type != "" || cfg.Auth.APIKey != "" || cfg.Auth.AccessToken != "" || cfg.Auth.RefreshToken != ""
	if !hasAuth {
		result := doHTTPProbe(rawURL, cfg.Headers, outboundguard.NewClient(timeout))
		if result.Status == "auth_required" {
			result.OK = false
			result.Status = "auth_required"
			result.Message = "服务器要求鉴权但未配置鉴权凭据（HTTP 401/403）"
		}
		return result
	}

	token, err := a.resolveToken(ctx, cfg.Auth)
	if err != nil {
		return TestResult{
			OK:      false,
			Status:  "auth_failed",
			Message: "鉴权凭据解析失败: " + err.Error(),
		}
	}

	headers := a.buildAuthHeaders(cfg, token)

	result := doHTTPProbe(rawURL, headers, outboundguard.NewClient(timeout))
	if result.Status == "auth_required" {
		result.OK = false
		result.Status = "auth_failed"
		result.Message = "鉴权凭据无效或已过期，服务器拒绝访问（HTTP 401/403）"
	}
	return result
}

func (a *AuthAwareProbe) buildAuthHeaders(cfg config.ServerConfig, token string) map[string]string {
	headers := make(map[string]string, len(cfg.Headers)+1)
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	headerName := strings.TrimSpace(cfg.Auth.HeaderName)
	if headerName != "" {
		headers[headerName] = token
	} else {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

type Prober struct {
	strategies map[ProbeMode]ProbeStrategy
}

func NewProber(tokenResolver TokenResolver) *Prober {
	connectivity := ConnectivityProbe{}
	return &Prober{
		strategies: map[ProbeMode]ProbeStrategy{
			ProbeModeConnectivity: connectivity,
			ProbeModeAuthAware:    NewAuthAwareProbe(tokenResolver),
		},
	}
}

func (p *Prober) Evaluate(ctx context.Context, enabled bool, configJSON string) TestResult {
	if !enabled {
		return TestResult{OK: false, Status: "unknown", Message: "MCP 服务器已停用，未执行连接测试"}
	}
	cfg, err := config.ParseServerConfigJSON(configJSON)
	if err != nil {
		return TestResult{OK: false, Status: "error", Message: "config_json 格式错误: " + err.Error()}
	}
	mode := ProbeMode(cfg.ProbeMode)
	if mode == "" {
		mode = ProbeModeConnectivity
	}
	if mode == ProbeModeFullHandshake {
		return TestResult{OK: false, Status: "error", Message: "full_handshake 探针模式尚未实现，请使用 connectivity 或 auth_aware"}
	}
	strategy, ok := p.strategies[mode]
	if !ok {
		return TestResult{OK: false, Status: "error", Message: "未知的探针模式: " + string(mode)}
	}
	return strategy.Probe(ctx, cfg)
}
