package service

import (
	"context"
	"errors"
	"net/http"
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
//   - SendMail 为 F2 占位（E-2 高风险闸同批），当前一律 412 拒绝。
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
	}
	if a.LastSyncAt != nil {
		out.LastSyncAt = timestamppb.New(*a.LastSyncAt)
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
	}, req.GetPassword())
	if err != nil {
		return nil, mailErr(err)
	}
	return mailAccountToProto(a), nil
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
	}, req.GetPassword())
	if err != nil {
		return nil, mailErr(err)
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
	imported, skipped, err := s.syncer.Sync(ctx, req.GetId(), userID)
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
	items, total, err := s.messages.List(ctx, userID, req.GetAccountId(), req.GetQuadrant(), int(req.GetPage()), int(req.GetPageSize()))
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
	return &v1.MailMessageDetail{Message: mailMessageToProto(m), Body: m.Body}, nil
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

// ---------- 发信（F2 占位） / 摘要 ----------

// SendMail F2 占位（E-2：SMTP 发信随高风险闸同批）：一律 412 拒绝，诚实不装。
func (s *OfficeMailService) SendMail(_ context.Context, _ *v1.SendMailRequest) (*v1.SendMailResponse, error) {
	return nil, apierror.ToKratos(apierror.FailedPrecondition(office.DomainOffice, "mail send is an F2 item pending the high-risk approval gate"))
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
