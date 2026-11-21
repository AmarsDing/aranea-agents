package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	v1 "aranea-agents/api/kratos/office/v1"
	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
)

// OfficeMailService 邮件域 RPC（E 阶段 W1d；proto 见
// api/kratos/office/v1/mail.proto）。权限由中间件按 proto option 硬判
// （office.mail）；属主/租户边界取自 JWT 主体（禁请求体自报）。
//
// 安全合同：
//   - 账号密码写后即弃——Create/Update 的 password 入参只存活到加密完成，
//     任何出参不含明文/密文（MailAccount proto 无 password 字段，验收 #6）；
//   - IMAP 上游故障统一映 502（OFFICE_MAIL_UPSTREAM），上游原文不进响应
//     （防凭据/邮箱内部结构泄漏），明细在审计；
//   - SendMail（邮件 P1）：只有登录用户在页面上点「发送」才走到这里；智能体没有
//     发信工具。审计只记收件人个数和附件数，不记地址。
type OfficeMailService struct {
	v1.UnimplementedOfficeMailServiceServer
	accounts *office.MailAccountUsecase
	messages *office.MailMessageUsecase
	syncer   *office.MailSyncUsecase
	digest   *office.MailDigestUsecase
}

func NewOfficeMailService(a *office.MailAccountUsecase, m *office.MailMessageUsecase, s *office.MailSyncUsecase, d *office.MailDigestUsecase) *OfficeMailService {
	return &OfficeMailService{accounts: a, messages: m, syncer: s, digest: d}
}

// mailErr 邮件域错误出口：ErrMailUpstream 映 502（apierror 无 BadGateway
// 族，映射收口于此），其余走 apierror 标准映射。
func mailErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, office.ErrMailUpstream) {
		return kerrors.New(http.StatusBadGateway, "OFFICE_MAIL_UPSTREAM", office.ErrMailUpstream.Message)
	}
	return apierror.ToKratos(err)
}

// ---------- 账号 ----------

func mailTLSModeFromProto(m v1.TlsMode) string {
	switch m {
	case v1.TlsMode_TLS_MODE_STARTTLS:
		return office.MailTLSStartTLS
	case v1.TlsMode_TLS_MODE_NONE:
		return office.MailTLSNone
	default:
		return office.MailTLSTLS
	}
}

func mailTLSModeToProto(m string) v1.TlsMode {
	switch m {
	case office.MailTLSStartTLS:
		return v1.TlsMode_TLS_MODE_STARTTLS
	case office.MailTLSNone:
		return v1.TlsMode_TLS_MODE_NONE
	default:
		return v1.TlsMode_TLS_MODE_TLS
	}
}

// smtpTLSFromProto SMTP 加密方式：没指定时留空（按服务商预设或默认补）。
func smtpTLSFromProto(m v1.TlsMode) string {
	if m == v1.TlsMode_TLS_MODE_UNSPECIFIED {
		return ""
	}
	return mailTLSModeFromProto(m)
}

func smtpTLSToProto(m string) v1.TlsMode {
	if m == "" {
		return v1.TlsMode_TLS_MODE_UNSPECIFIED
	}
	return mailTLSModeToProto(m)
}

// mailAccountToProto 出参永不携带 PasswordEnc（proto 无该字段，双保险）。
func mailAccountToProto(a *office.MailAccount) *v1.MailAccount {
	out := &v1.MailAccount{
		Id:          a.ID,
		WorkspaceId: a.WorkspaceID,
		Name:        a.Name,
		Host:        a.Host,
		Port:        int32(a.Port),
		TlsMode:     mailTLSModeToProto(a.TLSMode),
		Username:    a.Username,
		LastUid:     a.LastUID,
		CreatedAt:   timestamppb.New(a.CreatedAt),
		Email:       a.Email,
		Provider:    a.Provider,
		SmtpHost:    a.SMTPHost,
		SmtpPort:    int32(a.SMTPPort),
		SmtpTls:     smtpTLSToProto(a.SMTPTLS),
		Status:      a.Status,
		LastError:   a.LastError,
		Signature:   a.Signature,
		SaveSent:    a.SaveSent,
	}
	if a.LastSyncAt != nil {
		out.LastSyncAt = timestamppb.New(*a.LastSyncAt)
	}
	if a.SyncSince != nil {
		out.SyncSince = timestamppb.New(*a.SyncSince)
	}
	return out
}

