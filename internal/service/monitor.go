package service

import (
	"context"
	"encoding/json"
	"time"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ProcessLogEnabledProvider abstracts the process-log-on/off flag so that
// MonitorService does not depend on *conf.Server directly.
type ProcessLogEnabledProvider interface {
	ProcessLogEnabled() bool
}

type MonitorService struct {
	v1.UnimplementedMonitorServiceServer

	uc                 *biz.MonitorUsecase
	processLogEnabled  ProcessLogEnabledProvider
	diag               *biz.DiagBundleGenerator
	selfHeal           *biz.SelfHealUsecase
	selfHealObserver   *biz.SelfHealObserver
	selfCheckScheduler *monitor.SelfCheckScheduler
	selfCheckRepo      monitor.SelfCheckReportRepo
	lg                 loggateway.Logger

	flowLogSvc  *FlowLogService
	codeExecSvc *CodeExecutorService
}

func NewMonitorService(uc *biz.MonitorUsecase, processLogEnabled ProcessLogEnabledProvider, flowLogSvc *FlowLogService, codeExecSvc *CodeExecutorService, diag *biz.DiagBundleGenerator, selfHeal *biz.SelfHealUsecase, selfHealObserver *biz.SelfHealObserver, selfCheckScheduler *monitor.SelfCheckScheduler, selfCheckRepo monitor.SelfCheckReportRepo, lg loggateway.Logger) *MonitorService {
	return &MonitorService{uc: uc, processLogEnabled: processLogEnabled, flowLogSvc: flowLogSvc, codeExecSvc: codeExecSvc, diag: diag, selfHeal: selfHeal, selfHealObserver: selfHealObserver, selfCheckScheduler: selfCheckScheduler, selfCheckRepo: selfCheckRepo, lg: lg}
}

func bizAuditToProto(a biz.AuditLog) *v1.AuditLog {
	return &v1.AuditLog{
		Id:           a.ID,
		Action:       a.Action,
		Resource:     a.Resource,
		ResourceId:   a.ResourceID,
		RequestId:    a.RequestID,
		Detail:       a.Detail,
		CreatedAt:    a.CreatedAt,
		Actor:        a.Actor,
		Ip:           a.IP,
		UserAgent:    a.UserAgent,
		Severity:     a.Severity,
		MetadataJson: a.MetadataJSON,
	}
}

func bizMonitorRowToProto(row biz.MonitorPlatformRow, lg loggateway.Logger) *v1.MonitorPlatformRow {
	return &v1.MonitorPlatformRow{
		Id:           row.ID,
		Resource:     row.Resource,
		Key:          row.Key,
		Name:         row.Name,
		Description:  row.Description,
		Status:       row.Status,
		Enabled:      row.Enabled,
		SortOrder:    int32(row.SortOrder),
		ParentId:     row.ParentID,
		Level:        row.Level,
		AgentId:      row.AgentID,
		Provider:     row.Provider,
		Model:        row.Model,
		ConfigJson:   monitor.SanitizeJSONString(row.ConfigJSON, lg),
		MetadataJson: monitor.SanitizeJSONString(row.MetadataJSON, lg),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		DeletedAt:    row.DeletedAt,
		AgentName:    row.AgentName,
		TeamName:     row.TeamName,
		SessionId:    row.SessionID,
		RunId:        row.RunID,
	}
}

func notFoundMonitor(err error) error {
	if apierror.IsCode(err, apierror.CodeNotFound) {
		return apierror.NotFound(apierror.DomainMonitor, "monitor resource not found")
	}
	return err
}

// wrapInternalError preserves the error chain: if err is already an *apierror.Error
// it is returned directly; otherwise it is wrapped as an INTERNAL error.
func wrapInternalError(err error) error {
	if ae, ok := apierror.From(err); ok {
		return ae
	}
	return apierror.Internal(apierror.DomainMonitor, "monitor internal error")
}

func (s *MonitorService) ListAuditLogs(ctx context.Context, in *v1.ListAuditLogsRequest) (*v1.ListAuditLogsResponse, error) {
	result, err := s.uc.ListAuditLogs(ctx, biz.AuditQuery{
		Limit:         in.GetLimit(),
		Offset:        in.GetOffset(),
		Action:        in.GetAction(),
		Resource:      in.GetResource(),
		Actor:         in.GetActor(),
		Keyword:       in.GetKeyword(),
		ExcludeSystem: in.GetExcludeSystem(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.AuditLog, 0, len(result.Items))
	for _, a := range result.Items {
		out = append(out, bizAuditToProto(a))
	}
	return &v1.ListAuditLogsResponse{Items: out, Total: result.Total}, nil
}

func (s *MonitorService) DeleteAuditLogs(ctx context.Context, in *v1.DeleteAuditLogsRequest) (*v1.DeleteAuditLogsResponse, error) {
	if in.GetConfirm() != "CONFIRM" {
		return nil, apierror.BadRequest(apierror.DomainMonitor, "confirm must be \"CONFIRM\" to delete all audit logs")
	}
	deleted, err := s.uc.DeleteAuditLogs(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.DeleteAuditLogsResponse{Deleted: int32(deleted)}, nil
}

func (s *MonitorService) ListMonitorEvents(ctx context.Context, in *v1.ListMonitorEventsRequest) (*v1.ListMonitorEventsResponse, error) {
	result, err := s.uc.ListMonitorEvents(ctx, biz.MonitorEventsQuery{
		Limit:                 in.GetLimit(),
		Offset:                in.GetOffset(),
		EventType:             in.GetEventType(),
		AgentID:               in.GetAgentId(),
		Status:                in.GetStatus(),
		EventTypes:            in.GetEventTypes(),
		ExcludeEventTypes:     in.GetExcludeEventTypes(),
		HideLinkedCompletions: in.GetHideLinkedCompletions(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MonitorPlatformRow, 0, len(result.Items))
	for _, row := range result.Items {
		out = append(out, bizMonitorRowToProto(row, s.lg))
	}
	return &v1.ListMonitorEventsResponse{Items: out, Total: result.Total}, nil
}

func (s *MonitorService) GetMonitorEvent(ctx context.Context, in *v1.GetMonitorEventRequest) (*v1.MonitorPlatformRow, error) {
	row, err := s.uc.GetMonitorEvent(ctx, in.GetId())
	if err != nil {
		return nil, notFoundMonitor(err)
	}
	return bizMonitorRowToProto(row, s.lg), nil
}

func (s *MonitorService) GetMonitorTrace(ctx context.Context, in *v1.GetMonitorTraceRequest) (*v1.MonitorTraceDetail, error) {
	row, err := s.uc.GetMonitorTrace(ctx, in.GetId())
	if err != nil {
		return nil, notFoundMonitor(err)
	}
	cfg := monitor.ParseJSONMap(row.ConfigJSON, s.lg)
	spans := monitor.TraceSpansRaw(cfg)
	// Prefer persisted spans (monitor_trace_spans) over legacy config_json spans.
	if dbSpans, sErr := s.uc.ListTraceSpans(ctx, row.ID); sErr != nil {
		s.lg.Warn("查询 trace spans 失败，回退 config_json",
			loggateway.StepID("monitor.trace.spans_query_fail"), loggateway.Str("trace_id", row.ID), loggateway.Err(sErr))
	} else if len(dbSpans) > 0 {
		spans = traceSpansForWire(dbSpans, s.lg)
	}
	spansJSON, mErr := json.Marshal(spans)
	if mErr != nil {
		s.lg.Warn("spans 序列化失败", loggateway.StepID("monitor.trace.spans_marshal"), loggateway.Err(mErr))
		spansJSON = []byte("[]")
	}
	cfgSanitized := monitor.SanitizeJSONString(row.ConfigJSON, s.lg)
	metaSanitized := monitor.SanitizeJSONString(row.MetadataJSON, s.lg)
	tr := bizMonitorRowToProto(row, s.lg)
	return &v1.MonitorTraceDetail{
		Trace:        tr,
		ConfigJson:   cfgSanitized,
		MetadataJson: metaSanitized,
		SpansJson:    string(spansJSON),
	}, nil
}

// traceSpansForWire flattens persisted spans into the wire shape consumed by
// the waterfall/tree tabs. start_ms is normalized relative to the earliest
// span so offset percentages render meaningfully; duration_ms falls back to
// "still open" (0) when the span never closed.
func traceSpansForWire(spans []biz.MonitorTraceSpan, lg loggateway.Logger) []any {
	var minStart int64
	for _, sp := range spans {
		if sp.StartedAt > 0 && (minStart == 0 || sp.StartedAt < minStart) {
			minStart = sp.StartedAt
		}
	}
	out := make([]any, 0, len(spans))
	for _, sp := range spans {
		var durationMs int64
		if sp.EndedAt > sp.StartedAt {
			durationMs = sp.EndedAt - sp.StartedAt
		}
		entry := map[string]any{
			"id":          sp.SpanID,
			"parent_id":   sp.ParentSpanID,
			"kind":        sp.Kind,
			"name":        sp.Name,
			"status":      sp.Status,
			"start_ms":    sp.StartedAt - minStart,
			"duration_ms": durationMs,
		}
		if attrs := monitor.ParseJSONMap(sp.AttributesJSON, lg); len(attrs) > 0 {
			entry["attributes"] = attrs
		}
		if errObj := monitor.ParseJSONMap(sp.ErrorJSON, lg); len(errObj) > 0 {
			entry["error"] = errObj["message"]
		}
		out = append(out, entry)
	}
	return out
}

func toProtoAlertRule(r biz.MonitorAlertRule) *v1.MonitorAlertRule {
	return &v1.MonitorAlertRule{
		Id:               r.ID,
		Name:             r.Name,
		MetricKey:        r.MetricKey,
		Threshold:        r.Threshold,
		WindowMinutes:    int32(r.WindowMinutes),
		Enabled:          r.Enabled,
		Severity:         r.Severity,
		NotifyWebhookUrl: r.NotifyWebhookURL,
		NotifyChannelId:  r.NotifyChannelID,
		CooldownMinutes:  int32(r.CooldownMinutes),
	}
}

func fromProtoAlertRule(r *v1.MonitorAlertRule) biz.MonitorAlertRule {
	if r == nil {
		return biz.MonitorAlertRule{}
	}
	return biz.MonitorAlertRule{
		ID:               r.GetId(),
		Name:             r.GetName(),
		MetricKey:        r.GetMetricKey(),
		Threshold:        r.GetThreshold(),
		WindowMinutes:    int(r.GetWindowMinutes()),
		Enabled:          r.GetEnabled(),
		Severity:         r.GetSeverity(),
		NotifyWebhookURL: r.GetNotifyWebhookUrl(),
		NotifyChannelID:  r.GetNotifyChannelId(),
		CooldownMinutes:  int(r.GetCooldownMinutes()),
	}
}

func (s *MonitorService) ListMonitorAlertRules(ctx context.Context, _ *v1.GetMonitorLogsRequest) (*v1.ListMonitorAlertRulesResponse, error) {
	rules, err := s.uc.ListAlertRulesWithDefaults(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListMonitorAlertRulesResponse{Items: make([]*v1.MonitorAlertRule, 0, len(rules))}
	for i := range rules {
		resp.Items = append(resp.Items, toProtoAlertRule(rules[i]))
	}
	return resp, nil
}

// ListAlertMetrics returns the alert metric directory: every registered
// metric with human-readable metadata and its current value, so the Alerts
// page can explain what each metric means instead of showing raw keys.
func (s *MonitorService) ListAlertMetrics(ctx context.Context, _ *v1.GetMonitorLogsRequest) (*v1.ListAlertMetricsResponse, error) {
	entries := s.uc.ListAlertMetricCatalog(ctx)
	resp := &v1.ListAlertMetricsResponse{Items: make([]*v1.AlertMetricInfo, 0, len(entries))}
	for _, e := range entries {
		resp.Items = append(resp.Items, &v1.AlertMetricInfo{
			Key:                  e.Key,
			Name:                 e.Name,
			Description:          e.Description,
			Unit:                 e.Unit,
			DefaultWindowMinutes: e.DefaultWindowMinutes,
			SuggestedThreshold:   e.SuggestedThreshold,
			CurrentValue:         e.CurrentValue,
			EvaluatedAt:          e.EvaluatedAt.UTC().Format(time.RFC3339),
		})
	}
	return resp, nil
}

func (s *MonitorService) PutMonitorAlertRules(ctx context.Context, req *v1.PutMonitorAlertRulesRequest) (*v1.PutMonitorAlertRulesResponse, error) {
	rules := make([]biz.MonitorAlertRule, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		rules = append(rules, fromProtoAlertRule(item))
	}
	if err := s.uc.ReplaceAlertRules(ctx, rules); err != nil {
		return nil, err
	}
	listed, err := s.ListMonitorAlertRules(ctx, &v1.GetMonitorLogsRequest{})
	if err != nil {
		return nil, err
	}
	return &v1.PutMonitorAlertRulesResponse{Items: listed.GetItems()}, nil
}

func (s *MonitorService) GetRunnerMetrics(ctx context.Context, req *v1.GetRunnerMetricsRequest) (*v1.RunnerMetricsSummary, error) {
	m, err := s.uc.GetRunnerMetrics(ctx, int(req.GetWindowMinutes()))
	if err != nil {
		return nil, err
	}
	return &v1.RunnerMetricsSummary{
		WindowMinutes: int32(m.WindowMinutes),
		TotalRuns:     m.TotalRuns,
		ErrorRuns:     m.ErrorRuns,
		ErrorRate:     m.ErrorRate,
		SuccessRate:   m.SuccessRate,
		AvgDurationMs: m.AvgDurationMs,
		P50DurationMs: m.P50DurationMs,
		P95DurationMs: m.P95DurationMs,
		P99DurationMs: m.P99DurationMs,
	}, nil
}

func (s *MonitorService) ListFlowLogs(ctx context.Context, in *v1.ListFlowLogsRequest) (*v1.ListFlowLogsResponse, error) {
	if s == nil || s.flowLogSvc == nil {
		return &v1.ListFlowLogsResponse{}, nil
	}
	return s.flowLogSvc.ListFlowLogs(ctx, in)
}

func (s *MonitorService) GetMonitorLogs(context.Context, *v1.GetMonitorLogsRequest) (*v1.GetMonitorLogsResponse, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := true
	if s.processLogEnabled != nil {
		enabled = s.processLogEnabled.ProcessLogEnabled()
	}
	msg := "Process logs disabled in server.monitor.process_log_enabled."
	if enabled {
		msg = "Process logs follow server.monitor.process_log_enabled; subscribe via WebSocket /v1/ws."
	}
	return &v1.GetMonitorLogsResponse{
		Items: []*v1.MonitorLogLine{{
			Id:        "ws-hint",
			Time:      now,
			Level:     "INFO",
			Message:   "Live log lines are pushed via WebSocket; flow_log always on, process log gated by config.",
			Source:    "monitor",
			CreatedAt: now,
		}},
		Enabled: enabled,
		Message: msg,
	}, nil
}

func (s *MonitorService) GetCodeExecutorCapabilities(ctx context.Context, in *v1.GetMonitorLogsRequest) (*v1.GetCodeExecutorCapabilitiesResponse, error) {
	if s == nil || s.codeExecSvc == nil {
		return &v1.GetCodeExecutorCapabilitiesResponse{}, nil
	}
	return s.codeExecSvc.GetCodeExecutorCapabilities(ctx, in)
}

func (s *MonitorService) GenerateDiagnosticBundle(ctx context.Context, in *v1.GenerateDiagnosticBundleRequest) (*v1.GenerateDiagnosticBundleResponse, error) {
	if s == nil || s.diag == nil {
		return nil, apierror.Unavailable(apierror.DomainMonitor, "diagnostic bundle generator not available")
	}
	bundle, err := s.diag.Generate(ctx,
		in.GetTraceId(), in.GetSessionId(), in.GetRunId(), in.GetStepId(),
		in.GetTriggerType(), in.GetContextMinutes(),
	)
	if err != nil {
		return nil, wrapInternalError(err)
	}
	manifestJSON, mErr := json.Marshal(bundle.Manifest)
	if mErr != nil {
		manifestJSON = []byte("{}")
	}
	if len(bundle.RootCauses) > 0 {
		var m map[string]any
		if err := json.Unmarshal(manifestJSON, &m); err == nil {
			m["root_causes"] = bundle.RootCauses
			if mj, mErr := json.Marshal(m); mErr == nil {
				manifestJSON = mj
			}
		}
	}
	return &v1.GenerateDiagnosticBundleResponse{
		BundleId:     bundle.BundleID,
		ManifestJson: string(manifestJSON),
		TotalEntries: int32(bundle.Total),
		FlowJsonl:    bundle.FlowJSONL,
		TraceJson:    bundle.TraceJSON,
		UsageJson:    bundle.UsageJSON,
		AlertsJsonl:  bundle.AlertsJSONL,
	}, nil
}

func (s *MonitorService) DiagnoseAndHeal(ctx context.Context, in *v1.DiagnoseAndHealRequest) (*v1.DiagnoseAndHealResponse, error) {
	result, err := heal.DiagnoseAndHeal(ctx, s.selfHealObserver, s.selfHeal,
		in.GetTraceId(), in.GetSessionId(), in.GetRunId(), in.GetStepId(),
		in.GetTriggerType(), in.GetContextMinutes(),
	)
	if err != nil {
		return nil, err
	}

	resp := &v1.DiagnoseAndHealResponse{
		HealId:               result.HealID,
		RuleId:               result.RuleID,
		Status:               result.Status,
		Reason:               result.Reason,
		Confidence:           result.Confidence,
		FixActionType:        result.FixAction.Type,
		FixActionMaxAttempts: int32(result.FixAction.MaxAttempts),
		FixActionParamsJson:  heal.DiagnoseAndHealFixParamsJSON(result),
		RuntimeAutoHealed:    result.RuntimeAutoHealed,
		RuntimeHealAttempts:  int32(result.RuntimeHealAttempts),
		CreatedAt:            result.CreatedAt,
	}

	if rc := result.RootCauseCondition; rc != nil {
		switch {
		case rc.AutoHealed != nil:
			resp.RootCauseCondition = &v1.RootCauseCondition{
				Condition: &v1.RootCauseCondition_AutoHealed{
					AutoHealed: &v1.AutoHealedCondition{
						AutoHealed:   rc.AutoHealed.AutoHealed,
						HealStrategy: rc.AutoHealed.HealStrategy,
					},
				},
			}
		case rc.HealAttempts != nil:
			resp.RootCauseCondition = &v1.RootCauseCondition{
				Condition: &v1.RootCauseCondition_HealAttempts{
					HealAttempts: &v1.HealAttemptsCondition{
						Attempts:     int32(rc.HealAttempts.Attempts),
						MaxAttempts:  int32(rc.HealAttempts.MaxAttempts),
						LastStrategy: rc.HealAttempts.LastStrategy,
					},
				},
			}
		case rc.SelfCheck != nil:
			resp.RootCauseCondition = &v1.RootCauseCondition{
				Condition: &v1.RootCauseCondition_SelfCheckStatus{
					SelfCheckStatus: &v1.SelfCheckStatusCondition{
						CheckName: rc.SelfCheck.CheckName,
						Status:    rc.SelfCheck.Status,
						Message:   rc.SelfCheck.Message,
					},
				},
			}
		}
	}

	return resp, nil
}

func (s *MonitorService) TriggerSelfCheck(ctx context.Context, _ *v1.TriggerSelfCheckRequest) (*v1.TriggerSelfCheckResponse, error) {
	if s == nil || s.selfCheckScheduler == nil {
		return nil, apierror.Unavailable(apierror.DomainMonitor, "self-check scheduler not available")
	}
	report := s.selfCheckScheduler.RunOnce(ctx)
	if report == nil {
		return nil, apierror.Internal(apierror.DomainMonitor, "self-check returned no report")
	}
	return &v1.TriggerSelfCheckResponse{
		Report: bizSelfCheckReportToProto(report),
	}, nil
}

func (s *MonitorService) ListSelfCheckReports(ctx context.Context, in *v1.ListSelfCheckReportsRequest) (*v1.ListSelfCheckReportsResponse, error) {
	if s == nil || s.selfCheckRepo == nil {
		return &v1.ListSelfCheckReportsResponse{}, nil
	}
	reports, total, err := s.selfCheckRepo.ListSelfCheckReports(ctx, int(in.GetLimit()), int(in.GetOffset()))
	if err != nil {
		return nil, wrapInternalError(err)
	}
	items := make([]*v1.SelfCheckReportEntry, 0, len(reports))
	for _, r := range reports {
		items = append(items, bizSelfCheckReportToProto(&r))
	}
	return &v1.ListSelfCheckReportsResponse{
		Items: items,
		Total: int32(total),
	}, nil
}

func bizSelfCheckReportToProto(r *monitor.SelfCheckReport) *v1.SelfCheckReportEntry {
	if r == nil {
		return nil
	}
	results := make([]*v1.SelfCheckResultEntry, 0, len(r.CheckResults))
	for _, cr := range r.CheckResults {
		detailsJSON, dErr := json.Marshal(cr.Details)
		if dErr != nil {
			detailsJSON = []byte("{}")
		}
		results = append(results, &v1.SelfCheckResultEntry{
			CheckId:     cr.CheckID,
			Checker:     cr.Checker,
			Status:      string(cr.Status),
			Message:     cr.Message,
			DetailsJson: string(detailsJSON),
			CheckedAt:   cr.CheckedAt.Format(time.RFC3339Nano),
		})
	}
	repairs := make([]*v1.RepairActionEntry, 0, len(r.RepairActions))
	for _, ra := range r.RepairActions {
		repairs = append(repairs, &v1.RepairActionEntry{
			Success: ra.Success,
			Action:  ra.Action,
			Message: ra.Message,
		})
	}
	return &v1.SelfCheckReportEntry{
		Id:            r.ID,
		CheckResults:  results,
		OverallStatus: string(r.OverallStatus),
		RepairActions: repairs,
		StartedAt:     r.StartedAt.Format(time.RFC3339Nano),
		FinishedAt:    r.FinishedAt.Format(time.RFC3339Nano),
		DurationMs:    r.DurationMs,
	}
}

func (s *MonitorService) GetHealStats(ctx context.Context, _ *v1.HealStatsRequest) (*v1.HealStatsResponse, error) {
	if s == nil || s.selfHealObserver == nil {
		return nil, apierror.Unavailable(apierror.DomainMonitor, "self-heal observer not available")
	}
	stats, err := s.selfHealObserver.GetHealStats(ctx)
	if err != nil {
		return nil, wrapInternalError(err)
	}
	resp := &v1.HealStatsResponse{
		TotalHeals:  int32(stats.TotalHeals),
		SuccessRate: stats.SuccessRate,
	}
	for _, r := range stats.TopFailRules {
		resp.TopFailRules = append(resp.TopFailRules, &v1.RuleFailCount{
			RuleId: r.RuleID,
			Count:  int32(r.Count),
		})
	}
	return resp, nil
}

func (s *MonitorService) ListHealRecords(ctx context.Context, in *v1.ListHealRecordsRequest) (*v1.ListHealRecordsResponse, error) {
	if s == nil || s.selfHealObserver == nil {
		return nil, apierror.Unavailable(apierror.DomainMonitor, "self-heal observer not available")
	}
	result, err := s.selfHealObserver.ListHealRecords(ctx, biz.HealRecordQuery{
		Limit:     int(in.GetLimit()),
		Offset:    int(in.GetOffset()),
		RuleID:    in.GetRuleId(),
		Status:    in.GetStatus(),
		SessionID: in.GetSessionId(),
	})
	if err != nil {
		return nil, wrapInternalError(err)
	}
	items := make([]*v1.HealRecordEntry, 0, len(result.Items))
	for _, r := range result.Items {
		items = append(items, &v1.HealRecordEntry{
			Id:                  r.ID,
			RuleId:              r.RuleID,
			TriggerType:         r.TriggerType,
			TraceId:             r.TraceID,
			SessionId:           r.SessionID,
			StepId:              r.StepID,
			FixActionType:       r.FixAction.Type,
			Confidence:          r.Confidence,
			Status:              r.Status,
			RuntimeAutoHealed:   r.RuntimeAutoHealed,
			RuntimeHealAttempts: int32(r.RuntimeHealAttempts),
			Reason:              r.Reason,
			CreatedAt:           r.CreatedAt,
		})
	}
	return &v1.ListHealRecordsResponse{Items: items, Total: int32(result.Total)}, nil
}
