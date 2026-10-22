// Package biz — Skill 信任分级（P0-7 A14/A15 去 ent 版）
//
// 设计锚点：docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §8.4②③⑤。
// 本文件为纯函数与策略面，不触碰 ent schema / 迁移 SQL / 存储层（列与回填
// 迁移记入 Batch B，待 codegen 协调窗口）。
//
// TrustLevel T1–T4 语义与加载层映射（§8.7 测试规格钉死）：
//
//	T1 builtin   —— 内置种子：全档信任（L1 元数据 + L2 指令注入 + L3 脚本执行）
//	T2 audited   —— 已绑定岗位/已审计：L2 指令 + L3 脚本
//	T3 standard  —— 存量默认：L1 元数据 + L2 指令注入（不可执行脚本）
//	T4 untrusted —— 仅 L1 元数据可见（路由可见，指令/脚本层禁入）
//
// 加载层门槛：L1 需 T4+、L2 需 T3+、L3 需 T2+。
//
// 默认降级策略（fail-closed 方向固定：未知一律往更严档归一）：
//   - 存量 Skill（source 未知/空）默认 T3 可加载——存量兼容保证（§8.5①）；
//   - LLM 演化产物最高 validated、注册为 draft，永不自动 applied
//     （§8.4⑥，Gemini Auto Memory 先例：草稿绝不自动落盘）；
//   - manifest 声明档与来源默认档冲突时取更严档（§8.4③）。
//
// 事件定级（AS-EVT-01 / §9.3）：skill.trust_changed=Important；
// skill.trust_filtered=Informational（仅日志观测，shadow 期不上 bus）。
package biz

import (
	"strings"

	skillpkg "aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/loggateway"
)

// ── TrustLevel / Source ──────────────────────────────────────────────────────

// SkillTrustLevel is the trust tier bound to a skill's origin.
//
// Stability:stable
type SkillTrustLevel string

const (
	SkillTrustT1Builtin   SkillTrustLevel = "T1"
	SkillTrustT2Audited   SkillTrustLevel = "T2"
	SkillTrustT3Standard  SkillTrustLevel = "T3"
	SkillTrustT4Untrusted SkillTrustLevel = "T4"
)

// SkillSource identifies how a skill entered the platform（来源绑定）。
//
// Stability:stable
type SkillSource string

const (
	SkillSourceBuiltin      SkillSource = "builtin"
	SkillSourceLLMEvolution SkillSource = "llm_evolution"
	SkillSourceZipImport    SkillSource = "zip_import"
	SkillSourceUserCreated  SkillSource = "user_created"
)

// Trust event names（AS-EVT-01 定级见文件头注释）。
const (
	SkillEventTrustChanged  = "skill.trust_changed"  // Important
	SkillEventTrustFiltered = "skill.trust_filtered" // Informational
)

// SkillTrust 内嵌 struct（§8.4②，AS-COG-01：收敛 trust/source/audit 三元组，
// 不向 PlatformSkill 平铺字段；schema 列 trust_level/source 由 Batch B 落）。
type SkillTrust struct {
	Level     SkillTrustLevel
	Source    SkillSource
	AuditedBy string
	AuditedAt string // RFC3339；空 = 未审计
}

// ParseSkillTrustLevel parses a stored/declared trust level; unknown values
// report ok=false（调用方据此 fail-closed 归一，绝不静默采信）。
func ParseSkillTrustLevel(s string) (SkillTrustLevel, bool) {
	switch SkillTrustLevel(strings.ToUpper(strings.TrimSpace(s))) {
	case SkillTrustT1Builtin:
		return SkillTrustT1Builtin, true
	case SkillTrustT2Audited:
		return SkillTrustT2Audited, true
	case SkillTrustT3Standard:
		return SkillTrustT3Standard, true
	case SkillTrustT4Untrusted:
		return SkillTrustT4Untrusted, true
	default:
		return "", false
	}
}

// ParseSkillSource parses a stored source tag; unknown values report ok=false.
func ParseSkillSource(s string) (SkillSource, bool) {
	switch SkillSource(strings.ToLower(strings.TrimSpace(s))) {
	case SkillSourceBuiltin:
		return SkillSourceBuiltin, true
	case SkillSourceLLMEvolution:
		return SkillSourceLLMEvolution, true
	case SkillSourceZipImport:
		return SkillSourceZipImport, true
	case SkillSourceUserCreated:
		return SkillSourceUserCreated, true
	default:
		return "", false
	}
}

