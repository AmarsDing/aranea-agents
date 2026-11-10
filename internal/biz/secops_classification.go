package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// secops 密级标签 + 会话/运行水位线（E 阶段 W2b D6，需求 §7.3 验收 #2/#5 数据面）。
// DDL 20261327：classification_labels（L1-L5 种子，L3=强制本地）+
// sessions/session_runs ceiling 列（0=未定级）。语义平移自 workbench：
//   - 水位线只升不降：写路径 GREATEST（Dialect.Greatest），显式降阈请求
//     被拒 + 审计（验收 §7.3 #2「降阈请求被拒+审计」）；
//   - 解析容错：ParseCeilingLevel 解析失败返回 error，V2 selector 按保守
//     nil 语义保持 base 路由（验收 §7.3 #5）。
// 审计 detail 只允许 id/级别/计数，绝不携带内容明文（明文三不进）。
// ============================================================================

const (
	// SecopsClassificationMinLevel/MaxLevel 密级合法区间（L1-L5）。
	SecopsClassificationMinLevel = 1
	SecopsClassificationMaxLevel = 5
)

// ClassificationLabel 密级标签（classification_labels 行）。
type ClassificationLabel struct {
	Level       int
	Name        string
	Description string
	UpdatedAt   time.Time
}

// SecopsClassificationRepo 密级标签仓储窄口。
type SecopsClassificationRepo interface {
	// ListLabels 按 level 升序列全部标签。
	ListLabels(ctx context.Context) ([]ClassificationLabel, error)
	// PutLabel upsert 指定 level 的名称/描述。
	PutLabel(ctx context.Context, label ClassificationLabel) error
}

// SecopsSessionWaterlineRepo 会话/运行水位线仓储窄口（GREATEST 单调写）。
type SecopsSessionWaterlineRepo interface {
	// GetSessionCeiling 读会话水位线；无行 → 0（未定级）。
	GetSessionCeiling(ctx context.Context, sessionID string) (int, error)
	// RaiseSessionCeiling 写 GREATEST(ceiling, level)（只升不降），
	// 返回写入后的水位线；会话不存在 → NotFound。
	RaiseSessionCeiling(ctx context.Context, sessionID string, level int) (int, error)
	// RaiseRunCeiling 同上（session_runs 行须已存在）。
	RaiseRunCeiling(ctx context.Context, runID string, level int) (int, error)
}

// ParseCeilingLevel 解析水位线文本："" → 0（未定级）；"0".."5" → 级别；
// 其余（非数字/越界/负数）→ error。供 V2 selector / V3 钩子容错解析——
// 解析失败不阻断原路由（验收 §7.3 #5 保守 nil 语义）。
func ParseCeilingLevel(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > SecopsClassificationMaxLevel {
		return 0, fmt.Errorf("invalid ceiling level %q", raw)
	}
	return n, nil
}

// CeilingForcesLocal 水位线 >= 本地阈值 → 强制本地通道。阈值合法区间
// 1..5（model_policy 写入校验），未定级 0 永不触发。
func CeilingForcesLocal(ceiling, localThreshold int) bool {
	return ceiling >= localThreshold
}

// SecopsClassificationUsecase 密级标签 + 水位线用例（L-1 数据面）。
type SecopsClassificationUsecase struct {
	labels    SecopsClassificationRepo
	waterline SecopsSessionWaterlineRepo
	audit     SecopsOffboardAuditRecorder
	lg        loggateway.Logger
}

// NewSecopsClassificationUsecase 装配密级用例（labels/waterline 硬依赖；
// audit 经 setter 后置注入，wire 友好，与 NewSecopsOffboardUsecase 同模式）。
func NewSecopsClassificationUsecase(labels SecopsClassificationRepo, waterline SecopsSessionWaterlineRepo, lg loggateway.Logger) *SecopsClassificationUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsClassificationUsecase{labels: labels, waterline: waterline, lg: lg}
}

// SetSecopsClassificationAuditRecorder 注入审计记录器（构造后置，wire 友好）。
func (uc *SecopsClassificationUsecase) SetSecopsClassificationAuditRecorder(rec SecopsOffboardAuditRecorder) {
	if uc != nil {
		uc.audit = rec
	}
}

// ListLabels 列全部密级标签（任何认证用户可读；service 层复核认证态）。
func (uc *SecopsClassificationUsecase) ListLabels(ctx context.Context) ([]ClassificationLabel, error) {
	if uc.labels == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "secops classification repo not wired")
	}
	return uc.labels.ListLabels(ctx)
}

// PutLabel 新增/改名密级标签：level 限 1..5、name 非空；写后审计
// （secops.classification.label_put）。sysadmin 面（secops.policy 码位）
// 的兜底闸门在 service 层。
func (uc *SecopsClassificationUsecase) PutLabel(ctx context.Context, level int, name, description string, actorID int64) (*ClassificationLabel, error) {
	if level < SecopsClassificationMinLevel || level > SecopsClassificationMaxLevel {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "level must be between %d and %d", SecopsClassificationMinLevel, SecopsClassificationMaxLevel)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apierror.BadRequest(apierror.DomainAdmin, "name is required")
	}
	if uc.labels == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "secops classification repo not wired")
	}
	label := ClassificationLabel{Level: level, Name: name, Description: strings.TrimSpace(description), UpdatedAt: time.Now().UTC()}
	if err := uc.labels.PutLabel(ctx, label); err != nil {
		return nil, err
	}
	uc.recordClassificationAudit(ctx, "secops.classification.label_put", strconv.Itoa(level), map[string]any{
		"level":    level,
		"name":     name,
		"actor_id": actorID,
	}, "info")
	return &label, nil
}

