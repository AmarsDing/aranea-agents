package office

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ============================================================================
// MailMessageUsecase —— 邮件阅读/删除/附件/.eml 导入导出/转任务/入知识库
//
// 平移与补齐：
//   - ImportEml：account_id=''（部分唯一索引豁免，导入天然可重入）；
//     解析走 ParseMailRaw 统一路径（附件随同事务入库，比 workbench
//     parseEml 仅正文的语义更强，验收 #3 故障注入同覆盖）；
//   - ExportEml：补 workbench 现状缺口——由库存字段重建 RFC822（附件
//     重组 multipart/mixed + base64）；
//   - MailToTask 契约修正：space 维度由服务端上下文解析（workspace+user
//     不再由前端传 space_id），落 office_tasks；幂等（task_ref 非空返回
//     既有引用）；任务含正文与回链、邮件行 task_ref 回写同事务；
//   - AttachmentToKnowledge：密级随行透传（D6 标签体系落位前由适配层
//     留痕），幂等（knowledge_doc_id 非空返回既有）。
// ============================================================================

// ErrKnowledgeUnavailable 知识库未装配（wire nil 降级，service 映 503）。
var ErrKnowledgeUnavailable = apierror.Unavailable(DomainOffice, "knowledge sink not configured")

// ErrMailClassificationInvalid 密级越界（合法区间 L0-L5；0=未标/沿用默认）。
var ErrMailClassificationInvalid = apierror.BadRequest(DomainOffice, "classification must be between 0 and 5")

// ErrMailEmlEmpty .eml 导入内容为空。
var ErrMailEmlEmpty = apierror.BadRequest(DomainOffice, "eml payload is empty")

// ErrMailViewInvalid 邮件页视图不认识。
var ErrMailViewInvalid = apierror.BadRequest(DomainOffice, "unknown mail view")

// ErrMailTooManyIDs 批量操作一次最多 500 封。
var ErrMailTooManyIDs = apierror.BadRequest(DomainOffice, "too many messages in one batch (max 500)")

// MailMessageUsecase 邮件操作面。
type MailMessageUsecase struct {
	messages    MailMessageRepo
	attachments MailAttachmentRepo
	tasks       OfficeTaskRepo
	knowledge   MailKnowledgeSink // nil=入知识库不可用（503）
	lg          loggateway.Logger
	now         func() time.Time
}

func NewMailMessageUsecase(messages MailMessageRepo, attachments MailAttachmentRepo, tasks OfficeTaskRepo, knowledge MailKnowledgeSink, lg loggateway.Logger) *MailMessageUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MailMessageUsecase{messages: messages, attachments: attachments, tasks: tasks, knowledge: knowledge, lg: lg, now: time.Now}
}

// List 本人邮件分页（accountID 空=全账号聚合）。
func (u *MailMessageUsecase) List(ctx context.Context, userID, accountID, quadrant string, page, pageSize int) ([]*MailMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return u.messages.ListMailMessages(ctx, userID, accountID, quadrant, page, pageSize)
}

// Query 邮件页列表（视图 + 账号 + 关键字）。
func (u *MailMessageUsecase) Query(ctx context.Context, userID string, q MailQuery) ([]*MailMessage, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	switch q.View {
	case "":
		q.View = MailViewInbox
	case MailViewInbox, MailViewUnread, MailViewFlagged, MailViewTask, MailViewAttachments,
		MailViewSent, MailViewDrafts, MailViewArchived:
	default:
		return nil, 0, ErrMailViewInvalid
	}
	q.Keyword = strings.TrimSpace(q.Keyword)
	if len([]rune(q.Keyword)) > 200 {
		q.Keyword = string([]rune(q.Keyword)[:200])
	}
	return u.messages.QueryMailMessages(ctx, userID, q)
}

// Counts 左栏计数。
func (u *MailMessageUsecase) Counts(ctx context.Context, userID string) (*MailboxCounts, error) {
	return u.messages.CountMailbox(ctx, userID)
}

// Mark 批量标已读 / 星标 / 归档（一次最多 500 封）。
func (u *MailMessageUsecase) Mark(ctx context.Context, userID string, ids []string, m MailMark) (int64, error) {
	if len(ids) == 0 || (m.Seen == nil && m.Flagged == nil && m.Archived == nil) {
		return 0, nil
	}
	if len(ids) > 500 {
		return 0, ErrMailTooManyIDs
	}
	return u.messages.MarkMailMessages(ctx, userID, ids, m)
}

