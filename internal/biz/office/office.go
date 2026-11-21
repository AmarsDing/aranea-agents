// Package office 实现 E 阶段 W1c 办公套件一期（需求文档 §2，拍板 K-1..K-5）：
// 日程（office.schedule）、通知中心 MVP（office.notify）、应用启动器
// （office.apps）。
//
// 平移语义（自 workbench server/misc.go 逐字保留，验收用例随
// verify-schedule.ps1 S1-S6 平移）：
//   - 同人同时段重叠 → 409（创建与任务排期两条路径同语义：重叠判定
//     NOT (end_at <= start OR start_at >= end)）；
//   - end_at > start_at 强制；title 必填；
//   - 任务双向联动：ScheduleTask 正向（标题缺省「执行：」前缀）、
//     ListTaskEvents 反查。
//
// K 拍板新增：
//   - K-1 ListEventsRange 范围聚合（月历一次请求，替代逐日 28-31 请求）；
//   - K-2 RRULE 重复规则不做；
//   - K-3 ics 只读订阅（token-in-path 公开 feed，token 只存 SHA-256 哈希，
//     明文仅创建时出现一次，吊销即 401）；
//   - K-4 reminder_minutes + ReminderWorker 每分钟扫描 → 通知中心落表 +
//     声明为提醒接收点的 IM 通道投递（at-least-once + 幂等键去重）；
//   - K-5 office.apps URL 启动器（command 类本机启动属远控面，F2 另评估）。
//
// agent 排期草案：SubmitScheduleDraft 只产草案（office_schedule_drafts），
// 经审批中心 kind=schedule_draft 逐项审批，Approve 在单事务内逐项复检冲突
// 后落 office_events——需求概要「agent 规划草案逐项审批」的落点。
package office

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
)

// 事件来源（office_events.source）。
const (
	SourceManual       = "manual"
	SourceTaskSchedule = "task_schedule"
	SourceAgentDraft   = "agent_draft"
)

// 日程条目类型（office_events.kind）：计划是事前打算做的，工作内容是事后的记录。
const (
	KindPlan = "plan"
	KindLog  = "log"
)

// 计划状态（office_events.status）。「逾期」不存，由结束时间推算。
const (
	StatusTodo      = "todo"
	StatusDoing     = "doing"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
)

// ValidKind / ValidStatus 入参白名单（空串由调用方按默认值处理）。
func ValidKind(k string) bool { return k == KindPlan || k == KindLog }
func ValidStatus(s string) bool {
	return s == StatusTodo || s == StatusDoing || s == StatusDone || s == StatusCancelled
}

// 通知类别（office_notifications.kind）。
const (
	NotifyKindScheduleReminder = "schedule_reminder"
	NotifyKindMailDigest       = "mail_digest"
	NotifyKindApprovalTodo     = "approval_todo"
	NotifyKindSystem           = "system"
)

// 草案状态（office_schedule_drafts.status）。
const (
	DraftStatusPending  = "pending"
	DraftStatusApproved = "approved"
	DraftStatusRejected = "rejected"
)

// 领域哨兵错误（service 层经 apierror.IsCode 之外的谓词映射 HTTP 状态：
// 冲突 409 / 未找到 404 / 非法入参 400 / 前置状态 409）。
var (
	// ErrEventConflict 同人同时段重叠（创建/更新/排期/草案批准共用）。
	ErrEventConflict = apierror.Conflict(DomainOffice, "event time conflict")
	// ErrEventNotFound 事件不存在或不属于调用者。
	ErrEventNotFound = apierror.NotFound(DomainOffice, "event not found")
	// ErrInvalidTimeRange 时间区间非法（end<=start 或缺界）。
	ErrInvalidTimeRange = apierror.BadRequest(DomainOffice, "invalid time range")
	// ErrTitleRequired 标题必填。
	ErrTitleRequired = apierror.BadRequest(DomainOffice, "title is required")
	// ErrInvalidKind / ErrInvalidStatus 日程条目类型、状态不在白名单内。
	ErrInvalidKind   = apierror.BadRequest(DomainOffice, "invalid event kind")
	ErrInvalidStatus = apierror.BadRequest(DomainOffice, "invalid event status")
	ErrInvalidRRule  = apierror.BadRequest(DomainOffice, "unsupported recurrence rule")
	// ErrDraftNotFound 草案不存在。
	ErrDraftNotFound = apierror.NotFound(DomainOffice, "schedule draft not found")
	// ErrDraftNotPending 草案已被决议（并发决议恰一个成功，其余得此错误）。
	ErrDraftNotPending = apierror.Conflict(DomainOffice, "schedule draft already decided")
	// ErrDraftEmpty 草案无条目。
	ErrDraftEmpty = apierror.BadRequest(DomainOffice, "schedule draft has no items")
	// ErrNotificationNotFound 通知不存在或不属于调用者。
	ErrNotificationNotFound = apierror.NotFound(DomainOffice, "notification not found")
	// ErrICalTokenInvalid 订阅令牌无效或已吊销（feed 端点映 401）。
	ErrICalTokenInvalid = apierror.Unauthorized(DomainOffice, "ical token invalid or revoked")
	// ErrICalTokenNotFound 吊销目标不存在/不属调用者/已吊销（管理端映 404）。
	ErrICalTokenNotFound = apierror.NotFound(DomainOffice, "ical token not found")
	// ErrAppNotFound 应用不存在。
	ErrAppNotFound = apierror.NotFound(DomainOffice, "app not found")
	// ErrRejectNoteRequired 驳回必填理由（与 review.v1 同约）。
	ErrRejectNoteRequired = apierror.BadRequest(DomainOffice, "reject note is required")
)

