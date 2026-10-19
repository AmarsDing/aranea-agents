package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/platformskill"
	"aranea-agents/internal/data/ent/predicate"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

func runPredicates(q biz.SkillRunQuery) []predicate.SkillInvocation {
	ps := []predicate.SkillInvocation{}
	if q.SkillID != "" {
		ps = append(ps, skillinvocation.SkillIDEQ(q.SkillID))
	}
	if q.AgentID != "" {
		ps = append(ps, skillinvocation.AgentIDEQ(q.AgentID))
	}
	if q.SessionID != "" {
		ps = append(ps, skillinvocation.SessionIDEQ(q.SessionID))
	}
	if q.Status != "" {
		ps = append(ps, skillinvocation.StatusEQ(q.Status))
	}
	ps = append(ps, invocationTimeRange(q.From, q.To)...)
	return ps
}

func (r *skillRepo) SearchSkillInvocations(ctx context.Context, query biz.SkillRunQuery) (biz.SkillRunResult, error) {
	c := r.data.RW().Read(ctx)
	preds := runPredicates(query)
	base := c.SkillInvocation.Query()
	if len(preds) > 0 {
		base = base.Where(preds...)
	}
	total, err := base.Clone().Count(ctx)
	if err != nil {
		return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}
	rows, err := base.Clone().
		Order(skillinvocation.ByCreatedAt(entsql.OrderDesc())).
		Limit(query.Limit).
		Offset(query.Offset).
		All(ctx)
	if err != nil {
		return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
	}

	skillIDs := map[string]struct{}{}
	agentIDs := map[string]struct{}{}
	for _, row := range rows {
		if row.SkillID != "" {
			skillIDs[row.SkillID] = struct{}{}
		}
		if row.AgentID != "" {
			agentIDs[row.AgentID] = struct{}{}
		}
	}
	sidList := make([]string, 0, len(skillIDs))
	for id := range skillIDs {
		sidList = append(sidList, id)
	}
	aidList := make([]string, 0, len(agentIDs))
	for id := range agentIDs {
		aidList = append(aidList, id)
	}
	skillNames := map[string]string{}
	if len(sidList) > 0 {
		skills, err := c.PlatformSkill.Query().Where(platformskill.IDIn(sidList...)).All(ctx)
		if err != nil {
			return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		for _, s := range skills {
			skillNames[s.ID] = s.Name
		}
	}
	agentNames := map[string]string{}
	if len(aidList) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(aidList...)).All(ctx)
		if err != nil {
			return biz.SkillRunResult{}, entErrToBizErr(err, apierror.DomainSkill)
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}

	items := make([]biz.SkillInvocation, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.SkillInvocation{
			ID:               row.ID,
			SkillID:          row.SkillID,
			SkillName:        skillNames[row.SkillID],
			SkillVersion:     row.SkillVersion,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			UserID:           row.UserID,
			SessionID:        row.SessionID,
			Status:           row.Status,
			DurationMS:       row.DurationMs,
			StartedAt:        coalesceTime(row.StartedAt, row.CreatedAt),
			EndedAt:          row.EndedAt,
			InputPreview:     row.InputPreview,
			InputHash:        row.InputHash,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
			Source:           row.Source,
			ActivationID:     row.ActivationID,
			MessageID:        row.MessageID,
			Permissions:      biz.SkillInvocationPermissions{},
		})
	}
	return biz.SkillRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *skillRepo) RecordSkillInvocation(ctx context.Context, in biz.SkillInvocationWrite) error {
	id := fmt.Sprintf("skillinv_%d", time.Now().UTC().UnixNano())
	now := nowRFC3339()
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "runtime"
	}
	started := strings.TrimSpace(in.StartedAt)
	if started == "" {
		started = now
	}
	ended := strings.TrimSpace(in.EndedAt)
	if ended == "" {
		ended = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "success"
	}
	builder := r.data.RW().Write(ctx).SkillInvocation.Create().
		SetID(id).
		SetSkillID(strings.TrimSpace(in.SkillID)).
		SetAgentID(strings.TrimSpace(in.AgentID)).
		SetStatus(status).
		SetInputJSON("{}").
		SetOutputJSON("{}").
		SetSkillVersion(strings.TrimSpace(in.SkillVersion)).
		SetUserID(strings.TrimSpace(in.UserID)).
		SetSessionID(strings.TrimSpace(in.SessionID)).
		SetDurationMs(in.DurationMS).
		SetStartedAt(started).
		SetEndedAt(ended).
		SetInputPreview(strings.TrimSpace(in.InputPreview)).
		SetInputHash(strings.TrimSpace(in.InputHash)).
		SetOutputPreview(strings.TrimSpace(in.OutputPreview)).
		SetErrorCode(strings.TrimSpace(in.ErrorCode)).
		SetErrorMessage(strings.TrimSpace(in.ErrorMessage)).
		SetSource(source).
		SetActivationID(strings.TrimSpace(in.ActivationID)).
		SetMessageID(strings.TrimSpace(in.MessageID)).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if in.SelectionReason != nil {
		builder = builder.SetSelectionReason(in.SelectionReason)
	}
	if strings.TrimSpace(in.Outcome) != "" {
		builder = builder.SetOutcome(strings.TrimSpace(in.Outcome))
	}
	if in.TokenUsage != nil {
		builder = builder.SetTokenUsage(in.TokenUsage)
	}
	if len(in.RoutedSlugs) > 0 {
		builder = builder.SetRoutedSlugs(in.RoutedSlugs)
	}
	if strings.TrimSpace(in.LoadedSlug) != "" {
		builder = builder.SetLoadedSlug(strings.TrimSpace(in.LoadedSlug))
	}
	_, err := builder.Save(ctx)
	return entErrToBizErr(err, apierror.DomainSkill)
}
