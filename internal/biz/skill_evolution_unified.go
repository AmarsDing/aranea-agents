package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// evolutionCallerWorkspace 解析调用方 workspace 用于 List/Count 租户过滤：
// 系统调用方（cron/admin，WithSystemWorkspace）返回 ""（不过滤）；
// 其余返回 ctx 中的 workspace（未设置时回退 default）。
// 与 session_search / agent_mcp_effective 的既有先例一致。
func evolutionCallerWorkspace(ctx context.Context) string {
	if workspace.IsSystem(ctx) {
		return ""
	}
	return workspace.IDFromContext(ctx)
}

// EvolutionTargetType 进化目标类型
type EvolutionTargetType string

const (
	EvolutionTargetSkill EvolutionTargetType = "skill"
	EvolutionTargetAgent EvolutionTargetType = "agent"
)

// EvolutionActionType 进化动作类型
type EvolutionActionType string

const (
	EvolutionActionCreate  EvolutionActionType = "create_skill"
	EvolutionActionImprove EvolutionActionType = "improve_skill"
	EvolutionActionMerge   EvolutionActionType = "merge_skill"
	EvolutionActionEvolve  EvolutionActionType = "evolve_agent"
)

// UnifiedEvolutionSuggestion 统一进化建议
type UnifiedEvolutionSuggestion struct {
	ID            string
	TargetType    EvolutionTargetType
	TargetID      string
	// WorkspaceID 归属租户；空串 = 共享/平台级（所有租户可见）。
	// 写入侧由 data 层 Create 在为空时从宿主表（skill/agents）派生。
	WorkspaceID   string
	ActionType    EvolutionActionType
	TriggerSource string // "pattern" / "health" / "agent_config" / "manual"
	TriggerReason string
	Status        string // pending / approved / rejected / applied / expired
	Priority      int    // 0=低, 1=中, 2=高, 3=紧急

	DraftBody     string
	DraftName     string // create 时建议的名称
	MergeTargetID string // merge 时的目标 ID

	LifecycleStatus string // draft / validating / ready
	SandboxPassed   bool
	SandboxResult   json.RawMessage

	Metadata json.RawMessage // 类型特定数据（如 pattern_hash, failure_tags 等）

	CreatedAt  time.Time
	ApprovedBy string
	AppliedAt  *time.Time
}