// DomainOffice 是本包 apierror 域码。
const DomainOffice = "OFFICE"

// DefaultTimezone 是事件缺省时区（IANA）。
const DefaultTimezone = "Asia/Shanghai"

// ============================================================================
// 领域类型
// ============================================================================

// Event 日程事件。
type Event struct {
	ID              string
	WorkspaceID     string
	UserID          string
	TaskRef         string
	TaskTitle       string // 排期时快照（反查展示，替代 workbench LEFT JOIN）
	Title           string
	Note            string
	StartAt         time.Time
	EndAt           time.Time
	Timezone        string
	ReminderMinutes int
	Source          string
	Kind            string // plan | log
	Status          string // todo | doing | done | cancelled
	AllDay          bool
	Outcome         string // 完成情况 / 结果
	Location        string
	URL             string
	Color           string // 分类色键：'' | work | meeting | personal | focus | travel
	RRule           string // 重复规则（见 recur.go），空 = 不重复
	ExDates         string // 跳过的日期 YYYY-MM-DD，逗号分隔
	// RecurrenceDay 只在按范围展开时填：这一项是系列在哪天的那一次（ID 仍是系列的）。
	RecurrenceDay string
	RemindedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EventPatch 更新时有「是否给了值」之分的字段（nil = 不改），
// 让清空完成情况、取消全天这类操作可以表达。
type EventPatch struct {
	Note     *string // 给了就改（可清空）；nil 时沿用旧口径：patch.Note 非空才改
	Kind     *string
	Status   *string
	AllDay   *bool
	Outcome  *string
	Location *string
	URL      *string
	Color    *string
	RRule    *string
	ExDates  *string
}

// DraftItem 草案单条排期项（与 proto ScheduleDraftItem 对应）。
type DraftItem struct {
	Title           string    `json:"title"`
	Note            string    `json:"note,omitempty"`
	StartAt         time.Time `json:"start_at"`
	EndAt           time.Time `json:"end_at"`
	ReminderMinutes int       `json:"reminder_minutes,omitempty"`
	TaskRef         string    `json:"task_ref,omitempty"`
}

// ScheduleDraft agent 排期草案（审批中心 kind=schedule_draft 的领域承载）。
type ScheduleDraft struct {
	ID          string
	WorkspaceID string
	UserID      string // 批准落库后的日历属主（提交时 JWT 主体）
	AgentID     string
	SessionID   string
	Status      string
	Items       []DraftItem
	DecidedBy   string
	DecideNote  string
	DecidedAt   *time.Time
	CreatedAt   time.Time
}

// Notification 通知中心条目。
type Notification struct {
	ID        string
	UserID    string
	Kind      string
	Title     string
	Body      string
	Link      string
	Read      bool
	CreatedAt time.Time
}

// ICalToken ics 订阅令牌（Token 明文仅创建时存在于内存，不落库）。
type ICalToken struct {
	ID        string
	UserID    string
	TokenHash string
	Name      string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// App 应用启动器条目。
type App struct {
	ID          string
	WorkspaceID string
	Name        string
	URL         string
	Icon        string
	GroupName   string
	SortOrder   int
	CreatedAt   time.Time
}

// ============================================================================
// 仓储窄端口（data 层实现，wire 装配；便于测试桩注入）
// ============================================================================

// EventRepo 日程事件仓储。
// Stability:evolving
type EventRepo interface {
	CreateEvent(ctx context.Context, ev *Event) error
	// UpdateEvent 按 id+user_id 限定更新（属主隔离在 SQL 内完成）。
	UpdateEvent(ctx context.Context, ev *Event) error
	DeleteEvent(ctx context.Context, id, userID string) error
	GetEvent(ctx context.Context, id, userID string) (*Event, error)
	// ListEventsRange 返回与 [start,end) 相交的本人事件（K-1 范围聚合），
	// 按 start_at 升序。
	ListEventsRange(ctx context.Context, userID string, start, end time.Time) ([]*Event, error)
	// ListTaskEvents 任务反查时间块（本人 + task_ref），按 start_at 升序。
	ListTaskEvents(ctx context.Context, userID, taskRef string) ([]*Event, error)
	// CountOverlap 统计本人与 [start,end) 相交的事件数；excludeID 非空时
	// 排除自身（更新路径用）。
	CountOverlap(ctx context.Context, userID string, start, end time.Time, excludeID string) (int, error)
	// ListDueReminders 拾取到期未提醒事件：reminder_minutes>0 且
	// reminded_at IS NULL 且 start_at - reminder <= now。迟到补投（进程
	// 停机错过的窗口在恢复后首个 tick 补发），按 start_at 升序限量。
	ListDueReminders(ctx context.Context, now time.Time, limit int) ([]*Event, error)
	// ListRecurringReminders 带提醒的重复计划系列（未完成、未取消），提醒 worker 自己算下一次。
	ListRecurringReminders(ctx context.Context, limit int) ([]*Event, error)
	// SearchEvents 本人日程关键字检索（标题 / 说明 / 完成情况 / 地点），按开始时间倒序限量。
	SearchEvents(ctx context.Context, userID, keyword string, limit int) ([]*Event, error)
	// MarkReminded 置提醒已投递标记（与通知落表同事务由调用方组合；
	// 本方法仅承担单写，幂等由 reminded_at IS NULL 前置查询保证）。
	MarkReminded(ctx context.Context, id string, at time.Time) error
}

// NotificationRepo 通知中心仓储（写路径仅供进程内生产方，无 RPC）。
// Stability:evolving
type NotificationRepo interface {
	CreateNotification(ctx context.Context, n *Notification) error
	// ListNotifications 返回本人通知（可选仅未读），按 created_at 倒序分页；
	// 同时给出 total（过滤后）与 unreadCount（恒为本人未读总数，铃铛徽标用）。
	ListNotifications(ctx context.Context, userID string, unreadOnly bool, page, pageSize int) ([]*Notification, int64, int64, error)
	// DismissNotification 置已读（id+user_id 限定，属主隔离在 SQL 内）；
	// 行不存在返回 ErrNotificationNotFound。
	DismissNotification(ctx context.Context, id, userID string) error
	MarkAllNotificationsRead(ctx context.Context, userID string) error
}

// ICalTokenRepo ics 订阅令牌仓储。
// Stability:evolving
type ICalTokenRepo interface {
	CreateICalToken(ctx context.Context, t *ICalToken) error
	// RevokeICalToken 按 id+user_id 吊销（置 revoked_at）；行不存在返回
	// ErrAppNotFound 同族 404（service 映 404）。
	RevokeICalToken(ctx context.Context, id, userID string) error
	ListICalTokens(ctx context.Context, userID string) ([]*ICalToken, error)
	// ResolveICalToken 按哈希解析有效令牌 → 属主 user_id；不存在或已吊销
	// 返回 ErrICalTokenInvalid。
	ResolveICalToken(ctx context.Context, tokenHash string) (string, error)
}

// AppRepo 应用启动器仓储（workspace 维度）。
// Stability:evolving
type AppRepo interface {
	UpsertApp(ctx context.Context, a *App) error
	DeleteApp(ctx context.Context, id, workspaceID string) error
	ListApps(ctx context.Context, workspaceID string) ([]*App, error)
}

// DraftRepo 排期草案仓储。
// Stability:evolving
type DraftRepo interface {
	CreateDraft(ctx context.Context, d *ScheduleDraft) error
	GetDraft(ctx context.Context, id string) (*ScheduleDraft, error)
	// ListPendingDrafts 审批中心 schedule_draft 源读口（workspace 维度，
	// 按 created_at 倒序）。
	ListPendingDrafts(ctx context.Context, workspaceID string, limit int) ([]*ScheduleDraft, error)
	CountPendingDrafts(ctx context.Context, workspaceID string) (int64, error)
	// ApproveDraftTx 单事务批准：逐项复检与属主已有事件重叠（任一冲突即
	// ErrEventConflict 整体回滚）、插入 events、CAS pending→approved
	// （0 行受影响即 ErrDraftNotPending）。
	ApproveDraftTx(ctx context.Context, draftID, decidedBy string, events []*Event) error
	// RejectDraft CAS pending→rejected（0 行受影响即 ErrDraftNotPending）。
	RejectDraft(ctx context.Context, draftID, decidedBy, note string) error
}