// TrustRank orders privilege: T1=4（最高）… T4=1，unknown=0（最低，fail-closed）。
func TrustRank(l SkillTrustLevel) int {
	switch l {
	case SkillTrustT1Builtin:
		return 4
	case SkillTrustT2Audited:
		return 3
	case SkillTrustT3Standard:
		return 2
	case SkillTrustT4Untrusted:
		return 1
	default:
		return 0
	}
}

// StricterTrust returns the lower-privilege（更严）of two levels, by rank。
func StricterTrust(a, b SkillTrustLevel) SkillTrustLevel {
	if TrustRank(a) <= TrustRank(b) {
		return a
	}
	return b
}

// DefaultTrustForSource 默认降级策略（§8.4⑤ 迁移默认分级的纯函数面）：
//
//	builtin       → T1（内置种子全档信任）
//	zip_import    → T4（外部供应链最严档，社区 Skill 风险见对比报告）
//	llm_evolution → T3（可加载至 L2；落户上限另行收窄为 validated）
//	user_created  → T3
//	未知/空 source → T3（存量 Skill 默认 T3 可加载——存量兼容保证，§8.5①）
func DefaultTrustForSource(s SkillSource) SkillTrustLevel {
	switch s {
	case SkillSourceBuiltin:
		return SkillTrustT1Builtin
	case SkillSourceZipImport:
		return SkillTrustT4Untrusted
	default:
		return SkillTrustT3Standard
	}
}

// ResolveSkillTrust resolves the effective level：来源默认档与 manifest/DB
// 声明档取更严档（fail-closed，§8.4③）。声明档为空或无法解析时直接采用
// 来源默认档（不视为声明）。
func ResolveSkillTrust(source SkillSource, declared string) SkillTrustLevel {
	base := DefaultTrustForSource(source)
	d, ok := ParseSkillTrustLevel(declared)
	if !ok {
		return base
	}
	return StricterTrust(base, d)
}

// DefaultLegacySkillTrust：存量 Skill 默认信任包（T3 可加载）。
func DefaultLegacySkillTrust() SkillTrust {
	return SkillTrust{Level: SkillTrustT3Standard}
}

// ── 加载层门槛（TrustLevel → ToolsProfile 工具面映射）────────────────────────

// SkillLoadLayer 为 Skill 内容进入 Agent 工具面的分层（org-invariants.md:33
// 工具面=ToolsProfile+Allow/Deny 的信任维度投影）。
type SkillLoadLayer string

const (
	SkillLoadLayerL1Metadata    SkillLoadLayer = "L1" // 元数据（路由概览）
	SkillLoadLayerL2Instruction SkillLoadLayer = "L2" // 指令注入
	SkillLoadLayerL3Script      SkillLoadLayer = "L3" // 脚本执行
)

// RequiredTrustForLayer 返回进入加载层的最低信任档：
// L1→T4、L2→T3、L3→T2；未知层 fail-closed 取 T1（最严）。
func RequiredTrustForLayer(layer SkillLoadLayer) SkillTrustLevel {
	switch layer {
	case SkillLoadLayerL1Metadata:
		return SkillTrustT4Untrusted
	case SkillLoadLayerL2Instruction:
		return SkillTrustT3Standard
	case SkillLoadLayerL3Script:
		return SkillTrustT2Audited
	default:
		return SkillTrustT1Builtin
	}
}

// CanLoadAtLayer reports whether level meets the layer's minimal trust.
func CanLoadAtLayer(level SkillTrustLevel, layer SkillLoadLayer) bool {
	return TrustRank(level) >= TrustRank(RequiredTrustForLayer(layer))
}

// SkillToolSurface 是信任档到 Agent 工具面（ToolsProfile 维度）的纯函数映射：
// 各层可见性由 CanLoadAtLayer 推导，单一推导点避免两处各自维护漂移。
type SkillToolSurface struct {
	MetadataVisible   bool // L1
	InstructionInject bool // L2
	ScriptExecute     bool // L3
}

// ToolSurfaceForTrust maps a resolved trust level to its tool surface.
// 未知/空 level rank=0 → 三层全禁（fail-closed）。
func ToolSurfaceForTrust(level SkillTrustLevel) SkillToolSurface {
	return SkillToolSurface{
		MetadataVisible:   CanLoadAtLayer(level, SkillLoadLayerL1Metadata),
		InstructionInject: CanLoadAtLayer(level, SkillLoadLayerL2Instruction),
		ScriptExecute:     CanLoadAtLayer(level, SkillLoadLayerL3Script),
	}
}