func (s *OfficeMailService) ListAccounts(ctx context.Context, _ *emptypb.Empty) (*v1.ListAccountsResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	items, err := s.accounts.List(ctx, userID)
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListAccountsResponse{Items: make([]*v1.MailAccount, 0, len(items))}
	for _, a := range items {
		out.Items = append(out.Items, mailAccountToProto(a))
	}
	return out, nil
}

func (s *OfficeMailService) CreateAccount(ctx context.Context, req *v1.CreateAccountRequest) (*v1.MailAccount, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	a, err := s.accounts.Create(ctx, &office.MailAccount{
		WorkspaceID: ws,
		UserID:      userID,
		Name:        req.GetName(),
		Host:        req.GetHost(),
		Port:        int(req.GetPort()),
		TLSMode:     mailTLSModeFromProto(req.GetTlsMode()),
		Username:    req.GetUsername(),
		Email:       req.GetEmail(),
		Provider:    req.GetProvider(),
		SMTPHost:    req.GetSmtpHost(),
		SMTPPort:    int(req.GetSmtpPort()),
		SMTPTLS:     smtpTLSFromProto(req.GetSmtpTls()),
		SyncSince:   tsPtr(req.GetSyncSince()),
		Signature:   req.GetSignature(),
		SaveSent:    req.GetSaveSent(),
	}, req.GetPassword())
	if err != nil {
		return nil, mailErr(err)
	}
	return mailAccountToProto(a), nil
}

func tsPtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func (s *OfficeMailService) UpdateAccount(ctx context.Context, req *v1.UpdateAccountRequest) (*v1.MailAccount, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	a, err := s.accounts.Update(ctx, req.GetId(), userID, &office.MailAccount{
		Name:     req.GetName(),
		Host:     req.GetHost(),
		Port:     int(req.GetPort()),
		TLSMode:  mailTLSModeFromProto(req.GetTlsMode()),
		Username: req.GetUsername(),
		Email:    req.GetEmail(),
		Provider: req.GetProvider(),
		SMTPHost: req.GetSmtpHost(),
		SMTPPort: int(req.GetSmtpPort()),
		SMTPTLS:  smtpTLSFromProto(req.GetSmtpTls()),
	}, req.GetPassword())
	if err != nil {
		return nil, mailErr(err)
	}
	if req.Signature != nil || req.SaveSent != nil {
		if a, err = s.accounts.UpdateSettings(ctx, req.GetId(), userID, req.Signature, req.SaveSent); err != nil {
			return nil, mailErr(err)
		}
	}
	return mailAccountToProto(a), nil
}

