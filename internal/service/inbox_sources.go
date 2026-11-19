package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	agentv1 "aranea-agents/api/kratos/agent/v1"
	officev1 "aranea-agents/api/kratos/office/v1"
	skillevov1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ============================================================================
// 统一收件箱的来源适配（工作台与经营看板重新设计 §4.1）。
//
// 每个来源复用所属领域既有的 service 方法做读与写——这些方法自带租户隔离、
// 归属校验与并发语义，收件箱不另开旁路。进程内直调会绕过 HTTP/gRPC 的权限
// 中间件，所以凡是对应 RPC 登记了权限码的来源，这里用同一个权限码显式过闸
// （inboxCan）：无权 = 空来源，与「不返回无权对象」一致。
// ============================================================================

// 收件箱来源名（审批七类沿用 approval.Kind* 常量）。
const (
	inboxSourceHITL        = "hitl_confirm"
	inboxSourcePublish     = "publish_review"
	inboxSourceOpportunity = "opportunity"
	inboxSourceDeadLetter  = "dead_letter"
	inboxSourceCronFailure = "cron_failure"
)

// 与各领域 RPC 注解一致的权限码（见 permission_registry_gen.go）。
const (
	inboxPermReviewView   = "review.view"
	inboxPermReviewDecide = "review.decide"
	inboxPermCron         = "system.cron"
)

const (
	// 工具确认的可签窗口：后端确认等待默认 5 分钟，留 6 倍容错；超窗的不进收件箱。
	inboxHITLWindow = 30 * time.Minute
	// 工具确认 asked/decided 配对的回看窗。
	inboxHITLLookback = 48 * time.Hour
	// 定时任务失败的统计窗：更早的失败不再算积压。
	inboxCronWindow = 7 * 24 * time.Hour
	// 按 correlation_id 批量回查的分批大小。
	inboxCorrelationBatch = 50
)

// InboxDeps 是收件箱来源依赖的领域服务；为 nil 的依赖对应来源不注册。
type InboxDeps struct {
	Approvals   *approval.Aggregator
	Bridge      *AgentBridgeService
	Agents      *AgentService
	SkillEvo    *SkillEvolutionSuggestionService
	Schedule    *OfficeScheduleService
	Chat        *ChatService
	Decisions   *DecisionRecordService
	Review      *ReviewService
	Teams       *TeamService
	Cron        *CronService
	nowOverride func() time.Time // 测试注入
}

func (d InboxDeps) now() time.Time {
	if d.nowOverride != nil {
		return d.nowOverride()
	}
	return time.Now()
}

// NewInboxAggregator 按依赖装配收件箱聚合器。注册顺序即计数输出顺序。
func NewInboxAggregator(d InboxDeps) *inbox.Aggregator {
	var sources []inbox.Source
	if d.Approvals != nil {
		for _, kind := range d.Approvals.Kinds() {
			sources = append(sources, &inboxApprovalSource{kind: kind, deps: d})
		}
	}
	if d.Decisions != nil && d.Chat != nil {
		sources = append(sources, &inboxHITLSource{deps: d})
	}
	if d.Review != nil {
		sources = append(sources, &inboxPublishSource{review: d.Review}, &inboxOpportunitySource{review: d.Review})
	}
	if d.Teams != nil {
		sources = append(sources, &inboxDeadLetterSource{teams: d.Teams})
	}
	if d.Cron != nil {
		sources = append(sources, &inboxCronSource{deps: d})
	}
	return inbox.NewAggregator(sources...)
}

func inboxCan(ctx context.Context, code string) bool {
	a, ok := auth.FromContext(ctx)
	return ok && a != nil && a.HasPermission(code)
}

func inboxActor(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return ""
}

