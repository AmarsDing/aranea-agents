// Package flowlog implements flow log persistence and query workflows.
package flowlog

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// Record is one persisted flow_log_events row.
type Record struct {
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

// Query filters historical flow logs.
type Query struct {
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

// ListResult is a paginated flow log list.
type ListResult struct {
	Items []Record
	Total int
}

// Repo persists FlowLogger v2 entries.
type Repo interface {
	Insert(ctx context.Context, rec Record) error
	List(ctx context.Context, q Query) (ListResult, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// Usecase reads/writes flow log history.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs a FlowLogUsecase.
func NewUsecase(repo Repo) *Usecase {
	if repo == nil {
		return nil
	}
	return &Usecase{repo: repo}
}

// Save inserts a flow log record.
func (uc *Usecase) Save(ctx context.Context, rec Record) error {
	if uc == nil || uc.repo == nil {
		return nil
	}
	return uc.repo.Insert(ctx, rec)
}

// List returns a page of flow log records matching the query.
func (uc *Usecase) List(ctx context.Context, q Query) (ListResult, error) {
	if uc == nil || uc.repo == nil {
		return ListResult{}, nil
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
		return ListResult{}, nil
	}
	return uc.repo.List(ctx, q)
}

const defaultFlowLogTTLDays = 30

// PurgeExpired deletes flow_log_events older than FLOW_LOG_TTL_DAYS (default 30).
func (uc *Usecase) PurgeExpired(ctx context.Context) (int64, error) {
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