func (s *OfficeMailService) DeleteAccount(ctx context.Context, req *v1.DeleteAccountRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	if err := s.accounts.Delete(ctx, req.GetId(), userID); err != nil {
		return nil, mailErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeMailService) SyncAccount(ctx context.Context, req *v1.SyncAccountRequest) (*v1.SyncAccountResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	// 首次同步常常超过前端等待时间：前端放弃不能把同步一起掐断（否则游标不落、下次从头再下）
	imported, skipped, err := s.syncer.Sync(context.WithoutCancel(ctx), req.GetId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.SyncAccountResponse{ImportedCount: int32(imported), SkippedCount: int32(skipped)}, nil
}

// ---------- 邮件 ----------

func mailMessageToProto(m *office.MailMessage) *v1.MailMessage {
	out := &v1.MailMessage{
		Id:             m.ID,
		AccountId:      m.AccountID,
		Uid:            m.UID,
		Subject:        m.Subject,
		Sender:         m.Sender,
		Snippet:        m.Snippet,
		TaskRef:        m.TaskRef,
		Quadrant:       m.Quadrant,
		HasAttachments: m.HasAttachments,
		CreatedAt:      timestamppb.New(m.CreatedAt),
		To:             m.To,
		Cc:             m.Cc,
		Seen:           m.Seen,
		Flagged:        m.Flagged,
		Archived:       m.ArchivedAt != nil,
		Folder:         m.Folder,
		Trashed:        m.TrashedAt != nil,
		Spam:           m.Spam,
	}
	if m.Labels != "" {
		out.Labels = strings.Split(m.Labels, ",")
	}
	if m.SendAt != nil {
		out.SendAt = timestamppb.New(*m.SendAt)
	}
	if m.ReceivedAt != nil {
		out.ReceivedAt = m.ReceivedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *OfficeMailService) ListMessages(ctx context.Context, req *v1.ListMessagesRequest) (*v1.ListMessagesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	items, total, err := s.messages.Query(ctx, userID, office.MailQuery{
		View: req.GetView(), AccountID: req.GetAccountId(), Keyword: req.GetKeyword(), Label: req.GetLabel(),
		Page: int(req.GetPage()), PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListMessagesResponse{Items: make([]*v1.MailMessage, 0, len(items)), Total: total}
	for _, m := range items {
		out.Items = append(out.Items, mailMessageToProto(m))
	}
	return out, nil
}

func (s *OfficeMailService) GetMessage(ctx context.Context, req *v1.GetMessageRequest) (*v1.MailMessageDetail, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	m, err := s.messages.Get(ctx, req.GetId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.MailMessageDetail{Message: mailMessageToProto(m), Body: m.Body, BodyHtml: m.BodyHTML}, nil
}

func (s *OfficeMailService) DeleteMessage(ctx context.Context, req *v1.DeleteMessageRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	if err := s.messages.Delete(ctx, req.GetId(), userID); err != nil {
		return nil, mailErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeMailService) PurgeMessages(ctx context.Context, req *v1.PurgeMessagesRequest) (*v1.PurgeMessagesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	n, err := s.messages.Purge(ctx, userID, req.GetView(), req.GetIds())
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.PurgeMessagesResponse{Deleted: n}, nil
}

// ---------- 附件 ----------

func (s *OfficeMailService) ListAttachments(ctx context.Context, req *v1.ListAttachmentsRequest) (*v1.ListAttachmentsResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	items, err := s.messages.ListAttachments(ctx, req.GetMessageId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListAttachmentsResponse{Items: make([]*v1.MailAttachment, 0, len(items))}
	for _, a := range items {
		out.Items = append(out.Items, &v1.MailAttachment{
			Id:             a.ID,
			MessageId:      a.MessageID,
			Filename:       a.Filename,
			ContentType:    a.ContentType,
			Size:           a.Size,
			KnowledgeDocId: a.KnowledgeDocID,
		})
	}
	return out, nil
}

// GetAttachment 附件下载（HttpBody 原始字节直通；utf8ResponseEncoder 已有
// HttpBody 特判，不 JSON 包裹）。
func (s *OfficeMailService) GetAttachment(ctx context.Context, req *v1.GetAttachmentRequest) (*httpbody.HttpBody, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	a, err := s.messages.GetAttachment(ctx, req.GetId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &httpbody.HttpBody{ContentType: ct, Data: a.Content}, nil
}

// ---------- .eml 导入导出 / 转任务 / 入知识库 ----------

func (s *OfficeMailService) ImportEml(ctx context.Context, req *v1.ImportEmlRequest) (*v1.MailMessage, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	m, err := s.messages.ImportEml(ctx, userID, ws, req.GetEml())
	if err != nil {
		return nil, mailErr(err)
	}
	return mailMessageToProto(m), nil
}

// ExportEml 补 workbench 缺口（HttpBody message/rfc822 原始字节）。
func (s *OfficeMailService) ExportEml(ctx context.Context, req *v1.ExportEmlRequest) (*httpbody.HttpBody, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	raw, _, err := s.messages.ExportEml(ctx, req.GetId(), userID)
	if err != nil {
		return nil, mailErr(err)
	}
	return &httpbody.HttpBody{ContentType: "message/rfc822", Data: raw}, nil
}

func (s *OfficeMailService) MailToTask(ctx context.Context, req *v1.MailToTaskRequest) (*v1.MailToTaskResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	taskRef, err := s.messages.MailToTask(ctx, req.GetId(), userID, req.GetTitleOverride())
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.MailToTaskResponse{TaskRef: taskRef}, nil
}

func (s *OfficeMailService) AttachmentToKnowledge(ctx context.Context, req *v1.AttachmentToKnowledgeRequest) (*v1.AttachmentToKnowledgeResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	docID, err := s.messages.AttachmentToKnowledge(ctx, req.GetId(), userID, int(req.GetClassification()))
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.AttachmentToKnowledgeResponse{KnowledgeDocId: docID}, nil
}

// ---------- 发信 / 草稿 / 计数 / 标记 / 向导 ----------

func outgoingAttachments(in []*v1.OutgoingAttachment) []office.MailOutgoingAttachment {
	out := make([]office.MailOutgoingAttachment, 0, len(in))
	for _, a := range in {
		out = append(out, office.MailOutgoingAttachment{Filename: a.GetFilename(), ContentType: a.GetContentType(), Content: a.GetContent()})
	}
	return out
}

// SendMail SMTP 发信（邮件 P1）。
func (s *OfficeMailService) SendMail(ctx context.Context, req *v1.SendMailRequest) (*v1.SendMailResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	out := &office.MailOutgoing{
		AccountID: req.GetAccountId(), To: req.GetTo(), Cc: req.GetCc(), Subject: req.GetSubject(), Body: req.GetBody(),
		ReplyToID: req.GetReplyToId(), ForwardOfID: req.GetForwardOfId(), DraftID: req.GetDraftId(),
		Attachments: outgoingAttachments(req.GetAttachments()), BodyHTML: req.GetBodyHtml(),
	}
	// 定时发送：存成带 send_at 的草稿，后台到点发出；返回的是草稿 id
	if at := tsPtr(req.GetSendAt()); at != nil && at.After(time.Now()) {
		out.SendAt = at
		id, err := s.syncer.SaveDraft(ctx, userID, out)
		if err != nil {
			return nil, mailErr(err)
		}
		return &v1.SendMailResponse{SendId: id}, nil
	}
	id, err := s.syncer.Send(ctx, userID, out)
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.SendMailResponse{SendId: id}, nil
}

func (s *OfficeMailService) SaveDraft(ctx context.Context, req *v1.SaveDraftRequest) (*v1.SaveDraftResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	id, err := s.syncer.SaveDraft(ctx, userID, &office.MailOutgoing{
		AccountID: req.GetAccountId(), To: req.GetTo(), Cc: req.GetCc(), Subject: req.GetSubject(), Body: req.GetBody(),
		ReplyToID: req.GetReplyToId(), ForwardOfID: req.GetForwardOfId(), DraftID: req.GetDraftId(),
		Attachments: outgoingAttachments(req.GetAttachments()), BodyHTML: req.GetBodyHtml(),
	})
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.SaveDraftResponse{DraftId: id}, nil
}

func (s *OfficeMailService) GetMailboxSummary(ctx context.Context, _ *emptypb.Empty) (*v1.MailboxSummary, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	c, err := s.messages.Counts(ctx, userID)
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.MailboxSummary{
		InboxUnread: c.InboxUnread, Flagged: c.Flagged, Task: c.Task, Attachments: c.Attachments,
		Sent: c.Sent, Drafts: c.Drafts, Archived: c.Archived, SpamUnread: c.SpamUnread, Trash: c.Trash,
	}
	for id, n := range c.AccountUnread {
		out.Accounts = append(out.Accounts, &v1.MailAccountUnread{AccountId: id, Unread: n})
	}
	for _, l := range c.Labels {
		out.Labels = append(out.Labels, &v1.MailLabelCount{Name: l.Name, Count: l.Count})
	}
	return out, nil
}

// MarkMessages 批量标记；已读 / 星标在后台尽力回写到邮箱服务器（不阻塞响应）。
func (s *OfficeMailService) MarkMessages(ctx context.Context, req *v1.MarkMessagesRequest) (*v1.MarkMessagesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	mark := office.MailMark{Seen: req.Seen, Flagged: req.Flagged, Archived: req.Archived, Trashed: req.Trashed, Spam: req.Spam,
		AddLabel: req.GetAddLabel(), RemoveLabel: req.GetRemoveLabel()}
	n, err := s.messages.Mark(ctx, userID, req.GetIds(), mark)
	if err != nil {
		return nil, mailErr(err)
	}
	if n > 0 && (mark.Seen != nil || mark.Flagged != nil) {
		ids := append([]string(nil), req.GetIds()...)
		go func() {
			pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			s.syncer.PushFlags(pctx, userID, ids, mark)
		}()
	}
	if n > 0 && (mark.Trashed != nil || mark.Spam != nil) {
		ids := append([]string(nil), req.GetIds()...)
		go func() {
			pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			s.syncer.PushMoves(pctx, userID, ids, mark)
		}()
	}
	return &v1.MarkMessagesResponse{Updated: n}, nil
}

// MarkAllRead 视图里的未读全部标已读，已读在后台尽力回写服务器。
func (s *OfficeMailService) MarkAllRead(ctx context.Context, req *v1.MarkAllReadRequest) (*v1.MarkMessagesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	ids, err := s.messages.MarkAllRead(ctx, userID, req.GetView(), req.GetAccountId())
	if err != nil {
		return nil, mailErr(err)
	}
	if len(ids) > 0 {
		seen := true
		go func() {
			pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			s.syncer.PushFlags(pctx, userID, ids, office.MailMark{Seen: &seen})
		}()
	}
	return &v1.MarkMessagesResponse{Updated: int64(len(ids))}, nil
}

func (s *OfficeMailService) ListThread(ctx context.Context, req *v1.ListThreadRequest) (*v1.ListMessagesResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	items, err := s.messages.Thread(ctx, userID, req.GetId())
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListMessagesResponse{Items: make([]*v1.MailMessage, 0, len(items)), Total: int64(len(items))}
	for _, m := range items {
		out.Items = append(out.Items, mailMessageToProto(m))
	}
	return out, nil
}

func (s *OfficeMailService) SuggestContacts(ctx context.Context, req *v1.SuggestContactsRequest) (*v1.SuggestContactsResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	hits, err := s.messages.SuggestContacts(ctx, userID, req.GetQ(), int(req.GetLimit()))
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.SuggestContactsResponse{Items: make([]*v1.MailContact, 0, len(hits))}
	for _, h := range hits {
		out.Items = append(out.Items, &v1.MailContact{Name: h.Name, Address: h.Address, Count: int32(h.Count)})
	}
	return out, nil
}

func (s *OfficeMailService) ListBlockedSenders(ctx context.Context, _ *emptypb.Empty) (*v1.ListBlockedSendersResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	list, err := s.messages.ListBlocked(ctx, userID)
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.ListBlockedSendersResponse{Addresses: list}, nil
}

func (s *OfficeMailService) BlockSender(ctx context.Context, req *v1.BlockSenderRequest) (*v1.BlockSenderResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	n, err := s.messages.Block(ctx, userID, req.GetAddress())
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.BlockSenderResponse{Marked: n}, nil
}

func (s *OfficeMailService) UnblockSender(ctx context.Context, req *v1.BlockSenderRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	if err := s.messages.Unblock(ctx, userID, req.GetAddress()); err != nil {
		return nil, mailErr(err)
	}
	return &emptypb.Empty{}, nil
}

func mailPresetToProto(p *office.MailProviderPreset, how string) *v1.MailProviderInfo {
	if p == nil {
		return &v1.MailProviderInfo{Found: false}
	}
	return &v1.MailProviderInfo{
		Found: true, Key: p.Key, Name: p.Name,
		ImapHost: p.IMAPHost, ImapPort: int32(p.IMAPPort), ImapTls: mailTLSModeToProto(p.IMAPTLS),
		SmtpHost: p.SMTPHost, SmtpPort: int32(p.SMTPPort), SmtpTls: mailTLSModeToProto(p.SMTPTLS),
		Supported: p.Supported, DetectedBy: how,
	}
}

func (s *OfficeMailService) DetectProvider(ctx context.Context, req *v1.DetectProviderRequest) (*v1.MailProviderInfo, error) {
	if _, _, err := officeSubject(ctx); err != nil {
		return nil, mailErr(err)
	}
	p, how := s.syncer.DetectProvider(ctx, req.GetEmail())
	return mailPresetToProto(p, how), nil
}

func (s *OfficeMailService) ListProviders(ctx context.Context, _ *emptypb.Empty) (*v1.ListProvidersResponse, error) {
	if _, _, err := officeSubject(ctx); err != nil {
		return nil, mailErr(err)
	}
	all := office.MailProviderPresets()
	out := &v1.ListProvidersResponse{Items: make([]*v1.MailProviderInfo, 0, len(all))}
	for i := range all {
		out.Items = append(out.Items, mailPresetToProto(&all[i], ""))
	}
	return out, nil
}

// TestAccount 测连接：有 id 测已保存的账号；没有 id 测向导里的配置。
func (s *OfficeMailService) TestAccount(ctx context.Context, req *v1.TestAccountRequest) (*v1.TestAccountResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	var res *office.MailConnTest
	if req.GetId() != "" {
		if res, err = s.syncer.TestSaved(ctx, req.GetId(), userID); err != nil {
			return nil, mailErr(err)
		}
	} else {
		if req.GetHost() == "" || req.GetUsername() == "" || req.GetPassword() == "" {
			return nil, mailErr(office.ErrMailHostRequired)
		}
		mode := mailTLSModeFromProto(req.GetTlsMode())
		port := int(req.GetPort())
		if port <= 0 {
			port = 993
			if mode != office.MailTLSTLS {
				port = 143
			}
		}
		acc := &office.MailAccount{
			Host: req.GetHost(), Port: port, TLSMode: mode, Username: req.GetUsername(),
			SMTPHost: req.GetSmtpHost(), SMTPPort: int(req.GetSmtpPort()), SMTPTLS: smtpTLSFromProto(req.GetSmtpTls()),
		}
		if acc.SMTPHost != "" && acc.SMTPPort <= 0 {
			acc.SMTPPort = 465
			if acc.SMTPTLS == office.MailTLSStartTLS {
				acc.SMTPPort = 587
			}
		}
		if acc.SMTPHost != "" && acc.SMTPTLS == "" {
			acc.SMTPTLS = office.MailTLSTLS
		}
		res = s.syncer.TestConnection(ctx, acc, req.GetPassword())
	}
	return &v1.TestAccountResponse{
		ImapOk: res.IMAPOK, ImapError: res.IMAPErr, InboxCount: int32(res.InboxCount),
		SmtpOk: res.SMTPOK, SmtpError: res.SMTPErr, SmtpSkipped: res.SMTPSkipped,
	}, nil
}

// TriggerDigest 手动触发摘要（验收 #8 入口；since 空=默认 24h 窗口）。
func (s *OfficeMailService) TriggerDigest(ctx context.Context, req *v1.TriggerDigestRequest) (*v1.TriggerDigestResponse, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	var since time.Time
	if req.GetSince() != nil {
		since = req.GetSince().AsTime()
	}
	count, summary, err := s.digest.Trigger(ctx, userID, ws, since)
	if err != nil {
		return nil, mailErr(err)
	}
	return &v1.TriggerDigestResponse{MessageCount: int32(count), Summary: summary}, nil
}