func (u *MailMessageUsecase) Get(ctx context.Context, id, userID string) (*MailMessage, error) {
	return u.messages.GetMailMessage(ctx, id, userID)
}

func (u *MailMessageUsecase) Delete(ctx context.Context, id, userID string) error {
	return u.messages.DeleteMailMessage(ctx, id, userID)
}

// Search 本人邮件关键词检索（agent mail_search 工具读口；限量卡口 1-100）。
func (u *MailMessageUsecase) Search(ctx context.Context, userID, query string, limit int) ([]*MailMessage, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return u.messages.SearchMailMessages(ctx, userID, query, limit)
}

// ListAttachments 附件列表（不含字节；属主经 message 归属校验）。
func (u *MailMessageUsecase) ListAttachments(ctx context.Context, messageID, userID string) ([]*MailAttachment, error) {
	if _, err := u.messages.GetMailMessage(ctx, messageID, userID); err != nil {
		return nil, err
	}
	return u.attachments.ListMailAttachments(ctx, messageID)
}

// GetAttachment 附件下载（含字节；属主隔离经 JOIN 在 SQL 内）。
func (u *MailMessageUsecase) GetAttachment(ctx context.Context, id, userID string) (*MailAttachment, error) {
	return u.attachments.GetMailAttachment(ctx, id, userID)
}

