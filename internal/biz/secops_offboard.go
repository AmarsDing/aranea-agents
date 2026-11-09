package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// secops 离职处置五步（E 阶段 W2a D3，需求文档 §6.3 / 整合方案 §4.3）。
// 移植 workbench memoryiso.go 五步流：freeze → request → approve(不可自批)
// → export → delete(前置「已导出」校验)。角色收敛：aranea 全流为 sysadmin
// 专属域（user.offboard 码位；wb 原流 freeze/request 亦允许 secadmin——本仓
// 用户生命周期归 user.* 码位，偏离点在 service 层注释标注，W4 评审复核）。
//
// 安全不变式（与 wb 等价）：任何单人都无法独走「导出+删除」——request 与
// approve 必须两个不同 sysadmin（不可自批服务端强制）；delete 前置
// export_checksum 非空（导出动作本身先落审计再持久化校验和，审计失败即
// 导出失败 → 删除前置不可能被绕过）。
// ============================================================================

// OffboardStatus* 是 secops_offboard_requests.status 取值（DDL 20261325，
// 默认 'requested'）。proto OffboardRequest.status 注释与本组对齐。
const (
	OffboardStatusRequested = "requested"
	OffboardStatusApproved  = "approved"
	OffboardStatusRejected  = "rejected"
	OffboardStatusCompleted = "completed"
)

// offboardExportFactLimit 是单次导出的 user 域事实拉取上限（离职导出走
// 管理面 ListFactRows——无 PII 门、含隔离行，正是导出语义；上限防御性截断，
// 超出部分由多次导出差量补齐，W4 评估是否需要游标）。
const offboardExportFactLimit = 10000

// SecopsOffboardRequest 是离职处置申请单（secops_offboard_requests 行）。
// ApprovedAt/Note 自 steps_json 解析（DDL 无独立列，迁移已定稿不追加）。
type SecopsOffboardRequest struct {
	ID             string
	TargetUserID   int64
	RequesterID    int64
	ApproverID     int64
	Status         string
	ExportChecksum string
	ExportedAt     time.Time
	ApprovedAt     time.Time
	Note           string
	StepsJSON      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SecopsOffboardExport 是导出物（该用户 user 域全量，解密明文 JSON——
// 调用方负责保管；明文三不进约束下本结构只允许在进程内/RPC 响应中存活，
// 不落日志、不进审计 detail）。
type SecopsOffboardExport struct {
	UserID     int64
	RequestID  string
	ItemsJSON  string
	Checksum   string
	ExportedAt time.Time
	FactCount  int
}

// SecopsOffboardRequestRepo 是申请单台账窄写口（data.secopsOffboardRepo 满足）。
type SecopsOffboardRequestRepo interface {
	Insert(ctx context.Context, req *SecopsOffboardRequest) error
	// GetLatestByTargetUser 返回该用户最新一条申请（任意状态）；无记录返 (nil, nil)。
	GetLatestByTargetUser(ctx context.Context, userID int64) (*SecopsOffboardRequest, error)
	// ListRecent 返回最近申请（created_at 倒序，limit 截断）。
	ListRecent(ctx context.Context, limit int) ([]*SecopsOffboardRequest, error)
	// ListPending 返回 status='requested' 的申请（审批中心 lane 数据源）。
	ListPending(ctx context.Context, limit int) ([]*SecopsOffboardRequest, error)
	// MarkDecision 记审批结果（approver/status/note/at 写入 steps_json）。
	MarkDecision(ctx context.Context, id string, approverID int64, status, note string, at time.Time) error
	MarkExported(ctx context.Context, id, checksum string, at time.Time) error
	MarkCompleted(ctx context.Context, id string, at time.Time) error
}

// SecopsOffboardFactLister 是 user 域事实只读导出窄口（l3FactRepo 满足；
// 管理面无 PII 门，含 pii_flag=1 隔离行——与导出语义一致）。
type SecopsOffboardFactLister interface {
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error)
}

// SecopsOffboardFactClearer 是离职删除的事实清理窄口。刻意不用
// ClearFactsByScope（多一重 user_id 列谓词）：导出按 (scope_type,scope_id)
// 取全集，删除必须同口径对称，否则 CopyFactRow 进 user 域未填 user_id 的
// 行会漏删（W4 日志 (i)）。data 层提供 ClearUserDomainFacts。
type SecopsOffboardFactClearer interface {
	ClearUserDomainFacts(ctx context.Context, scopeType, scopeID string) ([]string, error)
}

// SecopsOffboardAuditRecorder 是五步审计窄口（data.NewMonitorAuditRepo 满足）。
type SecopsOffboardAuditRecorder interface {
	InsertAuditLog(ctx context.Context, entry AuditLog) error
}

