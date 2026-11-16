package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	approvalv1 "aranea-agents/api/kratos/approval/v1"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ApprovalCenterService implements kratos approval.v1（E 阶段 W1b 统一审批
// 中心，需求文档 §5.3 拍板 A-1/A-4）。
//
// 红线：
//   - 审批聚合（ListApprovals/GetApprovalSummary）只读；统一收件箱的
//     DecideInboxItem 是统一决议入口，只做分发——按 id 回源重新解析后
//     调用各领域既有写路径（agentbridge ReviewTask、进化建议
//     Apply/Reject…），不绕过领域校验；另一个写口是 PutRiskMatrix
//     （approval.policy 权限码，配置类而非决议类）。
//   - 不返回无权对象：可见性过滤在 biz 聚合器逐源完成（照抄 workbench
//     approvals.go 模式），本层只做 proto 映射，不做二次放行。
//   - Viewer 一律由 JWT 主体派生（approval.ViewerFromContext），禁请求体
//     自报身份/工作区。
type ApprovalCenterService struct {
	approvalv1.UnimplementedApprovalCenterServiceServer

	agg    *approval.Aggregator
	matrix *approval.Matrix
	inbox  *inbox.Aggregator // 统一收件箱（WithInbox 挂接，可为 nil）
}

func NewApprovalCenterService(agg *approval.Aggregator, matrix *approval.Matrix) *ApprovalCenterService {
	return &ApprovalCenterService{agg: agg, matrix: matrix}
}

// ---- 枚举映射（proto ↔ biz 字符串常量，显式双向表，拒绝 String() 拼名）----

func approvalKindToProto(kind string) approvalv1.ApprovalKind {
	switch kind {
	case approval.KindTaskReview:
		return approvalv1.ApprovalKind_APPROVAL_KIND_TASK_REVIEW
	case approval.KindMaskProposal:
		return approvalv1.ApprovalKind_APPROVAL_KIND_MASK_PROPOSAL
	case approval.KindOffboardExport:
		return approvalv1.ApprovalKind_APPROVAL_KIND_OFFBOARD_EXPORT
	case approval.KindEvolutionSuggestion:
		return approvalv1.ApprovalKind_APPROVAL_KIND_EVOLUTION_SUGGESTION
	case approval.KindSkillApproval:
		return approvalv1.ApprovalKind_APPROVAL_KIND_SKILL_APPROVAL
	case approval.KindScheduleDraft:
		return approvalv1.ApprovalKind_APPROVAL_KIND_SCHEDULE_DRAFT
	case approval.KindMemoryFact:
		return approvalv1.ApprovalKind_APPROVAL_KIND_MEMORY_FACT
	default:
		return approvalv1.ApprovalKind_APPROVAL_KIND_UNSPECIFIED
	}
}

// approvalKindFromProto 解析类别过滤：UNSPECIFIED → （空串=全部, true）；
// 已定义类别 → （biz 常量, true）；未识别枚举值 → （"", false）。
func approvalKindFromProto(k approvalv1.ApprovalKind) (string, bool) {
	switch k {
	case approvalv1.ApprovalKind_APPROVAL_KIND_UNSPECIFIED:
		return "", true
	case approvalv1.ApprovalKind_APPROVAL_KIND_TASK_REVIEW:
		return approval.KindTaskReview, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_MASK_PROPOSAL:
		return approval.KindMaskProposal, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_OFFBOARD_EXPORT:
		return approval.KindOffboardExport, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_EVOLUTION_SUGGESTION:
		return approval.KindEvolutionSuggestion, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_SKILL_APPROVAL:
		return approval.KindSkillApproval, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_SCHEDULE_DRAFT:
		return approval.KindScheduleDraft, true
	case approvalv1.ApprovalKind_APPROVAL_KIND_MEMORY_FACT:
		return approval.KindMemoryFact, true
	default:
		return "", false
	}
}

func approvalRiskToProto(risk string) approvalv1.RiskLevel {
	switch risk {
	case approval.RiskLow:
		return approvalv1.RiskLevel_RISK_LEVEL_LOW
	case approval.RiskMedium:
		return approvalv1.RiskLevel_RISK_LEVEL_MEDIUM
	case approval.RiskHigh:
		return approvalv1.RiskLevel_RISK_LEVEL_HIGH
	default:
		return approvalv1.RiskLevel_RISK_LEVEL_UNSPECIFIED
	}
}

// approvalRiskFromProto 矩阵行反解：UNSPECIFIED/未知 → ""（交校验层拒绝）。
func approvalRiskFromProto(r approvalv1.RiskLevel) string {
	switch r {
	case approvalv1.RiskLevel_RISK_LEVEL_LOW:
		return approval.RiskLow
	case approvalv1.RiskLevel_RISK_LEVEL_MEDIUM:
		return approval.RiskMedium
	case approvalv1.RiskLevel_RISK_LEVEL_HIGH:
		return approval.RiskHigh
	default:
		return ""
	}
}

