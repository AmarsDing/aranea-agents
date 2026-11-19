package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	reviewv1 "aranea-agents/api/kratos/review/v1"
	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/mvp"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReviewService implements kratos review.v1（E 阶段 W1b：MVP 发文审批/商机
// 签收的 proto 收编，需求文档 §5.3 拍板 A-2）。
//
// 相对旧裸路由（internal/service/mvp_media_http.go 已下线的五路由）的行为修正：
//   - reviewer 一律取 JWT 主体（auth.FromContext），请求体自报字段废除；
//   - 不可自批：内容创建者 == reviewer 时服务端 400（biz 层强制）；
//   - Reject 强制 note（本层先验，biz 层兜底）；
//   - approve 后联动原样保留：MarkApproved → CreateFromApprovedReview →
//     ProcessNext(1) → mvp.AfterReviewDecision（图执行恢复）；
//   - 并发决议 CAS：ErrConflict / 非法状态迁移 → 409；任务不存在 → 404。
type ReviewService struct {
	reviewv1.UnimplementedReviewServiceServer
}

func NewReviewService() *ReviewService {
	return &ReviewService{}
}

// reviewerIDFromContext 从 JWT 主体取决议者 id（禁请求体自报，A-2 安全红线）。
func reviewerIDFromContext(ctx context.Context) (string, error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil || a.UserID <= 0 {
		return "", apierror.Unauthorized(apierror.DomainReview, "authenticated reviewer required")
	}
	return strconv.FormatInt(a.UserID, 10), nil
}

// mapReviewError 将 biz/review 哨兵错误映射为统一错误模型（中间件转 kratos
// HTTP 状态码）：404/400/409，其余原样透传。
func mapReviewError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, review.ErrNotFound):
		return apierror.NotFound(apierror.DomainReview, "%s", err.Error())
	case errors.Is(err, review.ErrSelfApproval):
		return apierror.BadRequest(apierror.DomainReview, "self-approval forbidden")
	case errors.Is(err, review.ErrConflict), errors.Is(err, review.ErrIllegalTransition):
		return apierror.Conflict(apierror.DomainReview, "%s", err.Error())
	default:
		return err
	}
}

func toProtoReviewTask(t review.Task) *reviewv1.ReviewTaskView {
	flagsJSON := "[]"
	if raw, err := json.Marshal(t.MachineFlags); err == nil && len(raw) > 0 {
		flagsJSON = string(raw)
	}
	out := &reviewv1.ReviewTaskView{
		Id:               t.ID,
		ContentItemId:    t.ContentItemID,
		DraftTitle:       t.DraftTitle,
		DraftBody:        t.DraftBody,
		MachineFlagsJson: flagsJSON,
		PlatformTargets:  append([]string{}, t.PlatformTargets...),
		Status:           t.Status,
		DecisionNote:     t.DecisionNote,
		ReviewerId:       t.ReviewerID,
	}
	if t.DecidedAt != nil {
		out.DecidedAt = timestamppb.New(*t.DecidedAt)
	}
	if !t.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(t.CreatedAt)
	}
	return out
}

func toProtoOpportunity(o commerce.Opportunity) *reviewv1.OpportunityView {
	out := &reviewv1.OpportunityView{
		Id:      o.ID,
		Title:   o.Title,
		Summary: o.Summary,
		Source:  o.Audience,
		Status:  o.Status,
	}
	if !o.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(o.CreatedAt)
	}
	return out
}

func reviewPage(reqPage, reqPageSize int32) (limit, offset int) {
	page := int(reqPage)
	if page <= 0 {
		page = 1
	}
	pageSize := int(reqPageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageSize, (page - 1) * pageSize
}

func (s *ReviewService) ListPendingReviews(ctx context.Context, req *reviewv1.ListPendingReviewsRequest) (*reviewv1.ListPendingReviewsResponse, error) {
	limit, offset := reviewPage(req.GetPage(), req.GetPageSize())
	items, total, err := mvp.Ensure().Review.ListPending(limit, offset)
	if err != nil {
		return nil, mapReviewError(err)
	}
	out := &reviewv1.ListPendingReviewsResponse{Total: int64(total)}
	for _, t := range items {
		out.Items = append(out.Items, toProtoReviewTask(t))
	}
	return out, nil
}

func (s *ReviewService) ApproveReview(ctx context.Context, req *reviewv1.ApproveReviewRequest) (*reviewv1.ReviewTaskView, error) {
	reviewerID, err := reviewerIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainReview, "id is required")
	}
	rt := mvp.Ensure()
	task, err := rt.Review.Approve(id, reviewerID, req.GetNote())
	if err != nil {
		return nil, mapReviewError(err)
	}
	// 联动与旧裸路由一致（决议已落库后的下游步骤失败不否决决议本身）：
	// 内容标记 → 打包 → 处理一件 → 图执行恢复。
	if task.ContentItemID != "" {
		_, _ = rt.Content.MarkApproved(task.ContentItemID)
	}
	platform := "generic"
	for _, p := range task.PlatformTargets {
		if strings.TrimSpace(p) != "" {
			platform = strings.TrimSpace(p)
			break
		}
	}
	if _, err := rt.Publish.CreateFromApprovedReview(task, platform); err != nil {
		return nil, mapReviewError(err)
	}
	_, _ = rt.Publish.ProcessNext(1)
	_ = mvp.AfterReviewDecision(task)
	return toProtoReviewTask(task), nil
}

func (s *ReviewService) RejectReview(ctx context.Context, req *reviewv1.RejectReviewRequest) (*reviewv1.ReviewTaskView, error) {
	reviewerID, err := reviewerIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainReview, "id is required")
	}
	note := strings.TrimSpace(req.GetNote())
	if note == "" {
		return nil, apierror.BadRequest(apierror.DomainReview, "reject requires note")
	}
	rt := mvp.Ensure()
	task, err := rt.Review.Reject(id, reviewerID, note)
	if err != nil {
		return nil, mapReviewError(err)
	}
	if task.ContentItemID != "" {
		_, _ = rt.Content.MarkRejected(task.ContentItemID)
	}
	_ = mvp.AfterReviewDecision(task)
	return toProtoReviewTask(task), nil
}

func (s *ReviewService) ListOpportunities(ctx context.Context, req *reviewv1.ListOpportunitiesRequest) (*reviewv1.ListOpportunitiesResponse, error) {
	limit, offset := reviewPage(req.GetPage(), req.GetPageSize())
	items, total, err := mvp.Ensure().Opportunity.List("", "", limit, offset)
	if err != nil {
		return nil, mapReviewError(err)
	}
	out := &reviewv1.ListOpportunitiesResponse{Total: int64(total)}
	for _, o := range items {
		out.Items = append(out.Items, toProtoOpportunity(o))
	}
	return out, nil
}

func (s *ReviewService) AckOpportunity(ctx context.Context, req *reviewv1.AckOpportunityRequest) (*reviewv1.OpportunityView, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainReview, "id is required")
	}
	o, err := mvp.Ensure().Opportunity.Acknowledge(id)
	if err != nil {
		if errors.Is(err, commerce.ErrOpportunityNotFound) {
			return nil, apierror.NotFound(apierror.DomainReview, "%s", err.Error())
		}
		return nil, err
	}
	return toProtoOpportunity(o), nil
}