// ── Metadata keys (A6 legacy view layer) ─────────────────────────────────────
//
// After the four-store convergence, legacy-only fields live in the Metadata
// JSON column. The constants below are the contract between the backfill
// migration (20261111), the writers in this package, and the service-layer
// view conversion that reconstructs legacy proto messages.
const (
	// L1 skill_proposals
	EvoMetaPatternHash = "pattern_hash"
	EvoMetaPatternDesc = "pattern_desc"
	EvoMetaApprovedAt  = "approved_at"
	// L1 + L2 shared
	EvoMetaRejectedBy      = "rejected_by"
	EvoMetaRejectionReason = "rejection_reason"
	// L2 skill_evolution_suggestions
	EvoMetaResolvedAt      = "resolved_at"
	EvoMetaSourceReportIDs = "source_report_ids"
	EvoMetaDraftVersionID  = "draft_version_id"
	EvoMetaParentVersionID = "parent_version_id"
	EvoMetaEvolutionReason = "evolution_reason"
	EvoMetaPreVerifyResult = "pre_verify_result"
	// L3 evolution_suggestions
	EvoMetaLegacyType       = "legacy_type"
	EvoMetaTitle            = "title"
	EvoMetaDiffPreview      = "diff_preview"
	EvoMetaPreApplySnapshot = "pre_apply_snapshot"
	// EvoMetaApplyPayload 持有可被 ApplySuggestion 真正写入 prompt 文件的内容。
	// 指标通知类建议（AgentConfigTrigger / 编排优化建议）不设置此键，apply
	// 一律拒绝——防止把"近30d负反馈 N 次…"之类的通知文本写进 IDENTITY.md /
	// AGENTS*.md（2026-08-07 P0-2 根因修复）。未来 LLM 草稿生成器产出实质
	// 修改内容时设置此键即解锁 apply。
	EvoMetaApplyPayload = "apply_payload"
	// EvoMetaCreatedFiles 记录 apply 过程中新建的 prompt 文件名列表（JSON
	// array string），rollback 据此精确删除 apply 新增文件，避免残留（P1-2）。
	EvoMetaCreatedFiles = "created_files"
	// P1 delta 协议与计数归因（skill 路径）
	// EvoMetaBaselineSuccessRate 记录触发时的基线成功率（7d 优先，无 7d 数据
	// 时用 30d 值），JSON number，供下一周期 AttributeLastEvolution 裁决。
	EvoMetaBaselineSuccessRate = "baseline_success_rate"
	// EvoMetaDeltaOps 记录本次 draft 实际应用的 delta 操作序列 JSON（仅
	// delta 模式），供归因提取 AffectedRuleIDs。
	EvoMetaDeltaOps = "delta_ops"
	// EvoMetaEffectiveness 记录归因裁决（helpful/harmful/neutral/
	// insufficient_data），由下一进化周期回写到最近一条 applied 建议。
	EvoMetaEffectiveness = "effectiveness"
	// EvoMetaDraftOrigin 记录草稿来源（llm / rule_template），F8：让
	// evolver=nil 的模板降级在 API 响应中可观测，不再静默。
	EvoMetaDraftOrigin = "draft_origin"
	// EvoMetaDraftAttemptAt 记录 L3 通知建议最近一次 LLM 草稿生成尝试时间
	//（RFC3339，成功或失败都记），EvolutionDrafter 据此做 1h 节流。
	EvoMetaDraftAttemptAt = "draft_attempt_at"
)

// MetadataMap decodes Metadata into a map. Returns an empty map when Metadata
// is nil or invalid; callers must not mutate the returned RawMessages.
func (s *UnifiedEvolutionSuggestion) MetadataMap() map[string]json.RawMessage {
	if s == nil || len(s.Metadata) == 0 {
		return map[string]json.RawMessage{}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(s.Metadata, &m); err != nil {
		return map[string]json.RawMessage{}
	}
	return m
}

// MetaString returns the JSON-string value at key, or "" when the key is
// absent, null, or not a JSON string.
func (s *UnifiedEvolutionSuggestion) MetaString(key string) string {
	raw, ok := s.MetadataMap()[key]
	if !ok {
		return ""
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return v
}

// MetaRaw returns the raw JSON value at key, or nil when absent/null.
func (s *UnifiedEvolutionSuggestion) MetaRaw(key string) json.RawMessage {
	raw, ok := s.MetadataMap()[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	return raw
}

// CountsForCooldown reports whether the suggestion's status participates in the
// trigger cooldown window (F9): only active lifecycle states (pending /
// approved / applied) suppress re-triggering. A rejected, expired or rolled
// back suggestion must not waste the skill's evolution opportunity for a week.
func (s *UnifiedEvolutionSuggestion) CountsForCooldown() bool {
	switch UnifiedEvolutionState(s.Status) {
	case UnifiedEvolutionStatePending, UnifiedEvolutionStateApproved, UnifiedEvolutionStateApplied:
		return true
	default:
		return false
	}
}

// UnifiedEvolutionCheckReader provides pending-check and latest-lookup queries.
// Stability:evolving
type UnifiedEvolutionCheckReader interface {
	HasPendingForTarget(ctx context.Context, targetType string, targetID string) (bool, error)
	GetLatestByTarget(ctx context.Context, targetType string, targetID string) (*UnifiedEvolutionSuggestion, error)
	GetLatestByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string) (*UnifiedEvolutionSuggestion, error)
}

// UnifiedEvolutionQueryReader provides by-ID, list, and count queries.
// The AndAction variants disambiguate views that share a target_type — e.g.
// L1 proposals (agent + create_skill) vs L3 agent suggestions (agent +
// evolve_agent) after the A6 physical convergence.
//
// workspaceID 过滤语义（P0-1b 租户隔离）：非空时仅返回该租户自有行 +
// workspace_id='' 的共享行；空串 = 不过滤（系统/后台任务专用，调用方必须
// 确保不直接把结果暴露给租户请求）。usecase 层经 workspace.IDFromContext
// 解析后传入。
// Stability:evolving
type UnifiedEvolutionQueryReader interface {
	GetByID(ctx context.Context, id string) (*UnifiedEvolutionSuggestion, error)
	ListByTarget(ctx context.Context, targetType string, targetID string, workspaceID string, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error)
	CountByTarget(ctx context.Context, targetType string, targetID string, workspaceID string, status string) (int, error)
	ListByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, workspaceID string, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error)
	CountByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, workspaceID string, status string) (int, error)
}