// ── 演化产物落户规则（§8.4⑥，即时生效的防护语义，纯函数面先落）──────────────

// LLMEvolutionLandingMaxState：LLM 演化产物落户上限为 validated、注册为
// draft——applied 必须经人工审批（learning_loop ApproveProposal CAS，A15⑤）。
func LLMEvolutionLandingMaxState() skillpkg.SkillState {
	return skillpkg.SkillStateValidated
}

// EvolutionAutoApplyAllowed：演化产物（任意来源）一律不自动 applied。
func EvolutionAutoApplyAllowed(source SkillSource) bool {
	return false
}

// ── 信任门禁（shadow，A15：永不真拒）────────────────────────────────────────

// TrustGateMode 为门禁三档（off→shadow→on）的模式位。
//
// 去 ent 版硬约束：on 在本批不接线执行过滤——规范化时归一为 shadow 并
// 上报警告日志（设计 §8.8 defer②：on 需 shadow 期数据证明零误拒后随
// Batch B 开启）。
type TrustGateMode string

const (
	TrustGateModeOff    TrustGateMode = "off"
	TrustGateModeShadow TrustGateMode = "shadow"
	TrustGateModeOn     TrustGateMode = "on"
)

// NormalizeTrustGateMode 归一化配置输入（沿用 config.go fail-closed 归一先例）：
// ""→off；"shadow"→shadow；"on"→shadow + degraded=true（本轮永不真拒）；
// 未知值→off + degraded=true（观测与执行都从严靠安全侧：不新增过滤，仅提示）。
func NormalizeTrustGateMode(raw string) (mode TrustGateMode, degraded bool) {
	switch TrustGateMode(strings.ToLower(strings.TrimSpace(raw))) {
	case TrustGateModeOff, "":
		return TrustGateModeOff, false
	case TrustGateModeShadow:
		return TrustGateModeShadow, false
	case TrustGateModeOn:
		return TrustGateModeShadow, true
	default:
		return TrustGateModeOff, true
	}
}

// EvaluateTrustGate 是 shadow 门禁的纯判定函数：
// 解析候选的来源/声明信任 → 与 required（加载层门槛）比较。
// wouldBlock=true 表示若门禁处于 on 档将被拒——shadow 下仅用于计数与日志，
// 永不在求值层产生过滤效果。
func EvaluateTrustGate(source string, declared string, required SkillTrustLevel) (resolved SkillTrustLevel, wouldBlock bool) {
	src, _ := ParseSkillSource(source) // 未知 source → 空 → 默认 T3（存量兼容）
	resolved = ResolveSkillTrust(src, declared)
	return resolved, TrustRank(resolved) < TrustRank(required)
}

// NewTrustGateHook 构造加载链 shadow 上报闭包（注入 skill.Usecase。
// SetTrustGateHook）：off 直接返回 nil（零行为变更）；shadow/降级项逐条
// would-block 计数并以结构化字段 Warn 上报（skill.trust_filtered=
// Informational，仅日志，永不上 bus 放大——AS-EVT-01）。
func NewTrustGateHook(lg loggateway.Logger, mode TrustGateMode, required SkillTrustLevel) skillpkg.TrustGateHook {
	norm, degraded := NormalizeTrustGateMode(string(mode))
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("skill_trust_gate"))
	if degraded {
		lg.Warn("skill trust gate mode degraded (enforcement deferred to Batch B; running as shadow/off)",
			loggateway.StepID("skill.trust_gate.mode_degraded"),
			loggateway.Str("mode", string(mode)),
			loggateway.Str("effective_mode", string(norm)),
			loggateway.Bool("would_block_only", true))
	}
	if norm == TrustGateModeOff {
		return nil
	}
	return func(c skillpkg.RuntimeCandidate) {
		resolved, wouldBlock := EvaluateTrustGate(c.Source, c.TrustLevel, required)
		if !wouldBlock {
			return // 通过候选举静默——零噪声（比对估算影子先例）
		}
		lg.Warn("skill would be blocked by trust gate (shadow; not enforced)",
			loggateway.StepID(SkillEventTrustFiltered),
			loggateway.Str("skill_id", c.SkillID),
			loggateway.Str("slug", c.Slug),
			loggateway.Str("trust_level", string(resolved)),
			loggateway.Str("required_level", string(required)),
			loggateway.Str("source", c.Source),
			loggateway.Bool("would_block", true))
	}
}
