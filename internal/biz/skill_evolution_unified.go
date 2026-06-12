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

// UnifiedEvolutionCheckReader provides pending-check and latest-lookup queries.
// Stability:evolving
type UnifiedEvolutionCheckReader interface {
	HasPendingForTarget(ctx context.Context, targetType string, targetID string) (bool, error)
	GetLatestByTarget(ctx context.Context, targetType string, targetID string) (*UnifiedEvolutionSuggestion, error)
	GetLatestByTargetAndAction(ctx context.Context, targetType string, targetID string, actionType string) (*UnifiedEvolutionSuggestion, error)
}

// UnifiedEvolutionQueryReader provides by-ID, list, and count queries.
// Stability:evolving
type UnifiedEvolutionQueryReader interface {
	GetByID(ctx context.Context, id string) (*UnifiedEvolutionSuggestion, error)
	ListByTarget(ctx context.Context, targetType string, targetID string, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error)
	CountByTarget(ctx context.Context, targetType string, targetID string, status string) (int, error)
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
	UnifiedEvolutionExpirationWriter
}

// EvolutionStoreBridge abstracts unified+legacy evolution suggestion storage access.
// It combines UnifiedEvolutionReader/Writer with the legacy SkillEvolutionSuggestion
// reader/writer into a single dependency to reduce field count on SkillIntelligenceUsecase.
// Stability:evolving
type EvolutionStoreBridge interface {
	UnifiedEvolutionReader
	UnifiedEvolutionWriter

	// Legacy access
	GetEvolutionSuggestion(ctx context.Context, id string) (*SkillEvolutionSuggestion, error)
	ListEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus, limit, offset int) ([]SkillEvolutionSuggestion, error)
	CountEvolutionSuggestions(ctx context.Context, skillID string, status EvolutionSuggestionStatus) (int, error)
	CreateSuggestion(ctx context.Context, s SkillEvolutionSuggestion) error
	UpdateSuggestionStatus(ctx context.Context, id string, status EvolutionSuggestionStatus, resolvedBy string, reason string) error
	UpdateSuggestionDraftBody(ctx context.Context, id string, draftBody string) error
	UpdateSuggestionLifecycleStatus(ctx context.Context, id string, lifecycleStatus EvolutionLifecycleStatus) error
	UpdateSuggestionSandboxResult(ctx context.Context, id string, passed bool, result json.RawMessage) error
	ListPendingSuggestions(ctx context.Context, limit, offset int) ([]SkillEvolutionSuggestion, error)
	GetLatestSuggestionBySkill(ctx context.Context, skillID string) (*SkillEvolutionSuggestion, error)
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
	reader   UnifiedEvolutionReader
	writer   UnifiedEvolutionWriter
	triggers []EvolutionTrigger
	triggersMu  sync.RWMutex // protects triggers for concurrent RegisterTrigger calls
	lg          loggateway.Logger
}

func NewSkillEvolutionOrchestrator(
	reader UnifiedEvolutionReader,
	writer UnifiedEvolutionWriter,
	lg loggateway.Logger,
) *SkillEvolutionOrchestrator {
	return &SkillEvolutionOrchestrator{
		reader: reader,
		writer: writer,
		lg:     lg,
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
// Delegates to the internal reader.
func (o *SkillEvolutionOrchestrator) HasPendingForTarget(ctx context.Context, targetType, targetID string) (bool, error) {
	return o.reader.HasPendingForTarget(ctx, targetType, targetID)
}

// CheckAndCreate 原子化检查+创建
// 1. 检查是否已有 pending 建议
// 2. 检查冷却期
// 3. 遍历触发器，收集所有触发的建议并逐个创建
// DB UNIQUE 约束兜底防止并发重复创建
func (o *SkillEvolutionOrchestrator) CheckAndCreate(ctx context.Context, targetType EvolutionTargetType, targetID string) ([]UnifiedEvolutionSuggestion, error) {
	// 1. 检查是否已有 pending 建议
	hasPending, err := o.reader.HasPendingForTarget(ctx, string(targetType), targetID)
	if err != nil {
		o.lg.Warn("orchestrator: check pending failed",
			loggateway.StepID("evo_orchestrator.check"),
			loggateway.Err(err))
		// 保守策略：查询失败时不阻止，让 DB 约束兜底
	} else if hasPending {
		return nil, nil
	}

	// 2. 检查冷却期（7 天）
	latest, err := o.reader.GetLatestByTarget(ctx, string(targetType), targetID)
	if err == nil && latest != nil {
		cooldownEnd := latest.CreatedAt.Add(evoCooldownDuration)
		if time.Now().UTC().Before(cooldownEnd) {
			return nil, nil
		}
	}

	// 3. 遍历触发器
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

		// 4. 逐个创建（DB UNIQUE 约束兜底）
		for _, suggestion := range suggestions {
			// Per-action-type cooldown check: each (targetType, targetID, actionType)
			// triple has its own independent cooldown period.
			latestByAction, lbErr := o.reader.GetLatestByTargetAndAction(ctx, string(suggestion.TargetType), suggestion.TargetID, string(suggestion.ActionType))
			if lbErr == nil && latestByAction != nil {
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
	s, err := o.reader.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s.Status != "pending" {
		return apierror.BadRequest("EVO_ORCHESTRATOR", "only pending suggestions can be approved, current status: "+s.Status)
	}
	return o.writer.UpdateStatus(ctx, id, "approved", approvedBy, "")
}

// Reject 拒绝建议
func (o *SkillEvolutionOrchestrator) Reject(ctx context.Context, id string, rejectedBy string, reason string) error {
	s, err := o.reader.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if s.Status != "pending" {
		return apierror.BadRequest("EVO_ORCHESTRATOR", "only pending suggestions can be rejected, current status: "+s.Status)
	}
	return o.writer.UpdateStatus(ctx, id, "rejected", rejectedBy, reason)
}

// ExpirePending 过期 pending 建议（超过 7 天）
func (o *SkillEvolutionOrchestrator) ExpirePending(ctx context.Context) (int, error) {
	cutoff := time.Now().UTC().Add(-evoCooldownDuration)
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

// isDuplicateKeyError 检查是否为 DB 唯一约束冲突错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate")
}