// UnifiedEvolutionPatternReader provides the L1 pattern-hash dedup lookup
// (pattern_hash lives in metadata after the A6 convergence).
// Stability:evolving
type UnifiedEvolutionPatternReader interface {
	GetLatestByPatternHash(ctx context.Context, agentID string, patternHash string) (*UnifiedEvolutionSuggestion, error)
}

// UnifiedEvolutionMutationWriter provides create and status/draft/lifecycle/sandbox mutations.
// Stability:evolving
type UnifiedEvolutionMutationWriter interface {
	Create(ctx context.Context, suggestion UnifiedEvolutionSuggestion) error
	UpdateStatus(ctx context.Context, id string, status string, actor string, reason string) error
	// UpdateStatusCAS atomically transitions a suggestion to `to` only when its
	// current status is in `from`. Returns ok=false when the precondition does
	// not hold (state changed concurrently or source state is not allowed).
	// Callers must validate the transition against UnifiedEvolutionStateMachine
	// before invoking; this method is the race guard, not the state machine.
	UpdateStatusCAS(ctx context.Context, id string, from []string, to string, actor string, reason string) (ok bool, err error)
	UpdateDraftBody(ctx context.Context, id string, draftBody string) error
	UpdateLifecycleStatus(ctx context.Context, id string, lifecycleStatus string) error
	UpdateSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error
}

// UnifiedEvolutionMetadataWriter provides single-key JSON metadata updates
// (e.g. the L3 pre_apply_snapshot saved before applying a suggestion).
// Stability:evolving
type UnifiedEvolutionMetadataWriter interface {
	UpdateMetadataKey(ctx context.Context, id string, key string, value string) error
}

