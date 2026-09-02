package codeexecutor

import (
	"os"
	"strings"
	"time"
)

const (
	TypeLocal     = "local"
	TypeDocker    = "docker"
	TypeE2B       = "e2b"
	TypeContainer = "container"
	TypeSandbox   = "sandbox"  // M82 pooled sandbox backend (sandbox→docker→local fallback chain)
	TypeAuto      = "auto"
	TypeDisabled  = "disabled" // code execution refused (e.g. production without sandbox)
)

// Fallback policy values for EnvConfig.FallbackPolicy (83-长时运行韧性 FR-3)。
const (
	FallbackPolicyDegrade = "degrade" // 默认：后端不可用时沿降级链回退（现状语义）
	FallbackPolicyStrict  = "strict"  // 严格：后端不可用即拒绝执行，禁止静默降级
)

// EnvConfig holds process-level code executor settings from environment variables.
type EnvConfig struct {
	Backend          string
	DockerImage      string
	Timeout          time.Duration
	E2BAPIKey        string
	AllowLocalInProd bool   // when false and ARANEA_ENV=production, refuse local (fail-closed)
	FallbackPolicy   string // CODE_EXECUTOR_FALLBACK_POLICY：degrade（默认）| strict
}

// StrictFallback reports whether backend degradation is forbidden
// (83-长时运行韧性 FR-3)：strict 模式下请求的后端不可用即拒绝（TypeDisabled），
// 不静默降级到隔离性更低的后端。
func (c EnvConfig) StrictFallback() bool {
	return c.FallbackPolicy == FallbackPolicyStrict
}

// LoadEnvConfig reads CODE_EXECUTOR_* and related environment variables.
func LoadEnvConfig() EnvConfig {
	cfg := EnvConfig{
		Backend: strings.TrimSpace(os.Getenv("CODE_EXECUTOR_BACKEND")),
		Timeout: 60 * time.Second,
	}
	if img := strings.TrimSpace(os.Getenv("CODE_EXECUTOR_DOCKER_IMAGE")); img != "" {
		cfg.DockerImage = img
	}
	if raw := strings.TrimSpace(os.Getenv("CODE_EXECUTOR_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}
	cfg.E2BAPIKey = strings.TrimSpace(os.Getenv("E2B_API_KEY"))
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODE_EXECUTOR_ALLOW_LOCAL_IN_PROD"))) {
	case "1", "true", "yes":
		cfg.AllowLocalInProd = true
	}
	cfg.FallbackPolicy = strings.ToLower(strings.TrimSpace(os.Getenv("CODE_EXECUTOR_FALLBACK_POLICY")))
	if cfg.FallbackPolicy != FallbackPolicyStrict {
		// 未知值归一默认，避免 typo（如 "stict"）被当作显式 degrade 漏过审查。
		cfg.FallbackPolicy = FallbackPolicyDegrade
	}
	return cfg
}

// ValidTypes lists supported executor backend identifiers.
// TypeDisabled is intentionally excluded: it represents a fallback state
// (e.g. production without sandbox) rather than a user-configurable backend.
func ValidTypes() []string {
	return []string{TypeLocal, TypeDocker, TypeE2B, TypeContainer, TypeSandbox}
}

// NormalizeType returns a canonical backend type or TypeLocal when unknown.
func NormalizeType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case TypeDocker:
		return TypeDocker
	case TypeE2B:
		return TypeE2B
	case TypeContainer:
		return TypeContainer
	case TypeSandbox:
		return TypeSandbox
	case TypeAuto:
		return TypeAuto
	case TypeLocal, "":
		return TypeLocal
	default:
		return TypeLocal
	}
}

// PreferDockerWhenUnset is true when the agent/env backend is empty or auto
// and the Docker daemon is up. Explicit "local" stays local.
func PreferDockerWhenUnset(agentType, envBackend string, dockerOK bool) bool {
	if !dockerOK {
		return false
	}
	if strings.TrimSpace(envBackend) != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "", TypeAuto:
		return true
	default:
		return false
	}
}

// ResolveType picks the effective backend: agent setting > env > default local.
func ResolveType(agentType, envBackend string) string {
	if t := strings.TrimSpace(agentType); t != "" {
		return NormalizeType(t)
	}
	if t := strings.TrimSpace(envBackend); t != "" {
		return NormalizeType(t)
	}
	return TypeLocal
}
