package biz

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// FlowLogRecord is one persisted flow_log_events row.
type FlowLogRecord struct {
	ID          string
	TraceID     string
	SessionID   string
	RunID       string
	TeamID      string
	Domain      string
	AgentKey    string
	StepID      string
	FlowPhase   string
	Severity    string
	Title       string
	Message     string
	PayloadJSON string
	CreatedAt   time.Time
}

// FlowLogQuery filters historical flow logs.
type FlowLogQuery struct {
	TraceID   string
	SessionID string
	RunID     string
	Severity  string
	Domain    string
	Since     time.Time
	Until     time.Time
	Limit     int
	Offset    int
}

// FlowLogListResult is a paginated flow log list.
type FlowLogListResult struct {
	Items []FlowLogRecord
	Total int
}

// FlowLogRepo persists FlowLogger v2 entries.
type FlowLogRepo interface {
	Insert(ctx context.Context, rec FlowLogRecord) error
	List(ctx context.Context, q FlowLogQuery) (FlowLogListResult, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// FlowLogUsecase reads/writes flow log history.
type FlowLogUsecase struct {
	repo FlowLogRepo
}

func NewFlowLogUsecase(repo FlowLogRepo) *FlowLogUsecase {
	if repo == nil {
		return nil
	}
	return &FlowLogUsecase{repo: repo}
}

func (uc *FlowLogUsecase) Save(ctx context.Context, rec FlowLogRecord) error {
	if uc == nil || uc.repo == nil {
		return nil
	}
	return uc.repo.Insert(ctx, rec)
}

func (uc *FlowLogUsecase) List(ctx context.Context, q FlowLogQuery) (FlowLogListResult, error) {
	if uc == nil || uc.repo == nil {
		return FlowLogListResult{}, nil
	}
	if q.Limit <= 0 {
		q.Limit = 200
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}
	if strings.TrimSpace(q.TraceID) == "" &&
		strings.TrimSpace(q.SessionID) == "" &&
		strings.TrimSpace(q.RunID) == "" {
		return FlowLogListResult{}, nil
	}
	return uc.repo.List(ctx, q)
}

const defaultFlowLogTTLDays = 30

// PurgeExpired deletes flow_log_events older than FLOW_LOG_TTL_DAYS (default 30).
func (uc *FlowLogUsecase) PurgeExpired(ctx context.Context) (int64, error) {
	if uc == nil || uc.repo == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-flowLogTTL())
	return uc.repo.DeleteOlderThan(ctx, cutoff)
}

func flowLogTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("FLOW_LOG_TTL_DAYS"))
	if raw == "" {
		return time.Duration(defaultFlowLogTTLDays) * 24 * time.Hour
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return time.Duration(defaultFlowLogTTLDays) * 24 * time.Hour
	}
	return time.Duration(days) * 24 * time.Hour
}