// UnifiedEvolutionExpirationWriter provides expiration of old pending suggestions.
// Stability:evolving
type UnifiedEvolutionExpirationWriter interface {
	ExpireOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

// UnifiedEvolutionPendingTarget identifies a distinct (target_type, target_id)
// pair that currently holds pending suggestions.
// Stability:evolving
type UnifiedEvolutionPendingTarget struct {
	TargetType string
	TargetID   string
}

// UnifiedEvolutionPendingTargetLister lists distinct targets holding pending
// suggestions; drives per-target expiration with per-agent TTL.
// Stability:evolving
type UnifiedEvolutionPendingTargetLister interface {
	ListPendingTargets(ctx context.Context) ([]UnifiedEvolutionPendingTarget, error)
}

// UnifiedEvolutionTargetExpirationWriter expires pending suggestions scoped to
// a single target (per-agent proposal TTL).
// Stability:evolving
type UnifiedEvolutionTargetExpirationWriter interface {
	ExpireOlderThanForTarget(ctx context.Context, targetType, targetID string, cutoff time.Time) (int, error)
}

// UnifiedEvolutionReader 统一进化建议的读接口 (composition of sub-interfaces).
type UnifiedEvolutionReader interface {
	UnifiedEvolutionCheckReader
	UnifiedEvolutionQueryReader
}

// UnifiedEvolutionWriter 统一进化建议的写接口 (composition of sub-interfaces).
type UnifiedEvolutionWriter interface {
	UnifiedEvolutionMutationWriter
	UnifiedEvolutionMetadataWriter
	UnifiedEvolutionExpirationWriter
}

// UnifiedEvolutionStore combines unified evolution reading and writing.
// Stability:evolving
type UnifiedEvolutionStore interface {
	UnifiedEvolutionReader
	UnifiedEvolutionWriter
}

// EvolutionTrigger 进化触发器接口
type EvolutionTrigger interface {
	TargetType() EvolutionTargetType
	ActionType() EvolutionActionType
	TriggerSource() string
	Check(ctx context.Context, targetID string) ([]UnifiedEvolutionSuggestion, error)
}

// SkillEvolutionOrchestrator 统一进化编排器
type SkillEvolutionOrchestrator struct {
	checkReader UnifiedEvolutionCheckReader
	writer      UnifiedEvolutionWriter
	triggers    []EvolutionTrigger
	triggersMu  sync.RWMutex // protects triggers for concurrent RegisterTrigger calls
	lg          loggateway.Logger

	// cooldownMul holds per-trigger-source cooldown escalations (D8 自适应
	// 降频：连续非 effective outcomes → 冷却期 ×2，上限 8×)。内存为热路径；
	// cooldownStore 非 nil 时跨重启持久化到 system_settings。
	cooldownMu    sync.RWMutex
	cooldownMul   map[string]float64
	cooldownStore SITriggerCooldownStore

	// expireResolver 解析单个 target 的 pending 建议 TTL（Agent 维度读
	// evo_proposal_ttl_days）；nil 时 ExpirePending 回退全局统一过期常量。
	expireResolver func(ctx context.Context, targetType, targetID string) time.Duration
}

func NewSkillEvolutionOrchestrator(
	checkReader UnifiedEvolutionCheckReader,
	writer UnifiedEvolutionWriter,
	lg loggateway.Logger,
) *SkillEvolutionOrchestrator {
	return &SkillEvolutionOrchestrator{
		checkReader: checkReader,
		writer:      writer,
		lg:          lg,
	}
}

// SetExpirationResolver 接线按 target 的过期时长解析器（可选）。
// resolver 返回 <= 0 时该 target 回退全局默认 evoExpirationDuration。
func (o *SkillEvolutionOrchestrator) SetExpirationResolver(resolver func(ctx context.Context, targetType, targetID string) time.Duration) {
	o.expireResolver = resolver
}

// RegisterTrigger 注册触发器。
// 线程安全：可在初始化期间或运行时动态注册。
func (o *SkillEvolutionOrchestrator) RegisterTrigger(trigger EvolutionTrigger) {
	o.triggersMu.Lock()
	defer o.triggersMu.Unlock()
	o.triggers = append(o.triggers, trigger)
}

// SITriggerCooldownStore persists per-trigger-source cooldown multipliers
// across process restarts (D8). Empty/missing map = all multipliers 1×.
// Stability:evolving
type SITriggerCooldownStore interface {
	LoadTriggerCooldownMultipliers(ctx context.Context) (map[string]float64, error)
	SaveTriggerCooldownMultipliers(ctx context.Context, multipliers map[string]float64) error
}

// siMaxTriggerCooldownMultiplier caps the D8 adaptive cooldown escalation
// (×2 per consecutive non-effective window, at most 8×).
const siMaxTriggerCooldownMultiplier = 8.0

// AttachCooldownStore binds the restart-durable store. Call once at
// construction (before HydrateTriggerCooldowns). nil is a no-op (in-memory).
func (o *SkillEvolutionOrchestrator) AttachCooldownStore(store SITriggerCooldownStore) {
	if o == nil {
		return
	}
	o.cooldownStore = store
}

// HydrateTriggerCooldowns loads persisted multipliers into memory (simulate
// restart by constructing a new orchestrator and calling this). Missing store
// or empty payload leaves all sources at 1×.
func (o *SkillEvolutionOrchestrator) HydrateTriggerCooldowns(ctx context.Context) error {
	if o == nil || o.cooldownStore == nil {
		return nil
	}
	raw, err := o.cooldownStore.LoadTriggerCooldownMultipliers(ctx)
	if err != nil {
		return err
	}
	cleaned := sanitizeTriggerCooldownMultipliers(raw)
	o.cooldownMu.Lock()
	o.cooldownMul = cleaned
	n := len(cleaned)
	o.cooldownMu.Unlock()
	if n > 0 {
		o.lg.Info("orchestrator: trigger cooldown multipliers hydrated",
			loggateway.StepID("evo_orchestrator.cooldown_hydrate"),
			loggateway.Int("sources", n))
	}
	return nil
}

// SetTriggerCooldownMultiplier multiplies the cooldown of one trigger source
// by factor (D8 触发器自适应降频). The effective multiplier is capped at
// siMaxTriggerCooldownMultiplier. factor <= 1 is a no-op. Safe for concurrent
// use. When a cooldown store is attached the map is persisted after each
// successful escalation (failure is logged, memory still updated).
func (o *SkillEvolutionOrchestrator) SetTriggerCooldownMultiplier(triggerSource string, factor float64) {
	if o == nil || triggerSource == "" || factor <= 1 {
		return
	}
	o.cooldownMu.Lock()
	if o.cooldownMul == nil {
		o.cooldownMul = map[string]float64{}
	}
	cur := o.cooldownMul[triggerSource]
	if cur <= 0 {
		cur = 1
	}
	cur *= factor
	if cur > siMaxTriggerCooldownMultiplier {
		cur = siMaxTriggerCooldownMultiplier
	}
	o.cooldownMul[triggerSource] = cur
	snapshot := cloneTriggerCooldownMultipliers(o.cooldownMul)
	store := o.cooldownStore
	o.cooldownMu.Unlock()

	o.lg.Info("orchestrator: trigger cooldown escalated",
		loggateway.StepID("evo_orchestrator.cooldown_escalate"),
		loggateway.Str("trigger_source", triggerSource),
		loggateway.Str("multiplier", fmt.Sprintf("%.1f", cur)))
	if store == nil {
		return
	}
	if err := store.SaveTriggerCooldownMultipliers(context.Background(), snapshot); err != nil {
		o.lg.Warn("orchestrator: persist cooldown multipliers failed",
			loggateway.StepID("evo_orchestrator.cooldown_persist"),
			loggateway.Err(err))
	}
}

// sanitizeTriggerCooldownMultipliers drops empty keys and non-finite / ≤1
// values, and caps each remaining multiplier at siMaxTriggerCooldownMultiplier.
func sanitizeTriggerCooldownMultipliers(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		if strings.TrimSpace(k) == "" || math.IsNaN(v) || math.IsInf(v, 0) || v <= 1 {
			continue
		}
		if v > siMaxTriggerCooldownMultiplier {
			v = siMaxTriggerCooldownMultiplier
		}
		out[k] = v
	}
	return out
}

