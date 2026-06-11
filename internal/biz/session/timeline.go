package session

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/pkg/apierror"
)

func (uc *SessionUsecase) Timeline(ctx context.Context, id string, q TimelineQuery) (SessionTimeline, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return SessionTimeline{}, validationErr("session id is required")
	}
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return SessionTimeline{}, err
	}
	if strings.TrimSpace(q.KindFilter) == "message" {
		return uc.timelineMessagesOnly(ctx, sess, q)
	}
	return uc.timelineUnionPaged(ctx, sess, q)
}

func (uc *SessionUsecase) timelineMessagesOnly(ctx context.Context, sess Session, q TimelineQuery) (SessionTimeline, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = MessageListDefaultLimit
	}
	if limit > MessageListMaxLimit {
		limit = MessageListMaxLimit
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	res, err := uc.ListMessagesPaged(ctx, sess.ID, limit, offset)
	if err != nil {
		return SessionTimeline{}, err
	}
	items := make([]SessionTimelineItem, 0, len(res.Items))
	for _, msg := range res.Items {
		items = append(items, messageTimelineItem(msg))
	}
	if q.SortOrder == "desc" {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].OccurredAt > items[j].OccurredAt
		})
	}
	summary := sessionTimelineSummary(sess, nil)
	summary.MessageCount = res.Total
	summary.Total = res.Total
	return SessionTimeline{
		SessionID: sess.ID,
		Items:     items,
		Summary:   summary,
	}, nil
}

func (uc *SessionUsecase) timelineUnionPaged(ctx context.Context, sess Session, q TimelineQuery) (SessionTimeline, error) {
	refs, total, err := uc.timelineReader.ListTimelineEventRefsPaged(ctx, sess.ID, q)
	if err != nil {
		return SessionTimeline{}, err
	}
	items, err := uc.hydrateTimelineRefs(ctx, sess.ID, refs)
	if err != nil {
		return SessionTimeline{}, err
	}
	summary := sessionTimelineSummary(sess, nil)
	summary.Total = total
	switch strings.TrimSpace(q.KindFilter) {
	case "tool":
		summary.ToolCount = total
	case "skill":
		summary.SkillCount = total
	case "mcp":
		summary.MCPCount = total
	}
	return SessionTimeline{
		SessionID: sess.ID,
		Items:     items,
		Summary:   summary,
	}, nil
}

func sessionTimelineSummary(sess Session, pageItems []SessionTimelineItem) SessionTimelineSummary {
	summary := SessionTimelineSummary{
		MessageCount: sess.MessageCount,
		ToolCount:    sess.ToolCallCount,
		SkillCount:   sess.SkillCallCount,
		MCPCount:     sess.MCPCallCount,
	}
	summary.Total = summary.MessageCount + summary.ToolCount + summary.SkillCount
	if summary.Total == 0 && len(pageItems) > 0 {
		for _, item := range pageItems {
			switch item.Kind {
			case "message":
				summary.MessageCount++
			case "tool":
				summary.ToolCount++
			case "skill":
				summary.SkillCount++
			case "mcp":
				summary.MCPCount++
			}
		}
		summary.Total = len(pageItems)
	}
	return summary
}

func validationErr(msg string) error {
	return apierror.BadRequest("SESSION", msg)
}
