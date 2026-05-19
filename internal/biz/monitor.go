package biz

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID          string
	Action      string
	Resource    string
	ResourceID  string
	RequestID   string
	Detail      string
	CreatedAt   string
	Actor       string
	IP          string
	UserAgent   string
	Severity    string
	MetadataJSON string
}

type AuditQuery struct {
	Limit    int32
	Offset   int32
	Action   string
	Resource string
	Actor    string
	Keyword  string
}

type AuditListResult struct {
	Items []AuditLog
	Total int32
}

type MonitorPlatformRow struct {
	Resource     string
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ParentID     string
	Level        string
	AgentID      string
	Provider     string
	Model        string
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

type MonitorEventsQuery struct {
	Limit     int32
	Offset    int32
	EventType string
	AgentID   string
	Status    string
}

type MonitorTracesQuery struct {
	Limit    int32
	Offset   int32
	AgentID  string
	Provider string
	Model    string
	Status   string
}

type MonitorListResult struct {
	Items []MonitorPlatformRow
	Total int32
}

type MonitorEventWrite struct {
	EventKey     string
	Name         string
	Description  string
	Status       string
	MetadataJSON string
}

type MonitorRepo interface {
	ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error)
	InsertAuditLog(ctx context.Context, entry AuditLog) error
	InsertMonitorEvent(ctx context.Context, ev MonitorEventWrite) error
	ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error)
	GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error)
	ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error)
	GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error)
}

type MonitorUsecase struct {
	repo MonitorRepo
}

func NewMonitorUsecase(repo MonitorRepo) *MonitorUsecase {
	return &MonitorUsecase{repo: repo}
}

func auditLimit(limit int32) int {
	l := int(limit)
	if l <= 0 {
		l = 200
	}
	return l
}

// RecordAuditLog persists an admin audit row (best-effort).
func (u *MonitorUsecase) RecordAuditLog(ctx context.Context, entry AuditLog) error {
	if u == nil || u.repo == nil {
		return nil
	}
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = uuid.NewString()
	}
	return u.repo.InsertAuditLog(ctx, entry)
}

// RecordMonitorEvent persists a monitor_events row (best-effort).
func (u *MonitorUsecase) RecordMonitorEvent(ctx context.Context, ev MonitorEventWrite) error {
	if u == nil || u.repo == nil {
		return nil
	}
	return u.repo.InsertMonitorEvent(ctx, ev)
}

func (u *MonitorUsecase) ListAuditLogs(ctx context.Context, query AuditQuery) (AuditListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	return u.repo.ListAuditLogs(ctx, query)
}

func (u *MonitorUsecase) ListMonitorEvents(ctx context.Context, query MonitorEventsQuery) (MonitorListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorEvents(ctx, query)
}

func (u *MonitorUsecase) GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorEvent(ctx, id)
}

func (u *MonitorUsecase) ListMonitorTraces(ctx context.Context, query MonitorTracesQuery) (MonitorListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return u.repo.ListMonitorTraces(ctx, query)
}

func (u *MonitorUsecase) GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorTrace(ctx, id)
}
