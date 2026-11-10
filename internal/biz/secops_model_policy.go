package biz

import (
	"context"
	"strconv"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// secops 模型路由策略（E 阶段 W2b D6，需求 §7.3）。model_policy 单行
//（id=1，DDL 20261327）：local_threshold 默认 3，水位线 >= 阈值强制本地
// 通道；仅 sysadmin 可改（secops.policy 码位，service 层兜底闸门）+ 审计。
//
// 热副本：V2 ClassificationAwareModelSelector 每 turn 调 GetPolicy 判路
//由，直接打库会成为会话热点——进程内短 TTL 缓存（拍板 L-1 热副本语义），
// SetPolicy 写后即失效，保证下一次判定见新值；TTL 只影响缓存陈旧窗口，
// 不改变「写后终态一致」语义。
// ============================================================================

// SecopsDefaultLocalThreshold 是 model_policy 种子默认值（迁移 20261327
// 单行 id=1；与 wb schema.sql 同默认）。
const SecopsDefaultLocalThreshold = 3

// secopsPolicyCacheTTL 是热副本陈旧窗口（写路径即时失效，TTL 仅兜底
// 其他节点/路径的变更刷新——当前单写口只有 SetModelPolicy RPC）。
const secopsPolicyCacheTTL = 5 * time.Second

// SecopsModelPolicy 模型路由策略（model_policy 行；无行时 GetPolicy
// 返回默认值结构）。
type SecopsModelPolicy struct {
	LocalThreshold int
	UpdatedBy      string
	UpdatedAt      time.Time
}

// SecopsModelPolicyRepo 模型策略仓储窄口。
type SecopsModelPolicyRepo interface {
	// GetPolicy 读单行策略；表空 → (nil, nil)（biz 层补默认值）。
	GetPolicy(ctx context.Context) (*SecopsModelPolicy, error)
	// SetPolicy upsert 单行（id=1）并记录更新人/时间。
	SetPolicy(ctx context.Context, threshold int, updatedBy string) error
}

// SecopsModelPolicyUsecase 模型路由策略用例（含热副本）。
type SecopsModelPolicyUsecase struct {
	repo  SecopsModelPolicyRepo
	audit SecopsOffboardAuditRecorder
	lg    loggateway.Logger

	mu      sync.RWMutex
	cache   *SecopsModelPolicy
	cacheAt time.Time
	now     func() time.Time // 测试可注入时钟
}

// NewSecopsModelPolicyUsecase 装配策略用例（repo 硬依赖；audit 经
// setter 后置注入，wire 友好）。
func NewSecopsModelPolicyUsecase(repo SecopsModelPolicyRepo, lg loggateway.Logger) *SecopsModelPolicyUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsModelPolicyUsecase{repo: repo, lg: lg, now: time.Now}
}

// SetSecopsModelPolicyAuditRecorder 注入审计记录器（构造后置，wire 友好）。
func (uc *SecopsModelPolicyUsecase) SetSecopsModelPolicyAuditRecorder(rec SecopsOffboardAuditRecorder) {
	if uc != nil {
		uc.audit = rec
	}
}

// GetPolicy 读当前生效策略（热副本优先）：表空 → 默认阈值
// SecopsDefaultLocalThreshold。返回值是拷贝，调用方不可变更是非。
func (uc *SecopsModelPolicyUsecase) GetPolicy(ctx context.Context) (*SecopsModelPolicy, error) {
	if uc.repo == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "secops model policy repo not wired")
	}
	now := uc.now().UTC()
	uc.mu.RLock()
	if uc.cache != nil && now.Sub(uc.cacheAt) < secopsPolicyCacheTTL {
		out := *uc.cache
		uc.mu.RUnlock()
		return &out, nil
	}
	uc.mu.RUnlock()

	row, err := uc.repo.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &SecopsModelPolicy{LocalThreshold: SecopsDefaultLocalThreshold}
	}
	uc.mu.Lock()
	uc.cache = row
	uc.cacheAt = now
	uc.mu.Unlock()
	out := *row
	return &out, nil
}

// SetModelPolicy 更新本地阈值：限 1..5（降出合法区间/抬出合法区间均
// 拒——BadRequest 输入校验，不进审计；变更本身记审计
// secops.classification.policy_update，detail 携带新旧阈值与 actor）。
// 写后热副本即时失效，下一次 GetPolicy 见新值。
func (uc *SecopsModelPolicyUsecase) SetModelPolicy(ctx context.Context, threshold int, actorID int64) (*SecopsModelPolicy, error) {
	if threshold < SecopsClassificationMinLevel || threshold > SecopsClassificationMaxLevel {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "local_threshold must be between %d and %d", SecopsClassificationMinLevel, SecopsClassificationMaxLevel)
	}
	if uc.repo == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "secops model policy repo not wired")
	}
	prev, err := uc.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	updatedBy := strconv.FormatInt(actorID, 10)
	if err := uc.repo.SetPolicy(ctx, threshold, updatedBy); err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.cache = nil
	uc.cacheAt = time.Time{}
	uc.mu.Unlock()
	out := &SecopsModelPolicy{LocalThreshold: threshold, UpdatedBy: updatedBy, UpdatedAt: uc.now().UTC()}
	uc.recordPolicyAudit(ctx, map[string]any{
		"old_threshold": prev.LocalThreshold,
		"new_threshold": threshold,
		"actor_id":      actorID,
	}, "info")
	return out, nil
}

// recordPolicyAudit 落策略变更审计（best-effort；detail 只允许阈值/actor）。
func (uc *SecopsModelPolicyUsecase) recordPolicyAudit(ctx context.Context, detail map[string]any, severity string) {
	if err := insertSecopsAudit(ctx, uc.audit, "secops.classification.policy_update", "model_policy", detail, severity); err != nil {
		uc.lg.Warn("model policy audit persist failed",
			loggateway.StepID("secops.policy_audit_fail"),
			loggateway.Err(err))
	}
}