func toProtoApprovalItem(it approval.Item) *approvalv1.ApprovalItem {
	out := &approvalv1.ApprovalItem{
		Kind:        approvalKindToProto(it.Kind),
		Id:          it.ID,
		Title:       it.Title,
		Summary:     it.Summary,
		ContextJson: it.ContextJSON,
		RiskLevel:   approvalRiskToProto(it.RiskLevel),
		Requester:   it.Requester,
		AgentKey:    it.AgentKey,
	}
	if !it.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(it.CreatedAt)
	}
	return out
}

func toProtoMatrixRow(r approval.RiskMatrixRow) *approvalv1.RiskMatrixRow {
	out := &approvalv1.RiskMatrixRow{
		MatcherKey:       r.MatcherKey,
		Risk:             approvalRiskToProto(r.Risk),
		DefaultMode:      r.DefaultMode,
		AutoApproveTimes: int32(r.AutoApproveTimes),
		UpdatedBy:        r.UpdatedBy,
	}
	if !r.UpdatedAt.IsZero() {
		out.UpdatedAt = timestamppb.New(r.UpdatedAt)
	}
	return out
}

// ---- RPC ----

func (s *ApprovalCenterService) ListApprovals(ctx context.Context, req *approvalv1.ListApprovalsRequest) (*approvalv1.ListApprovalsResponse, error) {
	kind, ok := approvalKindFromProto(req.GetKind())
	if !ok {
		return nil, apierror.BadRequest(apierror.DomainApproval, "unknown approval kind: %s", req.GetKind().String())
	}
	items, total, err := s.agg.List(ctx, approval.ViewerFromContext(ctx), kind, req.GetRequester(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		if approval.IsUnknownKind(err) {
			return nil, apierror.BadRequest(apierror.DomainApproval, "%s", err.Error())
		}
		return nil, err
	}
	out := &approvalv1.ListApprovalsResponse{Total: total}
	for _, it := range items {
		out.Items = append(out.Items, toProtoApprovalItem(it))
	}
	return out, nil
}

func (s *ApprovalCenterService) GetApprovalSummary(ctx context.Context, _ *emptypb.Empty) (*approvalv1.ApprovalSummary, error) {
	viewer := approval.ViewerFromContext(ctx)
	counts, total, err := s.agg.Summary(ctx, viewer)
	if err != nil {
		return nil, err
	}
	out := &approvalv1.ApprovalSummary{Total: total}
	// 稳定输出序=注册顺序；已注册但当前为零的类别以 0 出现（前端空 lane）。
	for _, k := range s.agg.Kinds() {
		out.Counts = append(out.Counts, &approvalv1.ApprovalKindCount{
			Kind:  approvalKindToProto(k),
			Count: counts[k],
		})
	}
	return out, nil
}

func (s *ApprovalCenterService) GetRiskMatrix(ctx context.Context, _ *emptypb.Empty) (*approvalv1.GetRiskMatrixResponse, error) {
	rows, err := s.matrix.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &approvalv1.GetRiskMatrixResponse{}
	for _, r := range rows {
		out.Rows = append(out.Rows, toProtoMatrixRow(r))
	}
	return out, nil
}

func (s *ApprovalCenterService) PutRiskMatrix(ctx context.Context, req *approvalv1.PutRiskMatrixRequest) (*approvalv1.GetRiskMatrixResponse, error) {
	rows := make([]approval.RiskMatrixRow, 0, len(req.GetRows()))
	for _, r := range req.GetRows() {
		rows = append(rows, approval.RiskMatrixRow{
			MatcherKey:       r.GetMatcherKey(),
			Risk:             approvalRiskFromProto(r.GetRisk()),
			DefaultMode:      r.GetDefaultMode(),
			AutoApproveTimes: int(r.GetAutoApproveTimes()),
		})
	}
	// service 层先验（错误文案直达调用方）；biz Matrix.Put 内再验一次兜底。
	if err := approval.ValidateRiskMatrixRows(rows); err != nil {
		return nil, apierror.BadRequest(apierror.DomainApproval, "%s", err.Error())
	}
	if err := s.matrix.Put(ctx, approval.ViewerFromContext(ctx), rows); err != nil {
		return nil, err
	}
	return s.GetRiskMatrix(ctx, &emptypb.Empty{})
}

// ---- 矩阵变更审计适配（拍板 A-4：配置存库可审计）----

// matrixAuditSink 将 approval.MatrixAuditSink 适配到 monitor.Usecase 的管理
// 审计面（RecordAdminAudit best-effort：失败仅告警不阻断，见 audit_record.go）。
type matrixAuditSink struct {
	mon *monitor.Usecase
}

// NewMatrixAuditSink 装配矩阵审计口；mon 为 nil 时审计静默关闭（存取仍可用）。
func NewMatrixAuditSink(mon *monitor.Usecase) approval.MatrixAuditSink {
	if mon == nil {
		return nil
	}
	return &matrixAuditSink{mon: mon}
}

func (s *matrixAuditSink) RecordMatrixChange(ctx context.Context, actor string, rows []approval.RiskMatrixRow) error {
	after := make(map[string]any, len(rows))
	for _, r := range rows {
		after[r.MatcherKey] = map[string]any{
			"risk":               r.Risk,
			"default_mode":       r.DefaultMode,
			"auto_approve_times": r.AutoApproveTimes,
		}
	}
	s.mon.RecordAdminAudit(ctx, monitor.AdminAuditEntry{
		Action:   monitor.AuditAction(monitor.AuditVerbUpdate, "approval_risk_matrix"),
		Resource: "approval_risk_matrix",
		Actor:    actor,
		Summary:  "rows=" + strconv.Itoa(len(rows)),
		After:    after,
	})
	return nil
}

// ---- 统一收件箱（工作台与经营看板重新设计 §4.1）----

// WithInbox 挂接统一收件箱聚合器；未挂接时三个收件箱 RPC 返回 503。
func (s *ApprovalCenterService) WithInbox(agg *inbox.Aggregator) *ApprovalCenterService {
	s.inbox = agg
	return s
}

func (s *ApprovalCenterService) inboxOrErr() (*inbox.Aggregator, error) {
	if s == nil || s.inbox == nil {
		return nil, apierror.Unavailable(apierror.DomainApproval, "inbox is not configured")
	}
	return s.inbox, nil
}

// mapInboxError 把聚合器哨兵错误映射为 400/404/409；领域错误原样透传。
func mapInboxError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, inbox.ErrInvalidID), errors.Is(err, inbox.ErrActionNotAllowed), errors.Is(err, inbox.ErrNoteRequired):
		return apierror.BadRequest(apierror.DomainApproval, "%s", err.Error())
	case errors.Is(err, inbox.ErrUnknownSource):
		return apierror.BadRequest(apierror.DomainApproval, "%s", err.Error())
	case errors.Is(err, inbox.ErrGone):
		return apierror.Conflict(apierror.DomainApproval, "%s", err.Error())
	default:
		return err
	}
}

