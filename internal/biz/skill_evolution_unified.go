package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

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
// Stability:evolving
type UnifiedEvolutionQueryReader interface {
	GetByID(ctx context.Context, id string) (*UnifiedEvolutionSuggestion, error)
	ListByTarget(ctx context.Context, targetType string, targetID string, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error)
	CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error)
	ListByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error)
	CountByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string, status string) (int, error)
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
	queryReader UnifiedEvolutionQueryReader
	writer      UnifiedEvolutionWriter
	triggers    []EvolutionTrigger
	triggersMu  sync.RWMutex                  // protects triggers for concurrent RegisterTrigger calls
	unifiedSM   *UnifiedEvolutionStateMachine // AS-FSM-01: validates status transitions
	lg          loggateway.Logger
}

func NewSkillEvolutionOrchestrator(
	checkReader UnifiedEvolutionCheckReader,
	queryReader UnifiedEvolutionQueryReader,
	writer UnifiedEvolutionWriter,
	lg loggateway.Logger,
) *SkillEvolutionOrchestrator {
	return &SkillEvolutionOrchestrator{
		checkReader: checkReader,
		queryReader: queryReader,
		writer:      writer,
		unifiedSM:   NewUnifiedEvolutionStateMachine(),
		lg:          lg,
	}
}

// RegisterTrigger 注册触发器。
// 线程安全：可在初始化期间或运行时动态注册。
func (o *SkillEvolutionOrchestrator) RegisterTrigger(trigger EvolutionTrigger) {
	o.triggersMu.Lock()
	defer o.triggersMu.Unlock()
	o.triggers = append(o.triggers, trigger)
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
				cooldownEnd := latestByAction.CreatedAt.Add(EvoTriggerCooldownHours * time.Hour)
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

// Approve 审批建议
func (o *SkillEvolutionOrchestrator) Approve(ctx context.Context, id string, approvedBy string) error {
	s, err := o.queryReader.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s == nil {
		return apierror.NotFound("EVO_ORCHESTRATOR", "suggestion not found")
	}
	// AS-FSM-01: validate transition via state machine instead of direct string comparison.
	if _, err := o.unifiedSM.Transition(ParseUnifiedEvolutionState(s.Status), UnifiedEvolutionEventApprove); err != nil {
		return apierror.BadRequest("EVO_ORCHESTRATOR", "only pending suggestions can be approved, current status: "+s.Status)
	}
	return o.writer.UpdateStatus(ctx, id, "approved", approvedBy, "")
}

// Reject 拒绝建议
func (o *SkillEvolutionOrchestrator) Reject(ctx context.Context, id string, rejectedBy string, reason string) error {
	s, err := o.queryReader.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s == nil {
		return apierror.NotFound("EVO_ORCHESTRATOR", "suggestion not found")
	}
	// AS-FSM-01: validate transition via state machine instead of direct string comparison.
	if _, err := o.unifiedSM.Transition(ParseUnifiedEvolutionState(s.Status), UnifiedEvolutionEventReject); err != nil {
		return apierror.BadRequest("EVO_ORCHESTRATOR", "only pending suggestions can be rejected, current status: "+s.Status)
	}
	return o.writer.UpdateStatus(ctx, id, "rejected", rejectedBy, reason)
}

// ExpirePending 过期 pending 建议（超过 7 天）
func (o *SkillEvolutionOrchestrator) ExpirePending(ctx context.Context) (int, error) {
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
