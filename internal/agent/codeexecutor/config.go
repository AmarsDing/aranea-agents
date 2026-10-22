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

// Surface 是 code_exec 的三个固定消费面（review 2026-10-21 §2.4 ③ per-surface
// 策略骨架；§2.5 #2：策略只允许三固定面，未知面按 degrade 处理——防误拍死）。
type Surface string

const (
	SurfaceChatCodeexec Surface = "chat_codeexec" // chat/聊天 code_exec
	SurfaceSkillGate    Surface = "skill_gate"    // 技能发布校验门禁
	SurfaceMonitor      Surface = "monitor"       // monitor 建议校验
)

// PolicyLevel 是单面三值：degrade（默认，现状降级链语义）| enforce（面级严格：
// 要求隔离执行域 sandbox/docker，不和解为 local，不可得即拒绝可见执行）|
// refuse（面级拒绝代码执行）。
type PolicyLevel string

const (
	PolicyDegrade PolicyLevel = "degrade"
	PolicyEnforce PolicyLevel = "enforce"
	PolicyRefuse  PolicyLevel = "refuse"
)

// SurfacePolicy keeps the three fixed surfaces as a typed struct
// (AS-COG-01: 单一 typed 字段注入 EnvConfig 而不是平铺三个字段)。
// 零值 = 三面全 degrade（现状语义）。
type SurfacePolicy struct {
	ChatCodeexec PolicyLevel
	SkillGate    PolicyLevel
	Monitor      PolicyLevel
}

// For resolves the effective policy for a surface; unknown surfaces fall back
// to degrade（防 typo 面名直接把面拍死）。
func (p SurfacePolicy) For(s Surface) PolicyLevel {
	switch s {
	case SurfaceSkillGate:
		return normLevel(p.SkillGate)
	case SurfaceMonitor:
		return normLevel(p.Monitor)
	case SurfaceChatCodeexec:
		return normLevel(p.ChatCodeexec)
	default:
		return PolicyDegrade
	}
}

func normLevel(l PolicyLevel) PolicyLevel {
	switch l {
	case PolicyEnforce, PolicyRefuse:
		return l
	default:
		return PolicyDegrade
	}
}

// parsePolicyLevel 归一 per-surface 策略值（沿用下方 FallbackPolicy 归一先例
// 的风格：ToLower+TrimSpace）。空 = 默认 degrade；三值之外一律归一为 refuse
// ——fail-closed：宁可 typo（如 "enforcce"）显式拍死，也不静默放宽执行面。
func parsePolicyLevel(raw string) PolicyLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return PolicyDegrade
	case string(PolicyDegrade):
		return PolicyDegrade
	case string(PolicyEnforce):
		return PolicyEnforce
	case string(PolicyRefuse):
		return PolicyRefuse
	default:
		return PolicyRefuse
	}
}

// EnvConfig holds process-level code executor settings from environment variables.
type EnvConfig struct {
	Backend          string
	DockerImage      string
	Timeout          time.Duration
	E2BAPIKey        string
	AllowLocalInProd bool   // when false and ARANEA_ENV=production, refuse local (fail-closed)
	FallbackPolicy   string // CODE_EXECUTOR_FALLBACK_POLICY：degrade（默认）| strict
	// Policy is the per-surface posture skeleton (P0-1 halv of P0-1↔P0-7：
	// 类型先落、批 B 接硬强制热路径）。零值三面全 degrade = 现状。
	Policy SurfacePolicy
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
	cfg.Policy = SurfacePolicy{
		ChatCodeexec: parsePolicyLevel(os.Getenv("CODE_EXECUTOR_POLICY_CHAT_CODEEXEC")),
		SkillGate:    parsePolicyLevel(os.Getenv("CODE_EXECUTOR_POLICY_SKILL_GATE")),
		Monitor:      parsePolicyLevel(os.Getenv("CODE_EXECUTOR_POLICY_MONITOR")),
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
