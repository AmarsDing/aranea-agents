package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type MonitorService struct {
	v1.UnimplementedMonitorServiceServer

	uc     *biz.MonitorUsecase
	server *conf.Server
	diag   *biz.DiagBundleGenerator
	selfHeal *biz.SelfHealUsecase
	selfHealObserver *biz.SelfHealObserver
	selfCheckScheduler *monitor.SelfCheckScheduler
	selfCheckRepo      monitor.SelfCheckReportRepo
	lg     loggateway.Logger

	flowLogSvc  *FlowLogService
	codeExecSvc *CodeExecutorService
}

func NewMonitorService(uc *biz.MonitorUsecase, server *conf.Server, flowLogSvc *FlowLogService, codeExecSvc *CodeExecutorService, diag *biz.DiagBundleGenerator, selfHeal *biz.SelfHealUsecase, selfHealObserver *biz.SelfHealObserver, selfCheckScheduler *monitor.SelfCheckScheduler, selfCheckRepo monitor.SelfCheckReportRepo, lg loggateway.Logger) *MonitorService {
	return &MonitorService{uc: uc, server: server, flowLogSvc: flowLogSvc, codeExecSvc: codeExecSvc, diag: diag, selfHeal: selfHeal, selfHealObserver: selfHealObserver, selfCheckScheduler: selfCheckScheduler, selfCheckRepo: selfCheckRepo, lg: lg}
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
	}
}

func notFoundMonitor(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apierror.NotFound("MONITOR_NOT_FOUND", err.Error())
	}
	return err
}

// wrapInternalError preserves the error chain: if err is already an *apierror.Error
// it is returned directly; otherwise it is wrapped as an INTERNAL error.
func wrapInternalError(err error) error {
	if ae, ok := apierror.From(err); ok {
		return ae
	}
	return apierror.Internal("INTERNAL", err.Error())
}

func (s *MonitorService) ListAuditLogs(ctx context.Context, in *v1.ListAuditLogsRequest) (*v1.ListAuditLogsResponse, error) {
	result, err := s.uc.ListAuditLogs(ctx, biz.AuditQuery{
		Limit:    in.GetLimit(),
		Offset:   in.GetOffset(),
		Action:   in.GetAction(),
		Resource: in.GetResource(),
		Actor:    in.GetActor(),
		Keyword:  in.GetKeyword(),
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

func (s *MonitorService) ListMonitorEvents(ctx context.Context, in *v1.ListMonitorEventsRequest) (*v1.ListMonitorEventsResponse, error) {
	result, err := s.uc.ListMonitorEvents(ctx, biz.MonitorEventsQuery{
		Limit:     in.GetLimit(),
		Offset:    in.GetOffset(),
		EventType: in.GetEventType(),
		AgentID:   in.GetAgentId(),
		Status:    in.GetStatus(),
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

func (s *MonitorService) ListMonitorTraces(ctx context.Context, in *v1.ListMonitorTracesRequest) (*v1.ListMonitorTracesResponse, error) {
	result, err := s.uc.ListMonitorTraces(ctx, biz.MonitorTracesQuery{
		Limit:    in.GetLimit(),
		Offset:   in.GetOffset(),
		AgentID:  in.GetAgentId(),
		Provider: in.GetProvider(),
		Model:    in.GetModel(),
		Status:   in.GetStatus(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MonitorPlatformRow, 0, len(result.Items))
	for _, row := range result.Items {
		out = append(out, bizMonitorRowToProto(row, s.lg))
	}
	return &v1.ListMonitorTracesResponse{Items: out, Total: result.Total}, nil
}

func (s *MonitorService) GetMonitorTrace(ctx context.Context, in *v1.GetMonitorTraceRequest) (*v1.MonitorTraceDetail, error) {
	row, err := s.uc.GetMonitorTrace(ctx, in.GetId())
	if err != nil {
		return nil, notFoundMonitor(err)
	}
	cfg := monitor.ParseJSONMap(row.ConfigJSON, s.lg)
	spans := monitor.TraceSpansRaw(cfg)
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
	if s.server != nil {
		enabled = s.server.ProcessLogEnabled()
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
		return nil, apierror.Unavailable("SERVICE_UNAVAILABLE", "diagnostic bundle generator not available")
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
	result, err := s.uc.DiagnoseAndHeal(ctx, s.selfHealObserver, s.selfHeal,
		in.GetTraceId(), in.GetSessionId(), in.GetRunId(), in.GetStepId(),
		in.GetTriggerType(), in.GetContextMinutes(),
	)
	if err != nil {
		return nil, err
	}

	resp := &v1.DiagnoseAndHealResponse{
		HealId:              result.HealID,
		RuleId:              result.RuleID,
		Status:              result.Status,
		Reason:              result.Reason,
		Confidence:          result.Confidence,
		FixActionType:       result.FixAction.Type,
		FixActionMaxAttempts: int32(result.FixAction.MaxAttempts),
		FixActionParamsJson: monitor.DiagnoseAndHealFixParamsJSON(result),
		RuntimeAutoHealed:   result.RuntimeAutoHealed,
		RuntimeHealAttempts: int32(result.RuntimeHealAttempts),
		CreatedAt:           result.CreatedAt,
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
		return nil, apierror.Unavailable("SERVICE_UNAVAILABLE", "self-check scheduler not available")
	}
	report := s.selfCheckScheduler.RunOnce(ctx)
	if report == nil {
		return nil, apierror.Internal("INTERNAL", "self-check returned no report")
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
			CheckId:    cr.CheckID,
			Checker:    cr.Checker,
			Status:     string(cr.Status),
			Message:    cr.Message,
			DetailsJson: string(detailsJSON),
			CheckedAt:  cr.CheckedAt.Format(time.RFC3339Nano),
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
		return nil, apierror.Unavailable("SERVICE_UNAVAILABLE", "self-heal observer not available")
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
		return nil, apierror.Unavailable("SERVICE_UNAVAILABLE", "self-heal observer not available")
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
