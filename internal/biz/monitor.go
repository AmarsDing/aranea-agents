package biz

import (
	"context"
)

// AuditLog mirrors SQLite audit_logs.
type AuditLog struct {
	ID         string
	Action     string
	Resource   string
	ResourceID string
	RequestID  string
	Detail     string
	CreatedAt  string
}

// MonitorPlatformRow mirrors legacy domain.PlatformResource for monitor-events / monitor-traces.
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

type MonitorRepo interface {
	ListAuditLogs(ctx context.Context, limit int) ([]AuditLog, error)
	ListMonitorEvents(ctx context.Context) ([]MonitorPlatformRow, error)
	GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error)
	ListMonitorTraces(ctx context.Context) ([]MonitorPlatformRow, error)
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

func (u *MonitorUsecase) ListAuditLogs(ctx context.Context, limit int32) ([]AuditLog, error) {
	return u.repo.ListAuditLogs(ctx, auditLimit(limit))
}

func (u *MonitorUsecase) ListMonitorEvents(ctx context.Context) ([]MonitorPlatformRow, error) {
	return u.repo.ListMonitorEvents(ctx)
}

func (u *MonitorUsecase) GetMonitorEvent(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorEvent(ctx, id)
}

func (u *MonitorUsecase) ListMonitorTraces(ctx context.Context) ([]MonitorPlatformRow, error) {
	return u.repo.ListMonitorTraces(ctx)
}

func (u *MonitorUsecase) GetMonitorTrace(ctx context.Context, id string) (MonitorPlatformRow, error) {
	return u.repo.GetMonitorTrace(ctx, id)
}
