package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizflowlog "aranea-agents/internal/biz/flowlog"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/flowlogevent"
	"aranea-agents/pkg/apierror"
)

type flowLogRepo struct {
	data *Data
}

var _ bizflowlog.Repo = (*flowLogRepo)(nil)

func NewFlowLogRepo(d *Data) biz.FlowLogRepo {
	return &flowLogRepo{data: d}
}

func (r *flowLogRepo) Insert(ctx context.Context, rec biz.FlowLogRecord) error {
	if r == nil || r.data == nil {
		return apierror.Internal("FLOW_LOG", "database not configured")
	}
	_, err := r.data.RW().Write(ctx).FlowLogEvent.Create().
		SetID(rec.ID).
		SetTraceID(rec.TraceID).
		SetSessionID(rec.SessionID).
		SetRunID(rec.RunID).
		SetTeamID(rec.TeamID).
		SetDomain(rec.Domain).
		SetAgentKey(rec.AgentKey).
		SetStepID(rec.StepID).
		SetFlowPhase(rec.FlowPhase).
		SetSeverity(rec.Severity).
		SetTitle(rec.Title).
		SetMessage(rec.Message).
		SetPayloadJSON(rec.PayloadJSON).
		SetCreatedAt(rec.CreatedAt).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil
		}
		return entErrToBizErr(err, "FLOW_LOG")
	}
	return nil
}

func (r *flowLogRepo) List(ctx context.Context, q biz.FlowLogQuery) (biz.FlowLogListResult, error) {
	if r == nil || r.data == nil {
		return biz.FlowLogListResult{}, apierror.Internal("FLOW_LOG", "database not configured")
	}
	client := r.data.RW().Read(ctx)
	query := client.FlowLogEvent.Query()
	if tid := strings.TrimSpace(q.TraceID); tid != "" {
		query = query.Where(flowlogevent.TraceIDEQ(tid))
	}
	if sid := strings.TrimSpace(q.SessionID); sid != "" {
		query = query.Where(flowlogevent.SessionIDEQ(sid))
	}
	if rid := strings.TrimSpace(q.RunID); rid != "" {
		query = query.Where(flowlogevent.RunIDEQ(rid))
	}
	if sev := strings.TrimSpace(q.Severity); sev != "" {
		query = query.Where(flowlogevent.SeverityEQ(sev))
	}
	if dom := strings.TrimSpace(q.Domain); dom != "" {
		query = query.Where(flowlogevent.DomainEQ(dom))
	}
	if !q.Since.IsZero() {
		query = query.Where(flowlogevent.CreatedAtGTE(q.Since))
	}
	if !q.Until.IsZero() {
		query = query.Where(flowlogevent.CreatedAtLTE(q.Until))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return biz.FlowLogListResult{}, entErrToBizErr(err, "FLOW_LOG")
	}
	rows, err := query.
		Order(ent.Asc(flowlogevent.FieldCreatedAt)).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.FlowLogListResult{}, entErrToBizErr(err, "FLOW_LOG")
	}
	items := make([]biz.FlowLogRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, biz.FlowLogRecord{
			ID:          row.ID,
			TraceID:     row.TraceID,
			SessionID:   row.SessionID,
			RunID:       row.RunID,
			TeamID:      row.TeamID,
			Domain:      row.Domain,
			AgentKey:    row.AgentKey,
			StepID:      row.StepID,
			FlowPhase:   row.FlowPhase,
			Severity:    row.Severity,
			Title:       row.Title,
			Message:     row.Message,
			PayloadJSON: row.PayloadJSON,
			CreatedAt:   row.CreatedAt,
		})
	}
	return biz.FlowLogListResult{Items: items, Total: total}, nil
}

func (r *flowLogRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if r == nil || r.data == nil {
		return 0, apierror.Internal("FLOW_LOG", "database not configured")
	}
	n, err := r.data.RW().Write(ctx).FlowLogEvent.Delete().
		Where(flowlogevent.CreatedAtLT(cutoff)).
		Exec(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "FLOW_LOG")
	}
	return int64(n), nil
}
