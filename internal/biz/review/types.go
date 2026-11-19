package review

import (
	"errors"
	"time"
)

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// 决议哨兵错误（service 层据此映射 HTTP 状态码）：
//   - ErrConflict：并发决议/状态已被推进——恰一个决议成功，其余 409；
//   - ErrSelfApproval：内容创建者自批——服务端强制拒绝（400），
//     reviewer 一律取 JWT 主体（review.v1 收编后禁请求体自报）；
//   - ErrNotFound：任务不存在——404；
//   - ErrIllegalTransition：状态机不允许的迁移（见 state_machine.go）——409。
var (
	ErrConflict     = errors.New("review: concurrent decision conflict")
	ErrSelfApproval = errors.New("review: self-approval forbidden")
	ErrNotFound     = errors.New("review: task not found")
)

// Task is a single-person final review gate over draft content.
type Task struct {
	ID               string
	ContentItemID    string
	GraphExecutionID string
	GraphNodeID      string
	GraphTaskID      string
	DraftArtifactID  string
	PlatformTargets []string
	MachineFlags    []string
	DraftTitle      string
	DraftBody       string
	Status          string
	DecisionNote    string
	ReviewerID      string
	// CreatorID 内容创建者（JWT 主体 id 的字符串形态）。图执行机器入队的
	// 任务无人类创建者（空串），不可自批判定只在两侧均非空时生效。
	CreatorID string
	DecidedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repo interface {
	Create(t Task) (Task, error)
	Get(id string) (Task, error)
	Update(t Task) (Task, error)
	ListPending(limit, offset int) ([]Task, int, error)
	GetByGraphExecution(executionID, nodeID string) (Task, bool, error)
	// Decide 以 CAS 方式落决议：仅当当前状态等于 from 时生效（状态机合法性
	// 由用例层先行校验，本方法是竞态防线）。0 行命中时：任务不存在返回
	// not found 错误；存在但状态已推进返回 ErrConflict。
	Decide(id, from, to, reviewerID, note string, decidedAt time.Time) (Task, error)
}