func cloneTriggerCooldownMultipliers(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// triggerCooldownMultiplier returns the effective cooldown multiplier of one
// trigger source (default 1).
func (o *SkillEvolutionOrchestrator) triggerCooldownMultiplier(triggerSource string) float64 {
	o.cooldownMu.RLock()
	defer o.cooldownMu.RUnlock()
	if m := o.cooldownMul[triggerSource]; m > 0 {
		return m
	}
	return 1
}

// HasPendingForTarget checks whether a pending suggestion exists for the given target.
// Delegates to the internal checkReader.
func (o *SkillEvolutionOrchestrator) HasPendingForTarget(ctx context.Context, targetType, targetID string) (bool, error) {
	return o.checkReader.HasPendingForTarget(ctx, targetType, targetID)
}

// CheckAndCreate 原子化检查+创建
// 1. 检查是否已有 pending 建议
// 2. 遍历触发器，收集所有触发的建议并逐个创建（含 per-action-type 冷却期检查）
// DB UNIQUE 约束兜底防止并发重复创建
func (o *SkillEvolutionOrchestrator) CheckAndCreate(ctx context.Context, targetType EvolutionTargetType, targetID string) ([]UnifiedEvolutionSuggestion, error) {
	// 1. 检查是否已有 pending 建议
	hasPending, err := o.checkReader.HasPendingForTarget(ctx, string(targetType), targetID)
	if err != nil {
		o.lg.Warn("orchestrator: check pending failed",
			loggateway.StepID("evo_orchestrator.check"),
			loggateway.Err(err))
		// 保守策略：查询失败时不阻止，让 DB 约束兜底
	} else if hasPending {
		return nil, nil
	}

	// 2. 遍历触发器
	o.triggersMu.RLock()
	snapshot := make([]EvolutionTrigger, len(o.triggers))
	copy(snapshot, o.triggers)
	o.triggersMu.RUnlock()

	var created []UnifiedEvolutionSuggestion
	for _, trigger := range snapshot {
		if trigger.TargetType() != targetType {
			continue
		}
		suggestions, err := trigger.Check(ctx, targetID)
		if err != nil {
			o.lg.Warn("orchestrator: trigger check failed",
				loggateway.StepID("evo_orchestrator.trigger"),
				loggateway.Str("trigger_source", trigger.TriggerSource()),
				loggateway.Err(err))
			continue
		}
		if len(suggestions) == 0 {
			continue
		}

		// 3. 逐个创建（DB UNIQUE 约束兜底）
		for _, suggestion := range suggestions {
			// Per-action-type cooldown check: each (targetType, targetID, actionType)
			// triple has its own independent cooldown period. F9: only active
			// lifecycle states count — a rejected/expired suggestion never blocks.
			latestByAction, lbErr := o.checkReader.GetLatestByTargetAndAction(ctx, string(suggestion.TargetType), suggestion.TargetID, string(suggestion.ActionType))
			if lbErr == nil && latestByAction != nil && latestByAction.CountsForCooldown() {
				cooldownHours := time.Duration(float64(EvoTriggerCooldownHours) * o.triggerCooldownMultiplier(suggestion.TriggerSource))
				cooldownEnd := latestByAction.CreatedAt.Add(cooldownHours * time.Hour)
				if time.Now().UTC().Before(cooldownEnd) {
					o.lg.Debug("orchestrator: cooldown active for action type, skipping",
						loggateway.StepID("evo_orchestrator.cooldown"),
						loggateway.Str("action_type", string(suggestion.ActionType)))
					continue
				}
			}

			if createErr := o.writer.Create(ctx, suggestion); createErr != nil {
				if isDuplicateKeyError(createErr) {
					o.lg.Debug("orchestrator: concurrent creation detected, skipping",
						loggateway.StepID("evo_orchestrator.create"))
					continue
				}
				return created, createErr
			}

			o.lg.Info("orchestrator: evolution suggestion created",
				loggateway.StepID("evo_orchestrator.create"),
				loggateway.Str("target_type", string(targetType)),
				loggateway.Str("target_id", targetID),
				loggateway.Str("action", string(suggestion.ActionType)),
				loggateway.Str("trigger_source", suggestion.TriggerSource))

			created = append(created, suggestion)
		}
	}
	return created, nil
}

// ExpirePending 过期 pending 建议：已接线 resolver 且存储支持 per-target 过期时，
// 按 target 独立 TTL（Agent 维度读 evo_proposal_ttl_days）；否则回退全局统一过期
// （EvoExpirationDays 常量，兼容未升级存储/测试桩）。
func (o *SkillEvolutionOrchestrator) ExpirePending(ctx context.Context) (int, error) {
	lister, canList := o.checkReader.(UnifiedEvolutionPendingTargetLister)
	targetWriter, canExpirePerTarget := o.writer.(UnifiedEvolutionTargetExpirationWriter)
	if o.expireResolver == nil || !canList || !canExpirePerTarget {
		return o.expirePendingGlobal(ctx)
	}
	targets, err := lister.ListPendingTargets(ctx)
	if err != nil {
		o.lg.Warn("orchestrator: ListPendingTargets failed",
			loggateway.StepID("evo_orchestrator.expire"),
			loggateway.Err(err))
		return 0, err
	}
	now := time.Now().UTC()
	total := 0
	for _, target := range targets {
		ttl := o.expireResolver(ctx, target.TargetType, target.TargetID)
		if ttl <= 0 {
			ttl = evoExpirationDuration
		}
		n, err := targetWriter.ExpireOlderThanForTarget(ctx, target.TargetType, target.TargetID, now.Add(-ttl))
		if err != nil {
			// 单 target 失败不阻塞其余 target 的过期扫描。
			o.lg.Warn("orchestrator: per-target expire failed",
				loggateway.StepID("evo_orchestrator.expire"),
				loggateway.Err(err))
			continue
		}
		total += n
	}
	if total > 0 {
		o.lg.Info("orchestrator: expired pending suggestions",
			loggateway.StepID("evo_orchestrator.expire"),
			loggateway.Int("count", total))
	}
	return total, nil
}

// expirePendingGlobal 过期 pending 建议（统一超过 EvoExpirationDays，不区分 target）。
func (o *SkillEvolutionOrchestrator) expirePendingGlobal(ctx context.Context) (int, error) {
	cutoff := time.Now().UTC().Add(-evoExpirationDuration)
	expired, err := o.writer.ExpireOlderThan(ctx, cutoff)
	if err != nil {
		o.lg.Warn("orchestrator: ExpirePending failed",
			loggateway.StepID("evo_orchestrator.expire"),
			loggateway.Err(err))
		return 0, err
	}
	if expired > 0 {
		o.lg.Info("orchestrator: expired pending suggestions",
			loggateway.StepID("evo_orchestrator.expire"),
			loggateway.Int("count", expired))
	}
	return expired, nil
}

// CreateSuggestion creates a single UnifiedEvolutionSuggestion directly.
// Use this when a caller (e.g. LearningLoop) has already determined the
// suggestion content and just needs to persist it through the unified pipeline.
// DB UNIQUE constraint handles concurrent duplicate prevention.
func (o *SkillEvolutionOrchestrator) CreateSuggestion(ctx context.Context, suggestion UnifiedEvolutionSuggestion) error {
	if err := o.writer.Create(ctx, suggestion); err != nil {
		if isDuplicateKeyError(err) {
			o.lg.Debug("orchestrator: concurrent creation detected, skipping",
				loggateway.StepID("evo_orchestrator.create_suggestion"))
			return nil
		}
		return err
	}
	o.lg.Info("orchestrator: evolution suggestion created",
		loggateway.StepID("evo_orchestrator.create_suggestion"),
		loggateway.Str("target_type", string(suggestion.TargetType)),
		loggateway.Str("target_id", suggestion.TargetID),
		loggateway.Str("action", string(suggestion.ActionType)),
		loggateway.Str("trigger_source", suggestion.TriggerSource))
	return nil
}

// isDuplicateKeyError 检查是否为 DB 唯一约束冲突错误。
// Postgres 路径经 entErrToBizErr 已译为 CodeConflict；SQLite 裸 SQL 路径
// 落入 CodeInternal，需按驱动错误文案兜底匹配。
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeConflict {
		return true
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate")
}
