package service

import (
	"context"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	decisionv1 "aranea-agents/api/kratos/decision/v1"
	"aranea-agents/internal/biz/inbox"
	"aranea-agents/pkg/apierror"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// 工具确认（HITL）：decision_records 里 asked 且无同 correlation_id 的已决记录，
// 且仍在可签窗口内、能定位到会话的，才算待办。
// ---------------------------------------------------------------------------

type inboxHITLSource struct {
	deps InboxDeps
}

func (s *inboxHITLSource) Name() string { return inboxSourceHITL }

func (s *inboxHITLSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	now := s.deps.now()
	records, err := s.fetch(ctx, now)
	if err != nil {
		return nil, 0, err
	}
	items := inboxPendingHITL(records, now)
	total := int64(len(items))
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// fetch 取回看窗内的 HITL 记录，并按 correlation_id 回查补齐配对（已决的那一
// 半可能被分页或时间窗挤出，不补会把已处理的误判为待办）。
func (s *inboxHITLSource) fetch(ctx context.Context, now time.Time) ([]*decisionv1.DecisionRecord, error) {
	const category = "hitl_approval"
	since := timestamppb.New(now.Add(-inboxHITLLookback))
	first, err := s.deps.Decisions.ListDecisionRecords(ctx, &decisionv1.ListDecisionRecordsRequest{Category: category, TimeFrom: since, Page: 1, PageSize: 100})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var all []*decisionv1.DecisionRecord
	add := func(rows []*decisionv1.DecisionRecord) {
		for _, r := range rows {
			if r == nil || seen[r.GetDecisionKey()] {
				continue
			}
			seen[r.GetDecisionKey()] = true
			all = append(all, r)
		}
	}
	add(first.GetItems())
	if first.GetTotal() > int64(len(first.GetItems())) {
		if second, err := s.deps.Decisions.ListDecisionRecords(ctx, &decisionv1.ListDecisionRecordsRequest{Category: category, TimeFrom: since, Page: 2, PageSize: 100}); err == nil {
			add(second.GetItems())
		}
	}
	var cids []string
	cidSeen := map[string]bool{}
	for _, r := range all {
		if !inboxHITLAsked(r) {
			continue
		}
		if cid := inboxHITLCorrelation(r); cid != "" && !cidSeen[cid] {
			cidSeen[cid] = true
			cids = append(cids, cid)
		}
	}
	for i := 0; i < len(cids); i += inboxCorrelationBatch {
		end := i + inboxCorrelationBatch
		if end > len(cids) {
			end = len(cids)
		}
		extra, err := s.deps.Decisions.ListDecisionRecords(ctx, &decisionv1.ListDecisionRecordsRequest{Category: category, CorrelationIds: cids[i:end], Page: 1, PageSize: 100})
		if err != nil {
			// 配对补查失败时宁可整源降级，也不把可能已处理的条目当待办交给用户。
			return nil, err
		}
		add(extra.GetItems())
	}
	return all, nil
}

func inboxHITLAsked(r *decisionv1.DecisionRecord) bool {
	return strings.EqualFold(strings.TrimSpace(r.GetOutcome()), "asked")
}

func inboxStructString(s *structpb.Struct, keys ...string) string {
	if s == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := s.GetFields()[k]; ok {
			if str := strings.TrimSpace(v.GetStringValue()); str != "" {
				return str
			}
		}
	}
	return ""
}

func inboxHITLCorrelation(r *decisionv1.DecisionRecord) string {
	return inboxStructString(r.GetMetadata(), "correlation_id", "correlationId")
}

func inboxHITLAgent(r *decisionv1.DecisionRecord) string {
	for _, v := range r.GetRelatedEntities().GetValues() {
		e := v.GetStructValue()
		if strings.EqualFold(inboxStructString(e, "type", "entity_type"), "agent") {
			return inboxStructString(e, "key", "entity_key")
		}
	}
	return ""
}

// inboxPendingHITL 是纯函数：从记录集中挑出仍可签的待确认条目。
func inboxPendingHITL(records []*decisionv1.DecisionRecord, now time.Time) []inbox.Item {
	decided := map[string]bool{}
	for _, r := range records {
		if inboxHITLAsked(r) {
			continue
		}
		if cid := inboxHITLCorrelation(r); cid != "" {
			decided[cid] = true
		}
	}
	var out []inbox.Item
	for _, r := range records {
		if !inboxHITLAsked(r) {
			continue
		}
		cid := inboxHITLCorrelation(r)
		if cid == "" || decided[cid] {
			continue
		}
		sessionID := strings.TrimSpace(r.GetSessionId())
		if sessionID == "" {
			sessionID = inboxStructString(r.GetSourceRef(), "session_id")
		}
		created := inboxTS(r.GetCreatedAt())
		if sessionID == "" || created.IsZero() || now.Sub(created) > inboxHITLWindow {
			continue
		}
		agentKey := inboxHITLAgent(r)
		requester := agentKey
		if requester == "" {
			requester = r.GetActorKey()
		}
		title := r.GetReasoning()
		if strings.TrimSpace(title) == "" {
			title = r.GetScenario()
		}
		out = append(out, inbox.Item{
			NativeID:  r.GetDecisionKey(),
			Kind:      inbox.KindConfirm,
			Title:     inboxClip(title, 80),
			Summary:   inboxClip(r.GetScenario(), 200),
			Risk:      inbox.RiskHigh,
			Requester: requester,
			AgentKey:  agentKey,
			CreatedAt: created,
			DueAt:     created.Add(inboxHITLWindow),
			Actions:   []string{inbox.ActionApprove, inbox.ActionReject},
			Link:      "/chat?session=" + sessionID,
			Ref:       map[string]string{"session_id": sessionID, "activity_id": cid},
		})
	}
	return out
}

func (s *inboxHITLSource) Decide(ctx context.Context, item inbox.Item, action, _ string) error {
	approved := action == inbox.ActionApprove
	res, err := s.deps.Chat.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  item.Ref["session_id"],
		ActivityId: item.Ref["activity_id"],
		Approved:   &approved,
	})
	if err != nil {
		return err
	}
	if !res.GetAccepted() {
		return apierror.Conflict(apierror.DomainApproval, "confirmation is no longer awaiting a decision")
	}
	return nil
}