// SecopsOffboardUsecase 实现离职处置五步。
type SecopsOffboardUsecase struct {
	reader   AdminReader
	writer   AdminWriter
	revoker  SessionRevoker
	requests SecopsOffboardRequestRepo
	facts    SecopsOffboardFactLister
	clearer  SecopsOffboardFactClearer
	audit    SecopsOffboardAuditRecorder
	lg       loggateway.Logger
}

// NewSecopsOffboardUsecase 装配离职处置用例（reader/writer/requests/facts/
// clearer 为硬依赖；revoker/audit 经 setter 后置注入，wire 友好——构造签名
// 稳定，与 SetSessionRevoker/SetMemoryFactCopyAuditRecorder 同模式）。
func NewSecopsOffboardUsecase(reader AdminReader, writer AdminWriter, requests SecopsOffboardRequestRepo, facts SecopsOffboardFactLister, clearer SecopsOffboardFactClearer, lg loggateway.Logger) *SecopsOffboardUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SecopsOffboardUsecase{
		reader:   reader,
		writer:   writer,
		requests: requests,
		facts:    facts,
		clearer:  clearer,
		lg:       lg,
	}
}

// SetSessionRevoker 注入会话吊销后端；nil 时 Freeze/Delete 的吊销环节诚实
// 失败返回错误（绝不静默假吊销，与 AdminUsecase 同契约）。
func (uc *SecopsOffboardUsecase) SetSessionRevoker(r SessionRevoker) {
	if uc != nil {
		uc.revoker = r
	}
}

// SetSecopsOffboardAuditRecorder 注入审计记录器（构造后置，wire 友好）。
func (uc *SecopsOffboardUsecase) SetSecopsOffboardAuditRecorder(rec SecopsOffboardAuditRecorder) {
	if uc != nil {
		uc.audit = rec
	}
}

// ── 第一步：冻结 ─────────────────────────────────────────────────────

// FreezeUser 冻结目标账号（status=frozen + 吊销全部会话）。冻结后
// LoginByUsername 的 P1 拒绝即生效（新登录 401），吊销覆盖旧 token。
func (uc *SecopsOffboardUsecase) FreezeUser(ctx context.Context, targetUserID, actorID int64) error {
	admin, err := uc.mustFindTarget(ctx, targetUserID)
	if err != nil {
		return err
	}
	if admin.Status == AdminStatusOffboarded {
		return apierror.FailedPrecondition(apierror.DomainAdmin, "user is already offboarded")
	}
	if admin.Status != AdminStatusFrozen {
		admin.Status = AdminStatusFrozen
		if _, err := uc.writer.UpdateAdmin(ctx, admin); err != nil {
			return err
		}
	}
	if uc.revoker == nil {
		return apierror.Internal(apierror.DomainAdmin, "session revoker not wired")
	}
	if _, err := uc.revoker.RevokeSessions(ctx, targetUserID); err != nil {
		return err
	}
	uc.recordOffboardAudit(ctx, "secops.offboard.freeze", targetUserID, map[string]any{
		"user_id":  targetUserID,
		"status":   AdminStatusFrozen,
		"actor_id": actorID,
	}, "info")
	return nil
}

// ── 第二步：发起导出申请 ─────────────────────────────────────────────

