package service

import (
	"context"
	"strings"
	"time"

	cronv1 "aranea-agents/api/kratos/cron/v1"
	reviewv1 "aranea-agents/api/kratos/review/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/pkg/apierror"
)

// ---------------------------------------------------------------------------
// 发文评审
// ---------------------------------------------------------------------------

type inboxPublishSource struct {
	review *ReviewService
}

func (s *inboxPublishSource) Name() string { return inboxSourcePublish }

func (s *inboxPublishSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if !inboxCan(ctx, inboxPermReviewView) {
		return nil, 0, nil
	}
	if limit > 200 {
		limit = 200
	}
	res, err := s.review.ListPendingReviews(ctx, &reviewv1.ListPendingReviewsRequest{Page: 1, PageSize: int32(limit)})
	if err != nil {
		return nil, 0, err
	}
	var actions []string
	if inboxCan(ctx, inboxPermReviewDecide) {
		actions = []string{inbox.ActionApprove, inbox.ActionReject}
	}
	out := make([]inbox.Item, 0, len(res.GetItems()))
	for _, t := range res.GetItems() {
		title := t.GetDraftTitle()
		if strings.TrimSpace(title) == "" {
			title = t.GetDraftBody()
		}
		out = append(out, inbox.Item{
			NativeID:           t.GetId(),
			Kind:               inbox.KindDecide,
			Title:              inboxClip(title, 80),
			Summary:            inboxClip(t.GetDraftBody(), 200),
			Risk:               inbox.RiskMedium,
			Requester:          strings.Join(t.GetPlatformTargets(), " "),
			CreatedAt:          inboxTS(t.GetCreatedAt()),
			Actions:            actions,
			RejectRequiresNote: true,
			Link:               "/review-queue",
		})
	}
	return out, res.GetTotal(), nil
}

func (s *inboxPublishSource) Decide(ctx context.Context, item inbox.Item, action, note string) error {
	if !inboxCan(ctx, inboxPermReviewDecide) {
		return apierror.Forbidden(apierror.DomainApproval, "permission denied")
	}
	if action == inbox.ActionApprove {
		_, err := s.review.ApproveReview(ctx, &reviewv1.ApproveReviewRequest{Id: item.NativeID, Note: note})
		return err
	}
	_, err := s.review.RejectReview(ctx, &reviewv1.RejectReviewRequest{Id: item.NativeID, Note: note})
	return err
}

// ---------------------------------------------------------------------------
// 上架确认（商机签收）
// ---------------------------------------------------------------------------

type inboxOpportunitySource struct {
	review *ReviewService
}

func (s *inboxOpportunitySource) Name() string { return inboxSourceOpportunity }

func inboxOpportunityOpen(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "pending", "new", "open":
		return true
	default:
		return false
	}
}

