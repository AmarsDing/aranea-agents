package biz

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz/shared"
)

// Tool assembly reconciliation (79-runtime-governance R8 / P5.2): the Go
// single source for the tool-assembly audit checks previously re-implemented
// in test/agent-audit/audit.py (ADR C2 统一裁定：effective tools 单源 =
// biz.GetEffectiveTools)。运行时 doctor（GET /api/v1/admin/diagnostics）与
// 离线 audit.py 均消费本实现——audit.py 的 Python 策略引擎复刻就此下线。
//
// 检查项与 audit.py §一/二/四 对齐（阈值/豁免同源移植）：
//
//	NO_SETTINGS_ROW   LOW   无 runtime_settings 行（运行时从 config_json legacy 迁移）
//	SPIRIT_PROFILE    HIGH  专家岗持久化 profile=spirit（org-invariants R17，运行时被钳制）
//	TOOLS_OFF         MID   tools_enabled=false
//	ZERO_TOOLS        HIGH  有效工具=0（纯聊天）
//	FEW_TOOLS         MID   有效工具 ≤3
//	GOV_NOT_READONLY  MID   治理岗有效 profile 非 read_only
//	UNDEFINED_PROFILE LOW   profile 非注册值，仅靠归一化兜底
//	REDUNDANT_DENY    LOW   read_only 下冗余 memory_* deny
//	DEAD_TOOL         LOW   目录层 enabled=false 且非 opt-in（全员硬 deny）
//
// 豁免（2026-08-24 P5 复盘，与 audit.py BY_DESIGN_NO_TOOLS / EVAL_PROBE_PREFIX
// 一致）：__voice_butler__（chat_only 刻意）、__memory__/__skills__（核心
// butler 工具经 CustomTools 按 agent_key 条件注入，目录面仅 datetime 属设计）、
// eval_*（评测探针）。

const (
	ToolAssemblySeverityHigh = "HIGH"
	ToolAssemblySeverityMid  = "MID"
	ToolAssemblySeverityLow  = "LOW"
)

const (
	ToolAssemblyCodeNoSettingsRow   = "NO_SETTINGS_ROW"
	ToolAssemblyCodeSpiritProfile   = "SPIRIT_PROFILE"
	ToolAssemblyCodeToolsOff        = "TOOLS_OFF"
	ToolAssemblyCodeZeroTools       = "ZERO_TOOLS"
	ToolAssemblyCodeFewTools        = "FEW_TOOLS"
	ToolAssemblyCodeGovNotReadonly  = "GOV_NOT_READONLY"
	ToolAssemblyCodeUndefinedProf   = "UNDEFINED_PROFILE"
	ToolAssemblyCodeRedundantDeny   = "REDUNDANT_DENY"
	ToolAssemblyCodeDeadTool        = "DEAD_TOOL"
	toolAssemblyFewToolsMax         = 3
	toolAssemblyEvalProbeKeyPrefix  = "eval_"
)

// ToolAssemblyIssue 是一条对账问题（severity/code 取值与 audit.py 一致）。
type ToolAssemblyIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	AgentKey string `json:"agent_key,omitempty"`
	Message  string `json:"message"`
}

// ToolAssemblyAgentRow 是单 agent 的有效工具面对账行（audit.py §三 明细的
// 单源替代）。
type ToolAssemblyAgentRow struct {
	AgentKey      string   `json:"agent_key"`
	DisplayName   string   `json:"display_name"`
	ProfileRaw    string   `json:"profile_raw"`
	ProfileEff    string   `json:"profile_eff"`
	Clamped       string   `json:"clamped,omitempty"`
	ToolsEnabled  bool     `json:"tools_enabled"`
	SettingsSrc   string   `json:"settings_src"` // db | legacy
	EffectiveKeys []string `json:"effective_keys"`
}

// ToolAssemblyReport 是全量对账结果：问题清单 + 逐 agent 明细 + 目录层死工具。
type ToolAssemblyReport struct {
	AgentsChecked int                    `json:"agents_checked"`
	Issues        []ToolAssemblyIssue    `json:"issues"`
	Agents        []ToolAssemblyAgentRow `json:"agents"`
	DeadTools     []string               `json:"dead_tools"`
}

// toolAssemblyByDesignNoTools 与 audit.py BY_DESIGN_NO_TOOLS 一致。
var toolAssemblyByDesignNoTools = map[string]bool{
	VoiceButlerAgentKey: true,
	MemoryAgentKey:      true,
	SkillsAgentKey:      true,
}

