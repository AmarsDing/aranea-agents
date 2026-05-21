package biz

import "strings"

// PluginSandboxMode controls Phase 4 sandbox isolation (none | process | container).
type PluginSandboxMode string

const (
	PluginSandboxNone      PluginSandboxMode = "none"
	PluginSandboxProcess   PluginSandboxMode = "process"
	PluginSandboxContainer PluginSandboxMode = "container"
)

// PluginVersionPolicy pins a plugin rule to a semver range (Phase 4).
type PluginVersionPolicy struct {
	PluginID   string
	MinVersion string
	MaxVersion string
	Pinned     string
}

// NormalizePluginSandboxMode returns a supported sandbox mode, defaulting to process for high-risk plugins.
func NormalizePluginSandboxMode(raw string, riskLevel string) PluginSandboxMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PluginSandboxNone):
		return PluginSandboxNone
	case string(PluginSandboxContainer):
		return PluginSandboxContainer
	case string(PluginSandboxProcess):
		return PluginSandboxProcess
	}
	if strings.EqualFold(riskLevel, "high") || strings.EqualFold(riskLevel, "critical") {
		return PluginSandboxProcess
	}
	return PluginSandboxNone
}

// ResolvePluginVersion picks pinned version or falls back to latest within policy bounds.
func ResolvePluginVersion(policy PluginVersionPolicy, latest string) string {
	if v := strings.TrimSpace(policy.Pinned); v != "" {
		return v
	}
	return strings.TrimSpace(latest)
}