func inboxClip(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func inboxParseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

func inboxTS(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func errInboxUnsupported(source, action string) error {
	return apierror.BadRequest(apierror.DomainApproval, "inbox source %s does not support action %s", source, action)
}

// ---------------------------------------------------------------------------
// 审批七类：读走 approval.Aggregator（逐类权限闸在其来源内完成）。
// ---------------------------------------------------------------------------

type inboxApprovalSource struct {
	kind string
	deps InboxDeps
}

func (s *inboxApprovalSource) Name() string { return s.kind }

// decidable 报告该审批类别当前是否具备服务端决议出口。
func (s *inboxApprovalSource) decidable() bool {
	switch s.kind {
	case approval.KindTaskReview:
		return s.deps.Bridge != nil
	case approval.KindEvolutionSuggestion:
		return s.deps.Agents != nil
	case approval.KindSkillApproval:
		return s.deps.SkillEvo != nil
	case approval.KindScheduleDraft:
		return s.deps.Schedule != nil
	default:
		// mask_proposal / offboard_export / memory_fact：决议留在各自专页
		//（多步流程或专用表单），收件箱只展示并深链。
		return false
	}
}

func (s *inboxApprovalSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if limit > 200 {
		limit = 200 // approval.Aggregator 单页上限
	}
	rows, total, err := s.deps.Approvals.List(ctx, approval.ViewerFromContext(ctx), s.kind, "", 1, limit)
	if err != nil {
		return nil, 0, err
	}
	kind := inbox.KindDecide
	if s.kind == approval.KindTaskReview {
		kind = inbox.KindReview
	}
	var actions []string
	if s.decidable() {
		actions = []string{inbox.ActionApprove, inbox.ActionReject}
	}
	out := make([]inbox.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, inbox.Item{
			NativeID:           r.ID,
			Kind:               kind,
			Title:              r.Title,
			Summary:            r.Summary,
			Risk:               r.RiskLevel,
			Requester:          r.Requester,
			AgentKey:           r.AgentKey,
			CreatedAt:          r.CreatedAt,
			Actions:            actions,
			RejectRequiresNote: true,
			Link:               "/approvals",
			ContextJSON:        r.ContextJSON,
		})
	}
	return out, total, nil
}

func (s *inboxApprovalSource) Decide(ctx context.Context, item inbox.Item, action, note string) error {
	approve := action == inbox.ActionApprove
	switch s.kind {
	case approval.KindTaskReview:
		if !inboxCan(ctx, approval.PermCodingDispatch) {
			return apierror.Forbidden(apierror.DomainApproval, "permission denied")
		}
		_, err := s.deps.Bridge.ReviewTask(ctx, item.NativeID, action, note)
		return err
	case approval.KindEvolutionSuggestion:
		var c struct {
			TargetID string `json:"target_id"`
		}
		_ = json.Unmarshal([]byte(item.ContextJSON), &c)
		if strings.TrimSpace(c.TargetID) == "" {
			return apierror.FailedPrecondition(apierror.DomainApproval, "evolution suggestion has no target agent")
		}
		if approve {
			_, err := s.deps.Agents.ApplyEvolutionSuggestion(ctx, &agentv1.ApplyEvolutionSuggestionRequest{AgentId: c.TargetID, SuggestionId: item.NativeID})
			return err
		}
		_, err := s.deps.Agents.RejectEvolutionSuggestion(ctx, &agentv1.RejectEvolutionSuggestionRequest{AgentId: c.TargetID, SuggestionId: item.NativeID, Reason: note})
		return err
	case approval.KindSkillApproval:
		actor := inboxActor(ctx)
		if approve {
			_, err := s.deps.SkillEvo.ApproveSkillEvolutionSuggestion(ctx, &skillevov1.ApproveSkillEvolutionSuggestionRequest{Id: item.NativeID, ApprovedBy: actor})
			return err
		}
		_, err := s.deps.SkillEvo.RejectSkillEvolutionSuggestion(ctx, &skillevov1.RejectSkillEvolutionSuggestionRequest{Id: item.NativeID, RejectedBy: actor, RejectionReason: note})
		return err
	case approval.KindScheduleDraft:
		if !inboxCan(ctx, approval.PermOfficeSchedule) {
			return apierror.Forbidden(apierror.DomainApproval, "permission denied")
		}
		if approve {
			_, err := s.deps.Schedule.ApproveScheduleDraft(ctx, &officev1.ApproveScheduleDraftRequest{Id: item.NativeID})
			return err
		}
		_, err := s.deps.Schedule.RejectScheduleDraft(ctx, &officev1.RejectScheduleDraftRequest{Id: item.NativeID, Reason: note})
		return err
	default:
		return errInboxUnsupported(s.kind, action)
	}
}