// ReconcileToolAssembly 对全部存活 agent 跑工具装配对账。有效工具面经与
// GetEffectiveTools 完全相同的管线阶段计算（runtime settings 读取 →
// ClampSpecialistToolFace → buildAgentEffectiveTools → overrides →
// DisableSpiritReservedTools），目录只加载一次——逻辑单源、无 Python 复刻。
// web_research 平台 readiness 门不影响计数语义（仅翻转该单键 state），此处不
// 重复装配。
func (u *AgentUsecase) ReconcileToolAssembly(ctx context.Context) (ToolAssemblyReport, error) {
	page, err := u.reader.SearchAgents(ctx, AgentListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return ToolAssemblyReport{}, err
	}
	catalog, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return ToolAssemblyReport{}, err
	}

	report := ToolAssemblyReport{
		Issues: []ToolAssemblyIssue{},
		Agents: make([]ToolAssemblyAgentRow, 0, len(page.Items)),
	}
	for _, t := range catalog.Items {
		if !t.Enabled && !registryOptInOnlyKeys[t.Key] {
			report.DeadTools = append(report.DeadTools, t.Key)
			report.Issues = append(report.Issues, ToolAssemblyIssue{
				Severity: ToolAssemblySeverityLow,
				Code:     ToolAssemblyCodeDeadTool,
				Message:  fmt.Sprintf("目录层死工具（enabled=false 且非 opt-in，全员硬 deny）cat=%s src=%s risk=%s", t.Category, t.Source, t.RiskLevel),
			})
		}
	}
	sort.Strings(report.DeadTools)

	for _, ag := range page.Items {
		row, issues := u.reconcileAgentToolAssembly(ctx, ag, catalog.Items)
		report.Agents = append(report.Agents, row)
		report.Issues = append(report.Issues, issues...)
	}
	report.AgentsChecked = len(report.Agents)
	sort.Slice(report.Issues, func(i, j int) bool {
		si, sj := toolAssemblySeverityRank(report.Issues[i].Severity), toolAssemblySeverityRank(report.Issues[j].Severity)
		if si != sj {
			return si < sj
		}
		if report.Issues[i].Code != report.Issues[j].Code {
			return report.Issues[i].Code < report.Issues[j].Code
		}
		return report.Issues[i].AgentKey < report.Issues[j].AgentKey
	})
	return report, nil
}

func toolAssemblySeverityRank(s string) int {
	switch s {
	case ToolAssemblySeverityHigh:
		return 0
	case ToolAssemblySeverityMid:
		return 1
	default:
		return 2
	}
}