// RequestOffboardExport 为 targetUser 开立导出申请单。同用户已存在
// requested/approved 活跃单 → Conflict（防并发双单）。
func (uc *SecopsOffboardUsecase) RequestOffboardExport(ctx context.Context, targetUserID, requesterID int64) (*SecopsOffboardRequest, error) {
	if _, err := uc.mustFindTarget(ctx, targetUserID); err != nil {
		return nil, err
	}
	latest, err := uc.requests.GetLatestByTargetUser(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if latest != nil && (latest.Status == OffboardStatusRequested || latest.Status == OffboardStatusApproved) {
		return nil, apierror.Conflict(apierror.DomainAdmin,
			"an active offboard request already exists for this user")
	}
	req := &SecopsOffboardRequest{
		TargetUserID: targetUserID,
		RequesterID:  requesterID,
		Status:       OffboardStatusRequested,
		StepsJSON:    "{}",
	}
	if err := uc.requests.Insert(ctx, req); err != nil {
		return nil, err
	}
	uc.recordOffboardAudit(ctx, "secops.offboard.request", targetUserID, map[string]any{
		"user_id":    targetUserID,
		"request_id": req.ID,
		"actor_id":   requesterID,
	}, "info")
	return req, nil
}

// ── 第三步：审批（不可自批）──────────────────────────────────────────

// ApproveOffboardExport 审批最新活跃申请。不可自批为服务端强制硬门：
// approverID == requesterID → Forbidden（sysadmin A 发起、sysadmin B 审批，
// 单人无法独走导出）。approve=false → 单据 rejected（可重新发起）。
// 角色门（sysadmin 专属）在 service 层以 HasRole 判定——本层只看数字身份。
func (uc *SecopsOffboardUsecase) ApproveOffboardExport(ctx context.Context, targetUserID, approverID int64, approve bool, note string) (*SecopsOffboardRequest, error) {
	req, err := uc.mustLatestRequested(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if req.RequesterID == approverID {
		return nil, apierror.Forbidden(apierror.DomainAdmin,
			"requester cannot approve their own offboard request")
	}
	status := OffboardStatusApproved
	action := "secops.offboard.approve"
	if !approve {
		status = OffboardStatusRejected
		action = "secops.offboard.reject"
	}
	if err := uc.requests.MarkDecision(ctx, req.ID, approverID, status, strings.TrimSpace(note), time.Now().UTC()); err != nil {
		return nil, err
	}
	uc.recordOffboardAudit(ctx, action, targetUserID, map[string]any{
		"user_id":     targetUserID,
		"request_id":  req.ID,
		"approver_id": approverID,
		"approve":     approve,
		"note":        strings.TrimSpace(note),
	}, "info")
	return uc.requests.GetLatestByTargetUser(ctx, targetUserID)
}

// ── 第四步：凭已审批单导出 ───────────────────────────────────────────

// GetOffboardExport 凭 approved 申请导出该用户 user 域全量（含 PII 隔离行，
// 解密明文）。前置：最新单 status=approved。审计失败即整体失败（fail-closed，
// 与 wb auditSensitive 等价）：export_checksum 不落库 → 删除前置无法满足。
func (uc *SecopsOffboardUsecase) GetOffboardExport(ctx context.Context, targetUserID, operatorID int64) (*SecopsOffboardExport, error) {
	req, err := uc.latestRequest(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Status != OffboardStatusApproved {
		return nil, apierror.Forbidden(apierror.DomainAdmin,
			"export requires an approved offboard request")
	}
	rows, _, _, _, err := uc.facts.ListFactRows(ctx, "user", strconv.FormatInt(targetUserID, 10),
		"", "", "", "", offboardExportFactLimit, 0)
	if err != nil {
		return nil, err
	}
	items := make([]json.RawMessage, 0, len(rows))
	for _, raw := range rows {
		items = append(items, json.RawMessage(raw))
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "failed to encode offboard export")
	}
	sum := sha256.Sum256(payload)
	checksum := hex.EncodeToString(sum[:])

	// 审计先于校验和持久化：审计失败 → 不导出（无 checksum → delete 被拒）。
	uc.recordOffboardAuditStrict(ctx, "secops.offboard.export", targetUserID, map[string]any{
		"user_id":    targetUserID,
		"request_id": req.ID,
		"fact_count": len(rows),
		"sha256":     checksum,
		"actor_id":   operatorID,
	}, "warning")
	now := time.Now().UTC()
	if err := uc.requests.MarkExported(ctx, req.ID, checksum, now); err != nil {
		return nil, err
	}
	return &SecopsOffboardExport{
		UserID:     targetUserID,
		RequestID:  req.ID,
		ItemsJSON:  string(payload),
		Checksum:   checksum,
		ExportedAt: now,
		FactCount:  len(rows),
	}, nil
}

// ── 第五步：删除（前置「已导出」）────────────────────────────────────

// DeleteOffboardedUser 清事实 + status=offboarded + 吊销会话 + 单据 completed。
// 硬前置：最新单 approved 且 export_checksum 非空（验收 #4：未导出先删 → 拒）。
// 事实清理先于账号置位：清理失败即中止，账号保持 frozen 可重试。
func (uc *SecopsOffboardUsecase) DeleteOffboardedUser(ctx context.Context, targetUserID, actorID int64) error {
	req, err := uc.latestRequest(ctx, targetUserID)
	if err != nil {
		return err
	}
	if req == nil || req.Status != OffboardStatusApproved {
		return apierror.Forbidden(apierror.DomainAdmin,
			"delete requires an approved offboard request")
	}
	if strings.TrimSpace(req.ExportChecksum) == "" {
		return apierror.FailedPrecondition(apierror.DomainAdmin,
			"offboard export must complete before delete")
	}
	cleared, err := uc.clearer.ClearUserDomainFacts(ctx, "user", strconv.FormatInt(targetUserID, 10))
	if err != nil {
		return err
	}
	admin, err := uc.mustFindTarget(ctx, targetUserID)
	if err != nil {
		return err
	}
	admin.Status = AdminStatusOffboarded
	if _, err := uc.writer.UpdateAdmin(ctx, admin); err != nil {
		return err
	}
	if uc.revoker == nil {
		return apierror.Internal(apierror.DomainAdmin, "session revoker not wired")
	}
	if _, err := uc.revoker.RevokeSessions(ctx, targetUserID); err != nil {
		return err
	}
	if err := uc.requests.MarkCompleted(ctx, req.ID, time.Now().UTC()); err != nil {
		return err
	}
	// 后置审计 warn 降级：业务态已一致（completed + offboarded），审计空洞
	// 由 monitor 对账发现；反向（先审计后执行）会在审计成功后业务失败留下
	// 误导性「已删」记录。
	uc.recordOffboardAudit(ctx, "secops.offboard.delete", targetUserID, map[string]any{
		"user_id":            targetUserID,
		"request_id":         req.ID,
		"fact_count_cleared": len(cleared),
		"actor_id":           actorID,
	}, "warning")
	return nil
}

// ── 查询 ────────────────────────────────────────────────────────────

// ListOffboardRequests 返回最近 200 条申请单（sysadmin 控制台；auditor 走
// audit.read 审计面，本 RPC 不向其开放——wb 偏离点，W4 复核）。
func (uc *SecopsOffboardUsecase) ListOffboardRequests(ctx context.Context, operatorID int64) ([]*SecopsOffboardRequest, error) {
	return uc.requests.ListRecent(ctx, 200)
}

// ── 内部 ────────────────────────────────────────────────────────────

func (uc *SecopsOffboardUsecase) mustFindTarget(ctx context.Context, userID int64) (*Admin, error) {
	if uc.reader == nil || uc.writer == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "admin store not wired")
	}
	admin, err := uc.reader.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrAdminNotFound) {
			return nil, apierror.NotFound(apierror.DomainAdmin, "target user not found")
		}
		return nil, err
	}
	if admin == nil {
		return nil, apierror.NotFound(apierror.DomainAdmin, "target user not found")
	}
	return admin, nil
}

