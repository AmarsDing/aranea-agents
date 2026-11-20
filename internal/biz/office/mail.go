package office

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
)

// ============================================================================
// 邮件域（E 阶段 W1d，需求文档 §1，拍板 E-1..E-5）
//
// 平移语义（自 workbench server/imap.go、misc.go L270-479、schema.sql
// L210-240，验收用例随 verify-imap.ps1 九节平移）：
//   - IMAP tls/starttls/none 三模式；IDLE 推送为主 + 轮询兜底（E-5）；
//   - (account_id,uid) 部分唯一索引幂等去重；每封邮件一个事务（邮件+附件
//     同生共死，附件失败整体回滚可重试）；
//   - 附件 BYTEA 入库 + 16MB 上限（对齐 IMAP 字面量上限，E-4）。
//
// 拍板变更：
//   - E-1 账号密码写后即弃：存储 password_enc（AES-GCM 信封，credential
//     设施），无明文列、无任何明文回显 RPC；解密仅发生在同步/IDLE 拨号前
//     的进程内存中，明文三不进（日志/prompt/API 响应）；
//   - 补齐现状缺口：账号更新/删除、邮件删除、.eml 导出；
//   - MailToTask 契约修正：space 维度由服务端上下文解析（workspace+user），
//     落 office_tasks（统一任务中心前身），邮件行 task_ref 回写；
//   - E-2 SMTP 发信 F2（高风险闸同批）；E-3 四象限分拣 F2（列已预留）。
// ============================================================================

// TLS 模式（office_mail_accounts.tls_mode）。
const (
	MailTLSTLS      = "tls"      // 993 直连 TLS
	MailTLSStartTLS = "starttls" // 明文起跳后 STARTTLS 升级
	MailTLSNone     = "none"     // 明文（仅测试/mock）
)

// 附件大小上限（E-4）：16MB，对齐 IMAP 字面量上限（mail_imap.go 同值卡口）。
const MailAttachmentMaxBytes = 16 << 20

// 邮件域哨兵错误（service 层映 HTTP：404/400/502/409）。
var (
	// ErrMailAccountNotFound 账号不存在或不属于调用者。
	ErrMailAccountNotFound = apierror.NotFound(DomainOffice, "mail account not found")
	// ErrMailMessageNotFound 邮件不存在或不属于调用者。
	ErrMailMessageNotFound = apierror.NotFound(DomainOffice, "mail message not found")
	// ErrMailAttachmentNotFound 附件不存在或不属于调用者。
	ErrMailAttachmentNotFound = apierror.NotFound(DomainOffice, "mail attachment not found")
	// ErrMailHostRequired host/username/password 必填。
	ErrMailHostRequired = apierror.BadRequest(DomainOffice, "mail host, username and password are required")
	// ErrMailTLSModeInvalid tls_mode 非法（仅 tls|starttls|none）。
	ErrMailTLSModeInvalid = apierror.BadRequest(DomainOffice, "tls_mode must be tls, starttls or none")
	// ErrMailAttachmentTooLarge 附件超 16MB 上限（E-4）。
	ErrMailAttachmentTooLarge = apierror.BadRequest(DomainOffice, "mail attachment exceeds 16MB limit")
	// ErrMailEmlTooLarge .eml 导入超 16MB 上限。
	ErrMailEmlTooLarge = apierror.BadRequest(DomainOffice, "eml payload exceeds 16MB limit")
	// ErrMailUpstream IMAP 上游不可达/握手失败/凭据被拒（service 层识别本哨兵
	// 映 HTTP 502——apierror 无 BadGateway 族，映射收口在 service；明细进
	// 审计，不回传上游原文防凭据泄漏）。
	ErrMailUpstream = apierror.Unavailable(DomainOffice, "mail upstream unavailable")
	// ErrMailTaskExists 邮件已转任务（task_ref 非空，幂等返回既有引用而非报错——
	// 由 usecase 判空短路，本错误保留给显式冲突路径）。
	ErrMailTaskExists = apierror.Conflict(DomainOffice, "mail message already converted to task")
)

// ============================================================================
// 领域类型
// ============================================================================