// reconcileAgentToolAssembly 计算单 agent 的对账行与问题列表。
func (u *AgentUsecase) reconcileAgentToolAssembly(ctx context.Context, ag Agent, catalog []Tool) (ToolAssemblyAgentRow, []ToolAssemblyIssue) {
	settings, src, err := u.toolAssemblyRuntimeSettings(ctx, ag)
	if err != nil {
		// 设置读取失败不阻断全量对账：按默认设置继续并记 LOW。
		settings = withSettingDefaults(DefaultAgentRuntimeSettings())
		src = "db"
	}
	rawProfile := strings.TrimSpace(settings.ToolsProfile)

	gov := IsOrgGovernanceAgent(ag)
	spirit := strings.TrimSpace(ag.AgentKey) == SpiritAgentKey

	clampedSettings := settings
	ClampSpecialistToolFace(&clampedSettings, ag)
	effProfile := clampedSettings.ToolsProfile
	clampedNote := ""
	switch {
	case !spirit && gov && CanonicalToolProfile(rawProfile) != "read_only":
		clampedNote = "gov->read_only"
	case !spirit && !gov && CanonicalToolProfile(rawProfile) == "spirit":
		clampedNote = "spirit->coding"
	}

	eff := buildAgentEffectiveTools(clampedSettings, catalog, u.lg)
	if overrides, oerr := u.tools.ListToolAgentOverridesByAgent(ctx, ag.ID); oerr == nil {
		ApplyAgentToolOverrides(&eff, catalog, overrides)
	}
	DisableSpiritReservedTools(&eff, ag.AgentKey)
	keys := make([]string, 0, len(eff.Items))
	for _, it := range eff.Items {
		if it.Enabled && it.EffectiveState == "allowed" {
			keys = append(keys, it.ToolKey)
		}
	}
	sort.Strings(keys)

	row := ToolAssemblyAgentRow{
		AgentKey:      ag.AgentKey,
		DisplayName:   ag.DisplayName,
		ProfileRaw:    rawProfile,
		ProfileEff:    effProfile,
		Clamped:       clampedNote,
		ToolsEnabled:  settings.ToolsEnabled,
		SettingsSrc:   src,
		EffectiveKeys: keys,
	}

	var issues []ToolAssemblyIssue
	issue := func(sev, code, msg string) {
		issues = append(issues, ToolAssemblyIssue{Severity: sev, Code: code, AgentKey: ag.AgentKey, Message: msg})
	}
	if src == "legacy" {
		issue(ToolAssemblySeverityLow, ToolAssemblyCodeNoSettingsRow, "无 runtime_settings 行，运行时从 config_json legacy 迁移")
	}
	if !spirit && !gov && CanonicalToolProfile(rawProfile) == "spirit" {
		issue(ToolAssemblySeverityHigh, ToolAssemblyCodeSpiritProfile, "专家岗持久化 profile=spirit（违反 org-invariants R17，运行时虽被钳制为 coding）")
	}
	if rawProfile != "" && !isRegisteredToolProfile(rawProfile) {
		issue(ToolAssemblySeverityLow, ToolAssemblyCodeUndefinedProf, fmt.Sprintf("profile=%s 非注册 profile，仅靠归一化兜底", rawProfile))
	}
	if CanonicalToolProfile(rawProfile) == "read_only" {
		var redundant []string
		for _, d := range jsonStringList(settings.ToolsDenyJSON, u.lg) {
			if strings.HasPrefix(d, "memory_") {
				redundant = append(redundant, d)
			}
		}
		if len(redundant) > 0 {
			issue(ToolAssemblySeverityLow, ToolAssemblyCodeRedundantDeny, fmt.Sprintf("read_only 下冗余 deny: %s", strings.Join(redundant, ",")))
		}
	}
	exempt := gov || spirit || toolAssemblyByDesignNoTools[ag.AgentKey] || strings.HasPrefix(ag.AgentKey, toolAssemblyEvalProbeKeyPrefix)
	if !exempt {
		switch {
		case !settings.ToolsEnabled:
			issue(ToolAssemblySeverityMid, ToolAssemblyCodeToolsOff, "tools_enabled=false，全员工具关闭")
		case len(keys) == 0:
			issue(ToolAssemblySeverityHigh, ToolAssemblyCodeZeroTools, fmt.Sprintf("有效工具=0（profile=%s），纯聊天", profileOrEmpty(rawProfile)))
		case len(keys) <= toolAssemblyFewToolsMax:
			issue(ToolAssemblySeverityMid, ToolAssemblyCodeFewTools, fmt.Sprintf("有效工具仅 %d 个: %s", len(keys), strings.Join(keys, ",")))
		}
	}
	if gov && CanonicalToolProfile(effProfile) != "read_only" {
		issue(ToolAssemblySeverityMid, ToolAssemblyCodeGovNotReadonly, fmt.Sprintf("治理岗有效 profile=%s", effProfile))
	}
	return row, issues
}

// toolAssemblyRuntimeSettings 读取 agent 运行设置并标记来源：db = settings 行
// 存在；legacy = 无行时从 config_json 迁移（与 audit.py get_settings 语义一致）。
func (u *AgentUsecase) toolAssemblyRuntimeSettings(ctx context.Context, ag Agent) (AgentRuntimeSettings, string, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, ag.ID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return withSettingDefaults(settingsFromLegacyConfig(ag.ConfigJSON)), "legacy", nil
		}
		return AgentRuntimeSettings{}, "", err
	}
	return withSettingDefaults(settings), "db", nil
}

// isRegisteredToolProfile 判定 profile 是否注册值（含归一化别名：canonical 后
// 命中注册表即视为注册，如 safe/general/system_admin）。
func isRegisteredToolProfile(profile string) bool {
	canon := CanonicalToolProfile(profile)
	if _, ok := toolProfiles[canon]; ok {
		return true
	}
	// spirit 在 toolProfiles 中注册；dept_lead 等未注册值此处返回 false。
	return false
}

func profileOrEmpty(p string) string {
	if p == "" {
		return "∅"
	}
	return p
}