func (uc *SecopsOffboardUsecase) latestRequest(ctx context.Context, userID int64) (*SecopsOffboardRequest, error) {
	if uc.requests == nil {
		return nil, apierror.Internal(apierror.DomainAdmin, "offboard request store not wired")
	}
	return uc.requests.GetLatestByTargetUser(ctx, userID)
}

func (uc *SecopsOffboardUsecase) mustLatestRequested(ctx context.Context, userID int64) (*SecopsOffboardRequest, error) {
	req, err := uc.latestRequest(ctx, userID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, apierror.NotFound(apierror.DomainAdmin, "no offboard request for this user")
	}
	if req.Status != OffboardStatusRequested {
		return nil, apierror.FailedPrecondition(apierror.DomainAdmin,
			fmt.Sprintf("offboard request is %s", req.Status))
	}
	return req, nil
}

// recordOffboardAudit 落五步审计（best-effort：失败仅告警降级，与
// memory.fact.copy 审计同契约；导出步例外——用 Strict 变体 fail-closed）。
// detail 只允许 id/计数/摘要/校验和，绝不携带记忆明文（明文三不进）。
func (uc *SecopsOffboardUsecase) recordOffboardAudit(ctx context.Context, action string, targetUserID int64, detail map[string]any, severity string) {
	if err := uc.insertOffboardAudit(ctx, action, targetUserID, detail, severity); err != nil {
		uc.lg.Warn("offboard audit persist failed",
			loggateway.StepID("secops.offboard_audit_fail"),
			loggateway.Str("action", action),
			loggateway.Int64("target_user_id", targetUserID),
			loggateway.Err(err))
	}
}

// recordOffboardAuditStrict 是导出步专用 fail-closed 审计：落库失败向上抛，
// 阻断导出（校验和随之不持久化 → delete 前置恒拒）。
func (uc *SecopsOffboardUsecase) recordOffboardAuditStrict(ctx context.Context, action string, targetUserID int64, detail map[string]any, severity string) error {
	return uc.insertOffboardAudit(ctx, action, targetUserID, detail, severity)
}

func (uc *SecopsOffboardUsecase) insertOffboardAudit(ctx context.Context, action string, targetUserID int64, detail map[string]any, severity string) error {
	if uc.audit == nil {
		return nil
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	if severity == "" {
		severity = "info"
	}
	return uc.audit.InsertAuditLog(ctx, AuditLog{
		Action:     action,
		Resource:   "admin",
		ResourceID: strconv.FormatInt(targetUserID, 10),
		Detail:     string(payload),
		Actor:      offboardActorFromContext(ctx),
		Severity:   severity,
	})
}

// offboardActorFromContext 从 JWT 主体取审计 actor（数字 id）；无主体空串。
func offboardActorFromContext(ctx context.Context) string {
	if p, ok := auth.FromContext(ctx); ok && p != nil {
		return strconv.FormatInt(p.UserID, 10)
	}
	return ""
}