// GetSessionCeiling 读会话水位线（未定级 → 0）。
func (uc *SecopsClassificationUsecase) GetSessionCeiling(ctx context.Context, sessionID string) (int, error) {
	if uc.waterline == nil {
		return 0, apierror.Internal(apierror.DomainAdmin, "secops waterline repo not wired")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "session_id is required")
	}
	return uc.waterline.GetSessionCeiling(ctx, sessionID)
}

// RaiseSessionCeiling 合并路径（L1 会话引 L3 来源 → ceiling=3）：GREATEST
// 只升不降，level 限 1..5；返回写入后水位线。密级来源判定在 V2/V3 接线。
func (uc *SecopsClassificationUsecase) RaiseSessionCeiling(ctx context.Context, sessionID string, level int) (int, error) {
	if uc.waterline == nil {
		return 0, apierror.Internal(apierror.DomainAdmin, "secops waterline repo not wired")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "session_id is required")
	}
	if level < SecopsClassificationMinLevel || level > SecopsClassificationMaxLevel {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "level must be between %d and %d", SecopsClassificationMinLevel, SecopsClassificationMaxLevel)
	}
	return uc.waterline.RaiseSessionCeiling(ctx, sessionID, level)
}

// SetSessionCeiling 显式设定水位线：低于当前值 → 拒绝（FailedPrecondition）
// + 审计（验收 §7.3 #2「降阈请求被拒+审计」）；等于/高于 → 走 GREATEST 写。
func (uc *SecopsClassificationUsecase) SetSessionCeiling(ctx context.Context, sessionID string, level int, actorID int64) (int, error) {
	if uc.waterline == nil {
		return 0, apierror.Internal(apierror.DomainAdmin, "secops waterline repo not wired")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "session_id is required")
	}
	if level < 0 || level > SecopsClassificationMaxLevel {
		return 0, apierror.BadRequest(apierror.DomainAdmin, "level must be between 0 and %d", SecopsClassificationMaxLevel)
	}
	current, err := uc.waterline.GetSessionCeiling(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if level < current {
		uc.recordClassificationAudit(ctx, "secops.classification.ceiling_rejected", sessionID, map[string]any{
			"session_id":      sessionID,
			"requested_level": level,
			"current_level":   current,
			"actor_id":        actorID,
		}, "warn")
		return current, apierror.FailedPrecondition(apierror.DomainAdmin, "ceiling cannot be lowered: current=%d requested=%d", current, level)
	}
	return uc.waterline.RaiseSessionCeiling(ctx, sessionID, level)
}

// recordClassificationAudit 落密级面审计（best-effort：失败仅告警降级，
// 与 recordOffboardAudit 同契约；detail 只允许 id/级别/计数）。
func (uc *SecopsClassificationUsecase) recordClassificationAudit(ctx context.Context, action, resourceID string, detail map[string]any, severity string) {
	if err := insertSecopsAudit(ctx, uc.audit, action, resourceID, detail, severity); err != nil {
		uc.lg.Warn("classification audit persist failed",
			loggateway.StepID("secops.classification_audit_fail"),
			loggateway.Str("action", action),
			loggateway.Err(err))
	}
}

// insertSecopsAudit 是密级面共享审计写口（secops_classification/
// secops_model_policy/secops_mask_proposal 共用；与 insertOffboardAudit
// 同列契约）。
func insertSecopsAudit(ctx context.Context, rec SecopsOffboardAuditRecorder, action, resourceID string, detail map[string]any, severity string) error {
	if rec == nil {
		return nil
	}
	entry, err := buildSecopsAuditEntry(ctx, action, resourceID, detail, severity)
	if err != nil {
		return err
	}
	return rec.InsertAuditLog(ctx, entry)
}

// buildSecopsAuditEntry 构造 secops 面审计条目（actor 由入参 ctx 的 JWT
// 主体决定；detail 只允许 id/级别/计数/类别，绝不携带明文）。事务内审计
// 闭包须在事务外先构造条目——ExecInTx 的 txCtx 派生自 Background，
// auth 主体值在 tx 内不可得。
func buildSecopsAuditEntry(ctx context.Context, action, resourceID string, detail map[string]any, severity string) (AuditLog, error) {
	payload, err := json.Marshal(detail)
	if err != nil {
		return AuditLog{}, err
	}
	if severity == "" {
		severity = "info"
	}
	return AuditLog{
		Action:     action,
		Resource:   "secops",
		ResourceID: resourceID,
		Detail:     string(payload),
		Actor:      offboardActorFromContext(ctx),
		Severity:   severity,
	}, nil
}