func (s *inboxOpportunitySource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if !inboxCan(ctx, inboxPermReviewView) {
		return nil, 0, nil
	}
	// 商机列表接口不按状态过滤，这里取一页后筛出未签收的。
	res, err := s.review.ListOpportunities(ctx, &reviewv1.ListOpportunitiesRequest{Page: 1, PageSize: 200})
	if err != nil {
		return nil, 0, err
	}
	var actions []string
	if inboxCan(ctx, inboxPermReviewDecide) {
		actions = []string{inbox.ActionAck}
	}
	var out []inbox.Item
	for _, o := range res.GetItems() {
		if !inboxOpportunityOpen(o.GetStatus()) {
			continue
		}
		out = append(out, inbox.Item{
			NativeID:  o.GetId(),
			Kind:      inbox.KindDecide,
			Title:     inboxClip(o.GetTitle(), 80),
			Summary:   inboxClip(o.GetSummary(), 200),
			Risk:      inbox.RiskLow,
			Requester: o.GetSource(),
			CreatedAt: inboxTS(o.GetCreatedAt()),
			Actions:   actions,
			Link:      "/opportunity-board",
		})
	}
	total := int64(len(out))
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

func (s *inboxOpportunitySource) Decide(ctx context.Context, item inbox.Item, _, _ string) error {
	if !inboxCan(ctx, inboxPermReviewDecide) {
		return apierror.Forbidden(apierror.DomainApproval, "permission denied")
	}
	_, err := s.review.AckOpportunity(ctx, &reviewv1.AckOpportunityRequest{Id: item.NativeID})
	return err
}

// ---------------------------------------------------------------------------
// 任务死信
// ---------------------------------------------------------------------------

type inboxDeadLetterSource struct {
	teams *TeamService
}

func (s *inboxDeadLetterSource) Name() string { return inboxSourceDeadLetter }

func (s *inboxDeadLetterSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if limit > 200 {
		limit = 200
	}
	res, err := s.teams.ListTaskDeadLetters(ctx, &teamv1.ListTaskDeadLettersRequest{WorkspaceScope: true, Status: "pending", Limit: int32(limit)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]inbox.Item, 0, len(res.GetItems()))
	for _, d := range res.GetItems() {
		link := ""
		if sid := strings.TrimSpace(d.GetSessionId()); sid != "" {
			link = "/chat?session=" + sid
		}
		out = append(out, inbox.Item{
			NativeID:  d.GetId(),
			Kind:      inbox.KindException,
			Title:     inboxClip(d.GetErrorMessage(), 80),
			Summary:   inboxClip(d.GetErrorMessage(), 200),
			Risk:      inbox.RiskHigh,
			Requester: d.GetSourceType(),
			CreatedAt: inboxParseTime(d.GetCreatedAt()),
			Actions:   []string{inbox.ActionResolve},
			Link:      link,
		})
	}
	return out, int64(len(out)), nil
}

func (s *inboxDeadLetterSource) Decide(ctx context.Context, item inbox.Item, _, _ string) error {
	_, err := s.teams.ResolveTaskDeadLetter(ctx, &teamv1.ResolveTaskDeadLetterRequest{Id: item.NativeID})
	return err
}

// ---------------------------------------------------------------------------
// 定时任务失败：同一任务反复失败只算一件，取最近一次；核销该次即核销该任务
// 全部未核销失败（CronService.ResolveCronTaskRun 语义）。
// ---------------------------------------------------------------------------

type inboxCronSource struct {
	deps InboxDeps
}

func (s *inboxCronSource) Name() string { return inboxSourceCronFailure }

func (s *inboxCronSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if !inboxCan(ctx, inboxPermCron) {
		return nil, 0, nil
	}
	res, err := s.deps.Cron.ListCronTaskRuns(ctx, &cronv1.ListCronTaskRunsRequest{Status: "failure", Page: 1, PageSize: 200})
	if err != nil {
		return nil, 0, err
	}
	items := inboxCronFailures(res.GetItems(), s.deps.now())
	total := int64(len(items))
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// inboxCronFailures 是纯函数：窗口内失败按任务去重，保留最近一次。
func inboxCronFailures(runs []*cronv1.CronTaskRun, now time.Time) []inbox.Item {
	type latest struct {
		run *cronv1.CronTaskRun
		at  time.Time
	}
	byTask := map[string]latest{}
	var order []string
	for _, r := range runs {
		if r == nil || strings.TrimSpace(r.GetResolvedAt()) != "" {
			continue
		}
		at := inboxParseTime(r.GetStartedAt())
		if at.IsZero() {
			at = inboxParseTime(r.GetCreatedAt())
		}
		if !at.IsZero() && now.Sub(at) > inboxCronWindow {
			continue
		}
		key := r.GetTaskId()
		if key == "" {
			key = r.GetTaskName()
		}
		if key == "" {
			key = r.GetId()
		}
		prev, ok := byTask[key]
		if !ok {
			order = append(order, key)
		}
		if !ok || at.After(prev.at) {
			byTask[key] = latest{run: r, at: at}
		}
	}
	out := make([]inbox.Item, 0, len(order))
	for _, key := range order {
		l := byTask[key]
		name := l.run.GetTaskName()
		if name == "" {
			name = key
		}
		out = append(out, inbox.Item{
			NativeID:  l.run.GetId(),
			Kind:      inbox.KindException,
			Title:     inboxClip(name, 80),
			Summary:   inboxClip(l.run.GetErrorMessage(), 200),
			Risk:      inbox.RiskMedium,
			Requester: name,
			CreatedAt: l.at,
			Actions:   []string{inbox.ActionResolve},
			Link:      "/cron",
		})
	}
	return out
}

func (s *inboxCronSource) Decide(ctx context.Context, item inbox.Item, _, _ string) error {
	if !inboxCan(ctx, inboxPermCron) {
		return apierror.Forbidden(apierror.DomainApproval, "permission denied")
	}
	_, err := s.deps.Cron.ResolveCronTaskRun(ctx, &cronv1.ResolveCronTaskRunRequest{Id: item.NativeID})
	return err
}
