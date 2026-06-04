package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type MonitorService struct {
	v1.UnimplementedMonitorServiceServer

	uc     *biz.MonitorUsecase
	server *conf.Server
	diag   *biz.DiagBundleGenerator
	selfHeal *biz.SelfHealUsecase
	lg     loggateway.Logger

	flowLogSvc  *FlowLogService
	codeExecSvc *CodeExecutorService
}

func NewMonitorService(uc *biz.MonitorUsecase, server *conf.Server, flowLogSvc *FlowLogService, codeExecSvc *CodeExecutorService, diag *biz.DiagBundleGenerator, selfHeal *biz.SelfHealUsecase, lg loggateway.Logger) *MonitorService {
	return &MonitorService{uc: uc, server: server, flowLogSvc: flowLogSvc, codeExecSvc: codeExecSvc, diag: diag, selfHeal: selfHeal, lg: lg}
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
		ConfigJson:   sanitizeJSONString(row.ConfigJSON, lg),
		MetadataJson: sanitizeJSONString(row.MetadataJSON, lg),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
		DeletedAt:    row.DeletedAt,
	}
}

func notFoundMonitor(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return kerrors.NotFound("MONITOR_NOT_FOUND", err.Error())
	}
	return err
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
	cfg := parseJSONMap(row.ConfigJSON, s.lg)
	spans := traceSpansRaw(cfg)
	spansJSON, _ := json.Marshal(spans)
	cfgSanitized := sanitizeJSONString(row.ConfigJSON, s.lg)
	metaSanitized := sanitizeJSONString(row.MetadataJSON, s.lg)
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
	rules, err := s.uc.ListAlertRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		defaults := defaultAlertRules()
		if err := s.uc.ReplaceAlertRules(ctx, defaults); err != nil {
			s.lg.Warn("ListMonitorAlertRules: ReplaceAlertRules failed",
				loggateway.StepID("monitor.alert_rules_replace_fail"),
				loggateway.Err(err),
			)
		}
		rules = defaults
	}
	resp := &v1.ListMonitorAlertRulesResponse{Items: make([]*v1.MonitorAlertRule, 0, len(rules))}
	for i := range rules {
		resp.Items = append(resp.Items, toProtoAlertRule(rules[i]))
	}
	return resp, nil
}

func defaultAlertRules() []biz.MonitorAlertRule {
	return []biz.MonitorAlertRule{{
		ID: "default-runner-errors", Name: "Runner error rate",
		MetricKey: "runner.error_rate", Threshold: 0.25, WindowMinutes: 60, Enabled: true, Severity: "warning",
	}}
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

func parseFlowLogTimeBounds(sinceRaw, untilRaw string) (since, until time.Time, err error) {
	if s := strings.TrimSpace(sinceRaw); s != "" {
		since, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			if since, err = time.Parse(time.RFC3339, s); err != nil {
				return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "invalid since: "+err.Error())
			}
		}
		since = since.UTC()
	}
	if u := strings.TrimSpace(untilRaw); u != "" {
		until, err = time.Parse(time.RFC3339Nano, u)
		if err != nil {
			if until, err = time.Parse(time.RFC3339, u); err != nil {
				return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "invalid until: "+err.Error())
			}
		}
		until = until.UTC()
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return time.Time{}, time.Time{}, kerrors.BadRequest("MONITOR", "until must be after since")
	}
	return since, until, nil
}

func sanitizeJSONString(raw string, lg loggateway.Logger) string {
	parsed := parseJSONMap(raw, lg)
	if len(parsed) == 0 {
		return raw
	}
	sanitized := sanitizeJSONValue(parsed)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return raw
	}
	return string(out)
}

func parseJSONMap(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		lg.Warn("json map unmarshal failed", loggateway.StepID("monitor.parse_json_map"), loggateway.Err(err))
		return map[string]any{}
	}
	return sanitizeJSONValue(parsed).(map[string]any)
}

func sanitizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveKey(key) {
				out[key] = "******"
				continue
			}
			out[key] = sanitizeJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, sanitizeJSONValue(child))
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "authorization", "cookie"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func traceSpansRaw(config map[string]any) []any {
	if spans, ok := config["spans"].([]any); ok {
		return spans
	}
	if trace, ok := config["trace"].(map[string]any); ok {
		if spans, ok := trace["spans"].([]any); ok {
			return spans
		}
	}
	return []any{}
}

func (s *MonitorService) GetCodeExecutorCapabilities(ctx context.Context, in *v1.GetMonitorLogsRequest) (*v1.GetCodeExecutorCapabilitiesResponse, error) {
	if s == nil || s.codeExecSvc == nil {
		return &v1.GetCodeExecutorCapabilitiesResponse{}, nil
	}
	return s.codeExecSvc.GetCodeExecutorCapabilities(ctx, in)
}

func (s *MonitorService) GenerateDiagnosticBundle(ctx context.Context, in *v1.GenerateDiagnosticBundleRequest) (*v1.GenerateDiagnosticBundleResponse, error) {
	if s == nil || s.diag == nil {
		return nil, kerrors.New(503, "SERVICE_UNAVAILABLE", "diagnostic bundle generator not available")
	}
	bundle, err := s.diag.Generate(ctx,
		in.GetTraceId(), in.GetSessionId(), in.GetRunId(), in.GetStepId(),
		in.GetTriggerType(), in.GetContextMinutes(),
	)
	if err != nil {
		return nil, kerrors.New(500, "INTERNAL", err.Error())
	}
	manifestJSON, _ := json.Marshal(bundle.Manifest)
	if len(bundle.RootCauses) > 0 {
		var m map[string]any
		if err := json.Unmarshal(manifestJSON, &m); err == nil {
			m["root_causes"] = bundle.RootCauses
			manifestJSON, _ = json.Marshal(m)
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
	if s == nil || s.selfHeal == nil {
		return nil, kerrors.New(503, "SERVICE_UNAVAILABLE", "self-heal service not available")
	}
	rec, err := s.selfHeal.DiagnoseAndHeal(ctx,
		in.GetTraceId(), in.GetSessionId(), in.GetRunId(), in.GetStepId(),
		in.GetTriggerType(), in.GetContextMinutes(),
	)
	if err != nil {
		return nil, kerrors.New(500, "INTERNAL", err.Error())
	}
	fixParamsJSON, _ := json.Marshal(rec.FixAction.Params)

	resp := &v1.DiagnoseAndHealResponse{
		HealId:              rec.ID,
		RuleId:              rec.RuleID,
		Status:              rec.Status,
		Reason:              rec.Reason,
		Confidence:          rec.Confidence,
		FixActionType:       rec.FixAction.Type,
		FixActionMaxAttempts: int32(rec.FixAction.MaxAttempts),
		FixActionParamsJson: string(fixParamsJSON),
		CreatedAt:           rec.CreatedAt,
	}

	// Populate RootCauseCondition based on heal result.
	switch rec.Status {
	case "applied":
		resp.RootCauseCondition = &v1.RootCauseCondition{
			Condition: &v1.RootCauseCondition_AutoHealed{
				AutoHealed: &v1.AutoHealedCondition{
					AutoHealed:   true,
					HealStrategy: rec.FixAction.Type,
				},
			},
		}
	case "skipped_low_confidence", "skipped_cooldown", "skipped_no_action":
		resp.RootCauseCondition = &v1.RootCauseCondition{
			Condition: &v1.RootCauseCondition_HealAttempts{
				HealAttempts: &v1.HealAttemptsCondition{
					Attempts:     0,
					MaxAttempts:  int32(rec.FixAction.MaxAttempts),
					LastStrategy: rec.FixAction.Type,
				},
			},
		}
	case "failed":
		resp.RootCauseCondition = &v1.RootCauseCondition{
			Condition: &v1.RootCauseCondition_SelfCheckStatus{
				SelfCheckStatus: &v1.SelfCheckStatusCondition{
					CheckName: rec.RuleID,
					Status:    "failed",
					Message:   rec.Reason,
				},
			},
		}
	}

	return resp, nil
}