// MailAccount 邮箱账号。PasswordEnc 为信封密文，仅限 biz/data 内部流转；
// service 层出参永不含本字段（无回显 RPC，验收 #6）。
type MailAccount struct {
	ID          string
	WorkspaceID string
	UserID      string
	Name        string
	Host        string
	Port        int
	TLSMode     string
	Username    string
	PasswordEnc string
	LastUID     int64
	LastSyncAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// 邮件 P1：发件地址、服务商预设键、SMTP（用同一组用户名 / 授权码）、
	// 连接状态（'' 未检查 | ok | auth_failed | unreachable）、首次同步范围。
	Email     string
	Provider  string
	SMTPHost  string
	SMTPPort  int
	SMTPTLS   string
	Status    string
	LastError string // 错误类别（见 MailErrKind*），不存上游原文
	SyncSince *time.Time
}

// 账号连接状态。
const (
	MailStatusOK          = "ok"
	MailStatusAuthFailed  = "auth_failed"
	MailStatusUnreachable = "unreachable"
)

// 邮件文件夹。
const (
	MailFolderInbox  = "inbox"
	MailFolderSent   = "sent"
	MailFolderDrafts = "drafts"
)

// MailMessage 邮件（Snippet 写时物化：正文前 200 字符，列表免读全文）。
type MailMessage struct {
	ID             string
	WorkspaceID    string
	UserID         string
	AccountID      string // .eml 导入为空串（不参与唯一索引）
	UID            int64
	Subject        string
	Sender         string
	Body           string
	Snippet        string
	ReceivedAt     *time.Time
	Classification int
	Quadrant       string // E-3 预留
	TaskRef        string // 转任务回写
	CreatedAt      time.Time
	// 邮件 P1
	To         string // 收件人（解码后的地址列表原文，逗号分隔）
	Cc         string
	Seen       bool
	Flagged    bool
	ArchivedAt *time.Time
	Folder     string // inbox | sent | drafts（空按 inbox）
	BodyHTML   string // 入库前已清洗（sanitizeMailHTML）
	MessageIDHdr  string
	ReferencesHdr string
	// HasAttachments 列表物化标记（附件数>0），DB 不存列、查询聚出。
	HasAttachments bool
	// Attachments 仅同步/导入事务内随邮件携带；查询路径经 AttachmentRepo。
	Attachments []*MailAttachment
}

// MailAttachment 附件（Content 仅下载/入知识库路径装载）。
type MailAttachment struct {
	ID             string
	MessageID      string
	Filename       string
	ContentType    string
	Size           int64
	Content        []byte
	KnowledgeDocID string
	CreatedAt      time.Time
}

