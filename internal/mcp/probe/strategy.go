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

// HandshakeFunc performs a real MCP handshake (initialize + tools/list) and
// returns the exposed tool names. headers carry resolved auth (nil for
// stdio). Injected once at startup via Prober.SetHandshakeFunc; production
// wires it to internal/tools.DiscoverMCPToolNames.
type HandshakeFunc func(ctx context.Context, cfg config.ServerConfig, headers map[string]string) ([]string, error)

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

// resolveHeaders returns the effective request headers with auth applied;
// without auth config it returns the static headers unchanged.
func (a *AuthAwareProbe) resolveHeaders(ctx context.Context, cfg config.ServerConfig) (map[string]string, error) {
	hasAuth := cfg.Auth.Type != "" || cfg.Auth.APIKey != "" || cfg.Auth.AccessToken != "" || cfg.Auth.RefreshToken != ""
	if !hasAuth {
		return cfg.Headers, nil
	}
	token, err := a.resolveToken(ctx, cfg.Auth)
	if err != nil {
		return nil, err
	}
	return a.buildAuthHeaders(cfg, token), nil
}

// ResolveHeaders exports the auth-aware header resolution for one-shot
// callers outside the probe pipeline (e.g. the wire-time tool discoverer
// adapter), so handshake auth stays identical to probe auth.
func ResolveHeaders(ctx context.Context, resolver TokenResolver, cfg config.ServerConfig) (map[string]string, error) {
	return NewAuthAwareProbe(resolver).resolveHeaders(ctx, cfg)
}

// maxProbeToolNames caps tool names embedded in the probe result Details so a
// server exposing hundreds of tools cannot bloat the API response.
const maxProbeToolNames = 50

// FullHandshakeProbe first runs the auth-aware connectivity probe (fast-fail
// on unreachable/misconfigured servers and SSRF re-validation), then performs
// a real MCP handshake (initialize + tools/list) via the injected
// HandshakeFunc. The discovered tool count/names ride in Details so callers
// can persist them without a second connection.
type FullHandshakeProbe struct {
	inner     *AuthAwareProbe
	handshake HandshakeFunc
}

func (f *FullHandshakeProbe) Name() string { return string(ProbeModeFullHandshake) }

func (f *FullHandshakeProbe) Probe(ctx context.Context, cfg config.ServerConfig) TestResult {
	if f.handshake == nil {
		return TestResult{OK: false, Status: "error", Message: "full_handshake 探针未配置握手能力（服务未注入 HandshakeFunc）"}
	}
	inner := f.inner.Probe(ctx, cfg)
	if !inner.OK {
		return inner
	}
	headers, err := f.inner.resolveHeaders(ctx, cfg)
	if err != nil {
		return TestResult{OK: false, Status: "auth_failed", Message: "鉴权凭据解析失败: " + err.Error()}
	}
	names, err := f.handshake(ctx, cfg, headers)
	if err != nil {
		return TestResult{
			OK:      false,
			Status:  "error",
			Message: err.Error(),
			Details: map[string]any{"phase": "handshake"},
		}
	}
	stored := names
	if len(stored) > maxProbeToolNames {
		stored = stored[:maxProbeToolNames]
	}
	return TestResult{
		OK:      true,
		Status:  "ok",
		Message: fmt.Sprintf("握手成功，发现 %d 个工具", len(names)),
		Details: map[string]any{"tool_count": len(names), "tool_names": stored},
	}
}

type Prober struct {
	strategies map[ProbeMode]ProbeStrategy
	authAware  *AuthAwareProbe
	handshake  HandshakeFunc
}

func NewProber(tokenResolver TokenResolver) *Prober {
	connectivity := ConnectivityProbe{}
	authAware := NewAuthAwareProbe(tokenResolver)
	return &Prober{
		strategies: map[ProbeMode]ProbeStrategy{
			ProbeModeConnectivity: connectivity,
			ProbeModeAuthAware:    authAware,
		},
		authAware: authAware,
	}
}

// SetHandshakeFunc injects the real-handshake capability used by the
// full_handshake probe mode. Must be called once at startup before serving;
// not concurrency-safe by design (wire-time configuration).
func (p *Prober) SetHandshakeFunc(f HandshakeFunc) {
	p.handshake = f
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
		return (&FullHandshakeProbe{inner: p.authAware, handshake: p.handshake}).Probe(ctx, cfg)
	}
	strategy, ok := p.strategies[mode]
	if !ok {
		return TestResult{OK: false, Status: "error", Message: "未知的探针模式: " + string(mode)}
	}
	return strategy.Probe(ctx, cfg)
}