func toProtoInboxItem(it inbox.Item) *approvalv1.InboxItem {
	out := &approvalv1.InboxItem{
		Id:                 it.ID,
		Source:             it.Source,
		Kind:               it.Kind,
		Title:              it.Title,
		Summary:            it.Summary,
		RiskLevel:          approvalRiskToProto(it.Risk),
		Requester:          it.Requester,
		AgentKey:           it.AgentKey,
		Actions:            append([]string{}, it.Actions...),
		RejectRequiresNote: it.RejectRequiresNote,
		Link:               it.Link,
		ContextJson:        it.ContextJSON,
	}
	if !it.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(it.CreatedAt)
	}
	if !it.DueAt.IsZero() {
		out.DueAt = timestamppb.New(it.DueAt)
	}
	return out
}

func toProtoInboxCounts(res inbox.Result) []*approvalv1.InboxSourceCount {
	out := make([]*approvalv1.InboxSourceCount, 0, len(res.Order))
	for _, name := range res.Order {
		out = append(out, &approvalv1.InboxSourceCount{Source: name, Count: res.Counts[name]})
	}
	return out
}

func (s *ApprovalCenterService) ListInbox(ctx context.Context, req *approvalv1.ListInboxRequest) (*approvalv1.ListInboxResponse, error) {
	agg, err := s.inboxOrErr()
	if err != nil {
		return nil, err
	}
	res, err := agg.List(ctx, req.GetSources(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapInboxError(err)
	}
	out := &approvalv1.ListInboxResponse{
		Total:           res.Total,
		Counts:          toProtoInboxCounts(res),
		DegradedSources: res.Degraded,
	}
	for _, it := range res.Items {
		out.Items = append(out.Items, toProtoInboxItem(it))
	}
	return out, nil
}

func (s *ApprovalCenterService) GetInboxSummary(ctx context.Context, _ *emptypb.Empty) (*approvalv1.InboxSummary, error) {
	agg, err := s.inboxOrErr()
	if err != nil {
		return nil, err
	}
	res := agg.Summary(ctx)
	return &approvalv1.InboxSummary{
		Total:           res.Total,
		HighRisk:        res.HighRisk,
		Counts:          toProtoInboxCounts(res),
		DegradedSources: res.Degraded,
	}, nil
}

func (s *ApprovalCenterService) DecideInboxItem(ctx context.Context, req *approvalv1.DecideInboxItemRequest) (*approvalv1.DecideInboxItemResponse, error) {
	agg, err := s.inboxOrErr()
	if err != nil {
		return nil, err
	}
	item, err := agg.Decide(ctx, req.GetId(), req.GetAction(), req.GetNote())
	if err != nil {
		return nil, mapInboxError(err)
	}
	return &approvalv1.DecideInboxItemResponse{
		Id:        item.ID,
		Action:    strings.ToLower(strings.TrimSpace(req.GetAction())),
		DecidedAt: timestamppb.Now(),
	}, nil
}
