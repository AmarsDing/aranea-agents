package biz

// D6 L-2 脱敏用例（脱敏五闸的执行面半边：规则热副本 + 严格审计）。
// BeforeModel 钩子（internal/agent mask_gate.go）经本用例取引擎、落审计；
// 五闸的提案/审批/激活面在 V4 纵列（RunMaskGates/Approve/Activate）。

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// secopsMaskRulesTTL 是规则热副本 TTL（与 model_policy 同款思路：钩子每
// 模型调用打缓存，V4 activate 写路径最迟 TTL 后生效；wb 在 activate 时
// 重载引擎，aranea 进程常驻改走 TTL 热副本）。
const secopsMaskRulesTTL = 30 * time.Second

// SecopsMaskRuleRepo 是脱敏规则只读窄口（data.NewSecopsMaskRuleRepo 满足）。
// V4 的提案/激活写口另立窄口，不混入本接口。
type SecopsMaskRuleRepo interface {
	// ListEnabledMaskRules 返回全部启用规则（含 shadow/灰度标注，引擎侧
	// 自行裁决）；顺序稳定（id ASC），保证规则间先后可预期（如身份证
	// 先于银行卡，避免 18 位身份证被银行卡规则抢先替换）。
	ListEnabledMaskRules(ctx context.Context) ([]MaskRule, error)
}

// SecopsMaskUsecase 持有脱敏引擎热副本与审计写口。
type SecopsMaskUsecase struct {
	repo  SecopsMaskRuleRepo
	audit SecopsOffboardAuditRecorder
	lg    loggateway.Logger
	// now 是可测时钟（测试可替换）。nil 时回落 time.Now。
	now func() time.Time

	mu          sync.Mutex
	engine      *MaskEngine
	refreshedAt time.Time
}

// NewSecopsMaskUsecase 装配脱敏用例。repo 为 nil 返回 nil（wire 侧据此
// 停用 L-2 钩子——与 classification 门零值停用同口径）。
func NewSecopsMaskUsecase(repo SecopsMaskRuleRepo, lg loggateway.Logger) *SecopsMaskUsecase {
	if repo == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsMaskUsecase{repo: repo, lg: lg, now: time.Now}
}

// SetAuditRecorder 后置注入审计写口（构造签名稳定，wire 友好）。
func (u *SecopsMaskUsecase) SetAuditRecorder(rec SecopsOffboardAuditRecorder) {
	if u != nil {
		u.audit = rec
	}
}

func (u *SecopsMaskUsecase) clock() time.Time {
	if u.now != nil {
		return u.now()
	}
	return time.Now()
}

// Engine 返回当前脱敏引擎快照（TTL 热副本）。
//
// 返回 nil 表示引擎不可用（冷缓存 + 规则读取/编译失败）：调用方须按
// fail-closed 处理（wb: engFn()==nil → mask_engine_unavailable → 拒绝上云）。
// 有上一份好快照时读取失败沿用旧引擎（日志告警），不做 fail-closed 降级
// ——旧规则集仍是可执行的安全策略。
func (u *SecopsMaskUsecase) Engine(ctx context.Context) *MaskEngine {
	if u == nil {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.clock()
	if u.engine != nil && now.Sub(u.refreshedAt) < secopsMaskRulesTTL {
		return u.engine
	}
	rules, err := u.repo.ListEnabledMaskRules(ctx)
	if err != nil {
		u.lg.Warn("脱敏规则热副本刷新失败，沿用上次快照",
			loggateway.StepID("secops.mask_rules_refresh_fail"),
			loggateway.Bool("cold", u.engine == nil),
			loggateway.Err(err))
		return u.engine
	}
	engine, err := NewMaskEngine(rules)
	if err != nil {
		u.lg.Error("脱敏规则编译失败，沿用上次快照",
			loggateway.StepID("secops.mask_rules_compile_fail"),
			loggateway.Bool("cold", u.engine == nil),
			loggateway.Err(err))
		return u.engine
	}
	u.engine = engine
	u.refreshedAt = now
	return engine
}

// MaskAuditAction 是四审计事件在 audit_logs 的 action 取值（wb 原名 + secops 前缀）。
const (
	SecopsMaskActionRouteLocalForced  = "secops.mask.route_local_forced"
	SecopsMaskActionEngineUnavailable = "secops.mask.engine_unavailable"
	SecopsMaskActionReject            = "secops.mask.reject"
	SecopsMaskActionCloudSend         = "secops.mask.cloud_send"
)

// RecordMaskAudit 是脱敏面**严格**审计写口（wb F3 语义：审计失败即拒——
// 调用方在审计 error 时不得放行上云）。detail 只允许计数/类别/水位线，
// 绝不携带原文、findings 原文或占位符映射（明文三不进）。
func (u *SecopsMaskUsecase) RecordMaskAudit(ctx context.Context, action, scope string, detail map[string]any, severity string) error {
	if u == nil {
		return nil
	}
	if scope == "" {
		scope = "unknown"
	}
	return insertSecopsAudit(ctx, u.audit, action, scope, detail, severity)
}
