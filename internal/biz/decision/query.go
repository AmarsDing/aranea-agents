package decision

import (
	"context"
	"strings"
	"time"
)

// 统一查询面（M80 设计 §4.1/§4.2）：decision_records 的只读查询契约。
// 写侧契约见 collector.go 的 Repo；读侧单列 QueryRepo，避免 outbox 的
// 测试替身被迫长出查询方法。

// ListFilter 是 ListDecisionRecords 的过滤条件（零值 = 不过滤）。
// EntityType/EntityKey 需同时给出才生效（related_entities 数组内对象匹配）。
//
// VisibleWorkspaces 是 workspace 隔离过滤（2026-08-27 t-dr-3，N5 IDOR 同
// 策略）：nil = 不过滤（系统 caller）；非 nil 时 SQL 侧 workspace_id IN
// (...)。service 层对非系统 caller 填 [callerWS, ""]——空串是共享记录
// （无租户的旧数据/系统路径产物），镜像 AssertWorkspaceOrShared 的
// "共享可读"语义；租户记录只对本租户可见。
type ListFilter struct {
	Category          string
	ActorKey          string
	EntityType        string
	EntityKey         string
	SourceRunID       string
	VisibleWorkspaces []string
	TimeFrom          time.Time
	TimeTo            time.Time
	Page              int
	PageSize          int
}

// maxListPageSize 是设计 §4.1 的 page_size 上限。
const maxListPageSize = 100

// NormalizePage 收敛分页参数：page<1 → 1；page_size∉[1,100] → 默认 20
// （>100 截断为 100）。返回 LIMIT/OFFSET 计算好的值。
func (f *ListFilter) NormalizePage() (limit, offset int) {
	if f.Page < 1 {
		f.Page = 1
	}
	switch {
	case f.PageSize <= 0:
		f.PageSize = 20
	case f.PageSize > maxListPageSize:
		f.PageSize = maxListPageSize
	}
	return f.PageSize, (f.Page - 1) * f.PageSize
}

// QueryRepo 是读侧持久化契约（internal/data 实现）。实现须双方言
// （SQLite/PG）可用：JSON 过滤走 Dialect 辅助，禁全表扫。
type QueryRepo interface {
	// ListRecords 按过滤条件分页查询；total 为同条件总行数。
	ListRecords(ctx context.Context, f ListFilter) (items []Record, total int64, err error)
	// GetByKey 按 decision_key 精确查询；未命中返回 nil, nil。
	GetByKey(ctx context.Context, decisionKey string) (*Record, error)
}

// QueryUsecase 是查询面的 biz 入口（service 层唯一依赖）。
// repo 为 nil（CLI/无库）时所有方法返回空结果而非 panic。
type QueryUsecase struct {
	repo QueryRepo
}

// NewQueryUsecase 构造查询用例。
func NewQueryUsecase(repo QueryRepo) *QueryUsecase {
	return &QueryUsecase{repo: repo}
}

// List 分页查询决策记录。
func (u *QueryUsecase) List(ctx context.Context, f ListFilter) ([]Record, int64, error) {
	if u == nil || u.repo == nil {
		return nil, 0, nil
	}
	f.NormalizePage()
	return u.repo.ListRecords(ctx, f)
}

// Get 按 decision_key 查询单条记录；未命中返回 nil。
func (u *QueryUsecase) Get(ctx context.Context, decisionKey string) (*Record, error) {
	if u == nil || u.repo == nil || strings.TrimSpace(decisionKey) == "" {
		return nil, nil
	}
	return u.repo.GetByKey(ctx, decisionKey)
}