// ImportEml .eml 导入（16MB 卡口；account_id=” 语义平移）。
func (u *MailMessageUsecase) ImportEml(ctx context.Context, userID, workspaceID string, raw []byte) (*MailMessage, error) {
	if len(raw) == 0 {
		return nil, ErrMailEmlEmpty
	}
	if len(raw) > MailAttachmentMaxBytes {
		return nil, ErrMailEmlTooLarge
	}
	m := ParseMailRaw(raw)
	now := u.now()
	receivedAt := m.Date
	if receivedAt.IsZero() {
		receivedAt = now
	}
	body := strings.TrimSpace(m.Body)
	msg := &MailMessage{
		ID:          "omm_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		UserID:      userID,
		AccountID:   "", // 导入行不参与 (account_id,uid) 唯一索引
		UID:         0,
		Subject:     m.Subject,
		Sender:      m.Sender,
		Body:        body,
		Snippet:     mailSnippet(body),
		ReceivedAt:  &receivedAt,
		CreatedAt:   now,
		To:            m.To,
		Cc:            m.Cc,
		Seen:          true, // 自己拖进来的，按已读
		Folder:        MailFolderInbox,
		BodyHTML:      m.HTML,
		MessageIDHdr:  m.MessageID,
		ReferencesHdr: m.References,
	}
	for _, a := range m.Attachments {
		msg.Attachments = append(msg.Attachments, &MailAttachment{
			ID:          "omat_" + uuid.NewString(),
			MessageID:   msg.ID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        int64(len(a.Content)),
			Content:     a.Content,
			CreatedAt:   now,
		})
	}
	msg.HasAttachments = len(msg.Attachments) > 0
	if _, err := u.messages.InsertMessageWithAttachmentsTx(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// ExportEml 由库存字段重建 RFC822（补 workbench 缺口；附件重组 multipart）。
// 返回 (eml 字节, 建议文件名)。
func (u *MailMessageUsecase) ExportEml(ctx context.Context, id, userID string) ([]byte, string, error) {
	msg, err := u.messages.GetMailMessage(ctx, id, userID)
	if err != nil {
		return nil, "", err
	}
	atts, err := u.attachments.ListMailAttachments(ctx, id)
	if err != nil {
		return nil, "", err
	}
	// 附件字节按需装载（列表不含 content）。
	full := make([]*MailAttachment, 0, len(atts))
	for _, a := range atts {
		fa, err := u.attachments.GetMailAttachment(ctx, a.ID, userID)
		if err != nil {
			return nil, "", err
		}
		full = append(full, fa)
	}
	filename := strings.TrimSpace(msg.Subject)
	if filename == "" {
		filename = msg.ID
	}
	return buildEml(msg, full), filename + ".eml", nil
}

// buildEml 重建 RFC822：encoded-word 头；无附件直出正文，有附件
// multipart/mixed（text/plain 首段 + base64 附件段）。
func buildEml(msg *MailMessage, atts []*MailAttachment) []byte {
	enc := mime.QEncoding
	var head strings.Builder
	head.WriteString("Subject: " + enc.Encode("utf-8", msg.Subject) + "\r\n")
	head.WriteString("From: " + enc.Encode("utf-8", msg.Sender) + "\r\n")
	date := msg.ReceivedAt
	if date == nil {
		now := msg.CreatedAt
		date = &now
	}
	head.WriteString("Date: " + date.UTC().Format(time.RFC1123Z) + "\r\n")
	head.WriteString("MIME-Version: 1.0\r\n")

	if len(atts) == 0 {
		head.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		head.WriteString(msg.Body)
		return []byte(head.String())
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	head.WriteString("Content-Type: multipart/mixed; boundary=\"" + mw.Boundary() + "\"\r\n\r\n")

	tp, err := mw.CreatePart(map[string][]string{
		"Content-Type": {"text/plain; charset=utf-8"},
	})
	if err == nil {
		_, _ = tp.Write([]byte(msg.Body))
	}
	for _, a := range atts {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ap, err := mw.CreatePart(map[string][]string{
			"Content-Type":              {ct + "; name=\"" + a.Filename + "\""},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition":       {"attachment; filename=\"" + a.Filename + "\""},
		})
		if err != nil {
			continue
		}
		w := base64.NewEncoder(base64.StdEncoding, ap)
		_, _ = w.Write(a.Content)
		_ = w.Close()
	}
	_ = mw.Close()
	return append([]byte(head.String()), body.Bytes()...)
}

// MailToTask 邮件转任务（契约修正；幂等；事务内建任务+回写 task_ref）。
func (u *MailMessageUsecase) MailToTask(ctx context.Context, id, userID, titleOverride string) (string, error) {
	msg, err := u.messages.GetMailMessage(ctx, id, userID)
	if err != nil {
		return "", err
	}
	if msg.TaskRef != "" {
		return msg.TaskRef, nil // 幂等：已转任务返回既有引用
	}
	title := strings.TrimSpace(titleOverride)
	if title == "" {
		title = "邮件：" + msg.Subject
	}
	now := u.now()
	task := &OfficeTask{
		ID:          "otk_" + uuid.NewString(),
		WorkspaceID: msg.WorkspaceID,
		UserID:      userID,
		Title:       title,
		Body:        "来自邮件：" + msg.ID + "\n\n" + msg.Body,
		Source:      "mail",
		SourceRef:   msg.ID,
		Status:      "open",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := u.tasks.CreateTaskFromMailTx(ctx, task, msg.ID, userID); err != nil {
		return "", err
	}
	return task.ID, nil
}

// AttachmentToKnowledge 附件入知识库（密级随行；幂等）。
func (u *MailMessageUsecase) AttachmentToKnowledge(ctx context.Context, id, userID string, classification int) (string, error) {
	if classification < 0 || classification > 5 {
		return "", ErrMailClassificationInvalid
	}
	att, err := u.attachments.GetMailAttachment(ctx, id, userID)
	if err != nil {
		return "", err
	}
	if att.KnowledgeDocID != "" {
		return att.KnowledgeDocID, nil // 幂等
	}
	if u.knowledge == nil {
		return "", ErrKnowledgeUnavailable
	}
	msg, err := u.messages.GetMailMessage(ctx, att.MessageID, userID)
	if err != nil {
		return "", err
	}
	docID, err := u.knowledge.IngestAttachment(ctx, MailKnowledgeIngest{
		WorkspaceID:    msg.WorkspaceID,
		UserID:         userID,
		Filename:       att.Filename,
		ContentType:    att.ContentType,
		Content:        att.Content,
		Classification: classification,
		SourceMailID:   att.MessageID,
	})
	if err != nil {
		return "", fmt.Errorf("ingest attachment to knowledge: %w", err)
	}
	if err := u.attachments.SetMailAttachmentKnowledgeDoc(ctx, id, docID); err != nil {
		return "", err
	}
	return docID, nil
}
