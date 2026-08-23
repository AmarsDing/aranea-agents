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
	TypeAuto      = "auto"
	TypeDisabled  = "disabled" // code execution refused (e.g. production without sandbox)
)

// EnvConfig holds process-level code executor settings from environment variables.
type EnvConfig struct {
	Backend          string
	DockerImage      string
	Timeout          time.Duration
	E2BAPIKey        string
	AllowLocalInProd bool // when false and ARANEA_ENV=production, refuse local (fail-closed)
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
	return cfg
}

// ValidTypes lists supported executor backend identifiers.
// TypeDisabled is intentionally excluded: it represents a fallback state
// (e.g. production without sandbox) rather than a user-configurable backend.
func ValidTypes() []string {
	return []string{TypeLocal, TypeDocker, TypeE2B, TypeContainer}
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