// OfficeTask 个人任务（邮件转任务 F1 落点；统一任务中心前身）。
type OfficeTask struct {
	ID          string
	WorkspaceID string
	UserID      string
	Title       string
	Body        string
	Source      string // manual | mail | delegate | note
	SourceRef   string // 邮件转任务：office_mail_messages.id；指令栏交办：承载会话 id
	Status      string // open | doing | done
	// 负责人：AssigneeKind 为空 = 属主本人；agent / team = 交办（AI 执行）对象。
	AssigneeKind string
	AssigneeID   string
	AssigneeName string // 交办当时的显示名快照
	SessionID    string // AI 执行的承载会话；没派给 AI 为空
	DueAt        *time.Time
	FileCount    int // 附件数（读路径物化，写路径忽略）
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OfficeTaskFile 任务附件。列表读路径不装载 Content。
type OfficeTaskFile struct {
	ID          string
	TaskID      string
	UserID      string
	Filename    string
	ContentType string
	Size        int64
	Content     []byte
	CreatedAt   time.Time
}

// OfficeTaskFilter 任务列表过滤：Statuses / Sources 非空才过滤；Assigned 为 1 只要
// 交办出去的（有执行对象），-1 只要本人负责的，0 不限。
type OfficeTaskFilter struct {
	Statuses []string
	Sources  []string
	Assigned int
}

// ============================================================================
// 仓储窄端口（data 层实现，wire 装配）
// ============================================================================

// MailAccountRepo 邮箱账号仓储（属主隔离在 SQL 内完成）。
// Stability:evolving
type MailAccountRepo interface {
	CreateMailAccount(ctx context.Context, a *MailAccount) error
	UpdateMailAccount(ctx context.Context, a *MailAccount) error
	// DeleteMailAccount 按 id+user_id 删除；行不存在返回 ErrMailAccountNotFound。
	// 邮件行不级联（account_id 仅引用无 FK），账号删除后历史邮件保留。
	DeleteMailAccount(ctx context.Context, id, userID string) error
	GetMailAccount(ctx context.Context, id, userID string) (*MailAccount, error)
	ListMailAccounts(ctx context.Context, userID string) ([]*MailAccount, error)
	// ListAllMailAccounts IDLE supervisor/轮询兜底扫全量账号（跨用户；
	// 进程内生产方专用，不暴露 RPC）。
	ListAllMailAccounts(ctx context.Context) ([]*MailAccount, error)
	// UpdateMailSyncCursor 同步成功后置 last_uid/last_sync_at。
	UpdateMailSyncCursor(ctx context.Context, id string, lastUID int64, at time.Time) error
	// UpdateMailAccountStatus 记录最近一次连接结果（同步 / 测试）。
	UpdateMailAccountStatus(ctx context.Context, id, status, lastErr string) error
}

// MailMessageRepo 邮件仓储。
// Stability:evolving
type MailMessageRepo interface {
	// InsertMessageWithAttachmentsTx 单事务插入邮件+附件（同生共死）；
	// (account_id,uid) 唯一冲突时返回 inserted=false（幂等命中，非错误）。
	InsertMessageWithAttachmentsTx(ctx context.Context, m *MailMessage) (inserted bool, err error)
	// ListMailMessages 本人邮件分页（accountID 空=全账号聚合；quadrant 可选
	// 过滤，F2），按 received_at 倒序（NULLS LAST）再 created_at 倒序。
	ListMailMessages(ctx context.Context, userID, accountID, quadrant string, page, pageSize int) ([]*MailMessage, int64, error)
	GetMailMessage(ctx context.Context, id, userID string) (*MailMessage, error)
	// DeleteMailMessage 按 id+user_id 删除（附件 ON DELETE CASCADE）；
	// 行不存在返回 ErrMailMessageNotFound。
	DeleteMailMessage(ctx context.Context, id, userID string) error
	// SetMailTaskRef 转任务回写（幂等：已写同值视为成功）。
	SetMailTaskRef(ctx context.Context, id, userID, taskRef string) error
	// ListMailMessagesSince 摘要窗口扫描（本人，received_at>=since，按时间
	// 升序；进程内生产方专用）。
	ListMailMessagesSince(ctx context.Context, userID string, since time.Time, limit int) ([]*MailMessage, error)
	// ListMailDigestUsers 摘要候选人扫描：窗口内有邮件的用户去重列表
	// （每日摘要 job 驱动；进程内生产方专用）。
	ListMailDigestUsers(ctx context.Context, since time.Time) ([]*MailDigestCandidate, error)
	// SearchMailMessages 本人邮件关键词检索（subject/sender/body ILIKE），
	// agent mail.search 工具读口；按 received_at 倒序限量。
	SearchMailMessages(ctx context.Context, userID, query string, limit int) ([]*MailMessage, error)
	// QueryMailMessages 邮件页列表：视图 + 账号 + 关键字（主题、发件人、收件人、
	// 正文、附件名），按时间倒序分页。
	QueryMailMessages(ctx context.Context, userID string, q MailQuery) ([]*MailMessage, int64, error)
	// CountMailbox 左栏各视图计数 + 各账号收件箱未读数。
	CountMailbox(ctx context.Context, userID string) (*MailboxCounts, error)
	// MarkMailMessages 批量标已读 / 星标 / 归档（nil = 不改）；返回改到的行数。
	MarkMailMessages(ctx context.Context, userID string, ids []string, m MailMark) (int64, error)
	// SaveMailDraftTx 单事务：删掉旧草稿（replaceID 非空时）再插入新草稿（含附件）。
	SaveMailDraftTx(ctx context.Context, userID, replaceID string, m *MailMessage) error
}

// 邮件页视图。
const (
	MailViewInbox       = "inbox"       // 收件箱（未归档）
	MailViewUnread      = "unread"      // 收件箱未读
	MailViewFlagged     = "flagged"     // 星标（不含草稿）
	MailViewTask        = "task"        // 已转任务
	MailViewAttachments = "attachments" // 有附件
	MailViewSent        = "sent"
	MailViewDrafts      = "drafts"
	MailViewArchived    = "archived"
)

// MailQuery 邮件页列表查询。
type MailQuery struct {
	View      string // 空 = inbox
	AccountID string
	Keyword   string
	Page      int
	PageSize  int
}

// MailboxCounts 左栏计数。
type MailboxCounts struct {
	InboxUnread  int64
	Flagged      int64
	Task         int64
	Attachments  int64
	Sent         int64
	Drafts       int64
	Archived     int64
	AccountUnread map[string]int64
}

// MailMark 批量标记（nil = 不改）。
type MailMark struct {
	Seen     *bool
	Flagged  *bool
	Archived *bool
}

// MailAttachmentRepo 附件仓储。
// Stability:evolving
type MailAttachmentRepo interface {
	ListMailAttachments(ctx context.Context, messageID string) ([]*MailAttachment, error)
	// GetMailAttachment 装载含 Content；属主隔离经 JOIN messages(user_id)。
	GetMailAttachment(ctx context.Context, id, userID string) (*MailAttachment, error)
	SetMailAttachmentKnowledgeDoc(ctx context.Context, id, docID string) error
}

// OfficeTaskRepo 个人任务仓储（F1 仅邮件转任务写路径 + 直查；列表页随
// 任务中心落位）。
// Stability:evolving
type OfficeTaskRepo interface {
	CreateOfficeTask(ctx context.Context, t *OfficeTask) error
	GetOfficeTask(ctx context.Context, id, userID string) (*OfficeTask, error)
	// CreateTaskFromMailTx 单事务：插入任务 + 回写邮件 task_ref（幂等前置
	// 由 usecase 判空保证；同事务防「任务建而回写丢」的孤儿态）。
	CreateTaskFromMailTx(ctx context.Context, t *OfficeTask, mailID, userID string) error
	// ListOfficeTasks 本人任务按过滤条件取，created_at 倒序限量。
	ListOfficeTasks(ctx context.Context, userID string, filter OfficeTaskFilter, limit int) ([]*OfficeTask, error)
	// SetOfficeTaskStatus 按 id+user_id 改状态；行不存在返回 ErrOfficeTaskNotFound。
	SetOfficeTaskStatus(ctx context.Context, id, userID, status string, at time.Time) error
	// UpdateOfficeTask 按 id+user_id 整行回写可变列（标题、说明、来源、状态、执行对象、
	// 会话、到期、更新时间）；行不存在返回 ErrOfficeTaskNotFound。
	UpdateOfficeTask(ctx context.Context, t *OfficeTask) error
	// DeleteOfficeTask 删任务（附件级联），并清掉指向它的邮件 task_ref。
	DeleteOfficeTask(ctx context.Context, id, userID string) error
	ListOfficeTaskFiles(ctx context.Context, taskID, userID string) ([]*OfficeTaskFile, error)
	CreateOfficeTaskFile(ctx context.Context, f *OfficeTaskFile) error
	// GetOfficeTaskFile 装载含 Content；不存在或不属于 userID 返回 ErrOfficeTaskFileNotFound。
	GetOfficeTaskFile(ctx context.Context, id, userID string) (*OfficeTaskFile, error)
	DeleteOfficeTaskFile(ctx context.Context, id, userID string) error
}

// ============================================================================
// 外部设施窄端口（wire 适配）
// ============================================================================

// MailSecretCrypto 账号密码信封加解密口（E-1，wire 适配 credential 设施）。
// 明文只在同步/IDLE 拨号前瞬态存在于内存；任何路径不得落日志/响应。
// Stability:evolving
type MailSecretCrypto interface {
	EncryptSecret(ctx context.Context, plain string) (string, error)
	DecryptSecret(ctx context.Context, enc string) (string, error)
}

// MailKnowledgeSink 附件入知识库窄口（密级随行：classification 透传至
// 知识层；D6 标签体系落位前由适配层留痕）。
// Stability:evolving
type MailKnowledgeSink interface {
	// IngestAttachment 将附件字节入库为知识文档，返回文档 id。
	IngestAttachment(ctx context.Context, in MailKnowledgeIngest) (docID string, err error)
}

// MailKnowledgeIngest 附件入知识库入参。
type MailKnowledgeIngest struct {
	WorkspaceID    string
	UserID         string
	Filename       string
	ContentType    string
	Content        []byte
	Classification int
	SourceMailID   string // 血缘回链
}

// MailCeoboardSink 摘要作战总台投递窄口（wire 适配 ceoboard store；
// UID 幂等键 aranea:mail-digest-<user>-<date>）。
// Stability:evolving
type MailCeoboardSink interface {
	EmitMailDigest(ctx context.Context, userID, title, body string, occurredAt time.Time) error
}
