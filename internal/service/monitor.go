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

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type MonitorService struct {
	v1.UnimplementedMonitorServiceServer

	uc *biz.MonitorUsecase
}

func NewMonitorService(uc *biz.MonitorUsecase) *MonitorService {
	return &MonitorService{uc: uc}
}

func bizAuditToProto(a biz.AuditLog) *v1.AuditLog {
	return &v1.AuditLog{
		Id:          a.ID,
		Action:      a.Action,
		Resource:    a.Resource,
		ResourceId:  a.ResourceID,
		RequestId:   a.RequestID,
		Detail:      a.Detail,
		CreatedAt:   a.CreatedAt,
		Actor:       a.Actor,
		Ip:          a.IP,
		UserAgent:   a.UserAgent,
		Severity:    a.Severity,
		MetadataJson: a.MetadataJSON,
	}
}

func bizMonitorRowToProto(row biz.MonitorPlatformRow) *v1.MonitorPlatformRow {
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
		ConfigJson:   sanitizeJSONString(row.ConfigJSON),
		MetadataJson: sanitizeJSONString(row.MetadataJSON),
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
		out = append(out, bizMonitorRowToProto(row))
	}
	return &v1.ListMonitorEventsResponse{Items: out, Total: result.Total}, nil
}

func (s *MonitorService) GetMonitorEvent(ctx context.Context, in *v1.GetMonitorEventRequest) (*v1.MonitorPlatformRow, error) {
	row, err := s.uc.GetMonitorEvent(ctx, in.GetId())
	if err != nil {
		return nil, notFoundMonitor(err)
	}
	return bizMonitorRowToProto(row), nil
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
		out = append(out, bizMonitorRowToProto(row))
	}
	return &v1.ListMonitorTracesResponse{Items: out, Total: result.Total}, nil
}

func (s *MonitorService) GetMonitorTrace(ctx context.Context, in *v1.GetMonitorTraceRequest) (*v1.MonitorTraceDetail, error) {
	row, err := s.uc.GetMonitorTrace(ctx, in.GetId())
	if err != nil {
		return nil, notFoundMonitor(err)
	}
	cfg := parseJSONMap(row.ConfigJSON)
	spans := traceSpansRaw(cfg)
	spansJSON, _ := json.Marshal(spans)
	cfgSanitized := sanitizeJSONString(row.ConfigJSON)
	metaSanitized := sanitizeJSONString(row.MetadataJSON)
	tr := bizMonitorRowToProto(row)
	return &v1.MonitorTraceDetail{
		Trace:        tr,
		ConfigJson:   cfgSanitized,
		MetadataJson: metaSanitized,
		SpansJson:    string(spansJSON),
	}, nil
}

func (s *MonitorService) GetMonitorLogs(context.Context, *v1.GetMonitorLogsRequest) (*v1.GetMonitorLogsResponse, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return &v1.GetMonitorLogsResponse{
		Items: []*v1.MonitorLogLine{{
			Id:        "ws-hint",
			Time:      now,
			Level:     "INFO",
			Message:   "Live log lines are pushed via WebSocket (server.ws port); GET snapshot here lists hints only.",
			Source:    "monitor",
			CreatedAt: now,
		}},
		Enabled: true,
		Message: "Use WebSocket /v1/ws to subscribe to monitor channels (logs, events).",
	}, nil
}

func sanitizeJSONString(raw string) string {
	parsed := parseJSONMap(raw)
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

func parseJSONMap(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
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
