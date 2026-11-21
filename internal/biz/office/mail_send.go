package office

// 邮件 P1：发信、存草稿、测试连接、已读 / 星标回写服务器（09-邮件.md §3.1–§3.2、§4）。
//
// 发信只由用户本人在页面上点「发送」触发（服务层要求登录用户 + office.mail 权限）；
// 智能体没有发信工具，P2 的「AI 起草回复」也只产草稿（§3.3）。

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// MailConnTest 测试连接结果（每一步一个结论，错误只给类别）。
type MailConnTest struct {
	IMAPOK      bool
	IMAPErr     string // MailErrKind*
	InboxCount  int
	SMTPOK      bool
	SMTPErr     string
	SMTPSkipped bool // 没配 SMTP
}

// DetectProvider 向导第 1 步：按邮箱地址认服务商。
func (u *MailSyncUsecase) DetectProvider(ctx context.Context, email string) (*MailProviderPreset, string) {
	return DetectMailProvider(ctx, email, u.mxLookup)
}

// TestConnection 测收信（登录 + 打开收件箱数封数）和发信（登录）两条连接。
// acc 可以是还没保存的配置；password 只在这次调用的内存里。
func (u *MailSyncUsecase) TestConnection(ctx context.Context, acc *MailAccount, password string) *MailConnTest {
	out := &MailConnTest{}
	// 向导只给错误类别；服务器原话（不含口令）记日志，排查「添加失败」靠它
	lg := u.lg.With(loggateway.Str("imap_host", acc.Host), loggateway.Str("smtp_host", acc.SMTPHost))
	if cli, kind, err := u.mailConnect(ctx, acc, password); err != nil {
		out.IMAPErr = kind
		lg.Warn("mail test: imap connect failed", loggateway.Str("kind", kind), loggateway.Err(err))
	} else {
		if n, err := cli.SelectInbox(); err != nil {
			out.IMAPErr = MailErrKindProtocol
			lg.Warn("mail test: imap select failed", loggateway.Err(err))
		} else {
			out.IMAPOK, out.InboxCount = true, n
		}
		cli.Close()
	}
	if strings.TrimSpace(acc.SMTPHost) == "" {
		out.SMTPSkipped = true
		return out
	}
	sc, err := u.smtpDialer(ctx, acc.SMTPHost, acc.SMTPPort, acc.SMTPTLS)
	if err != nil {
		out.SMTPErr = classifyMailDialErr(err)
		lg.Warn("mail test: smtp connect failed", loggateway.Str("kind", out.SMTPErr), loggateway.Err(err))
		return out
	}
	defer func() { _ = sc.Close() }()
	if err := sc.Auth(acc.Username, password); err != nil {
		out.SMTPErr = classifySMTPErr(err)
		lg.Warn("mail test: smtp auth failed", loggateway.Str("kind", out.SMTPErr), loggateway.Err(err))
		return out
	}
	out.SMTPOK = true
	return out
}

// TestSaved 测一个已保存的账号，并把结果记成账号状态。
func (u *MailSyncUsecase) TestSaved(ctx context.Context, id, userID string) (*MailConnTest, error) {
	acc, err := u.accounts.GetMailAccount(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	password, err := u.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return nil, ErrMailUpstream.WithCause(fmt.Errorf("decrypt account password: %w", err))
	}
	res := u.TestConnection(ctx, acc, password)
	kind := res.IMAPErr
	if kind == "" {
		kind = res.SMTPErr
	}
	u.markAccountStatus(ctx, acc, kind)
	return res, nil
}

// prepareOutgoing 校验收件人、取发件地址、把回复 / 转发的上下文带上。
func (u *MailSyncUsecase) prepareOutgoing(ctx context.Context, userID string, out *MailOutgoing, requireTo bool) (*MailAccount, *mail.Address, []*mail.Address, []*mail.Address, *MailMessage, error) {
	acc, err := u.accounts.GetMailAccount(ctx, out.AccountID, userID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	fromAddr := strings.TrimSpace(acc.Email)
	if fromAddr == "" && strings.Contains(acc.Username, "@") {
		fromAddr = acc.Username
	}
	if fromAddr == "" {
		return nil, nil, nil, nil, nil, ErrMailNoFromAddress
	}
	to, err := parseMailAddrs(out.To)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cc, err := parseMailAddrs(out.Cc)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if requireTo && len(to) == 0 {
		return nil, nil, nil, nil, nil, ErrMailRecipientRequired
	}
	var orig *MailMessage
	if id := out.ReplyToID; id != "" {
		if orig, err = u.messages.GetMailMessage(ctx, id, userID); err != nil {
			return nil, nil, nil, nil, nil, err
		}
	}
	total := len(out.Body)
	for _, a := range out.Attachments {
		if len(a.Content) > MailAttachmentMaxBytes {
			return nil, nil, nil, nil, nil, ErrMailAttachmentTooLarge
		}
		total += len(a.Content)
	}
	if total > mailOutgoingMaxBytes {
		return nil, nil, nil, nil, nil, ErrMailOutgoingTooLarge
	}
	return acc, &mail.Address{Name: acc.Name, Address: fromAddr}, to, cc, orig, nil
}

// carryAttachments 把已有的附件带上：从草稿继续（草稿里存过的附件，含当初转发带上的），
// 或者转发（原信的附件）。有草稿时以草稿为准，避免转发附件带两份。
// shortcut: 草稿里已有的附件 P1 不能单个去掉，要去掉就删草稿重写。
func (u *MailSyncUsecase) carryAttachments(ctx context.Context, userID string, out *MailOutgoing) error {
	src := out.ForwardOfID
	if out.DraftID != "" {
		src = out.DraftID
	}
	if src == "" || u.attachments == nil {
		return nil
	}
	if _, err := u.messages.GetMailMessage(ctx, src, userID); err != nil {
		return err
	}
	metas, err := u.attachments.ListMailAttachments(ctx, src)
	if err != nil {
		return err
	}
	for _, m := range metas {
		full, err := u.attachments.GetMailAttachment(ctx, m.ID, userID)
		if err != nil {
			return err
		}
		out.Attachments = append(out.Attachments, MailOutgoingAttachment{Filename: full.Filename, ContentType: full.ContentType, Content: full.Content})
	}
	return nil
}

func replyHeaders(orig *MailMessage) (inReplyTo, references string) {
	if orig == nil || orig.MessageIDHdr == "" {
		return "", ""
	}
	refs := strings.TrimSpace(orig.ReferencesHdr + " " + orig.MessageIDHdr)
	return orig.MessageIDHdr, refs
}

// Send 发一封信：SMTP 发出 → 存一份到「已发送」→ 删掉对应草稿 → 审计。
// 返回已发送那一行的 id。
func (u *MailSyncUsecase) Send(ctx context.Context, userID string, out *MailOutgoing) (string, error) {
	if err := u.carryAttachments(ctx, userID, out); err != nil {
		return "", err
	}
	acc, from, to, cc, orig, err := u.prepareOutgoing(ctx, userID, out, true)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(acc.SMTPHost) == "" || acc.SMTPPort <= 0 {
		return "", ErrMailSMTPNotConfigured
	}
	password, err := u.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return "", ErrMailUpstream.WithCause(fmt.Errorf("decrypt account password: %w", err))
	}
	now := u.now()
	messageID := fmt.Sprintf("<%s@%s>", uuid.NewString(), mailDomainOf(from.Address))
	inReplyTo, references := replyHeaders(orig)
	if orig == nil && strings.TrimSpace(out.References) != "" {
		// 定时草稿到点发出：原信不一定还在，用草稿里存的 References，最后一个就是回复的那封
		references = strings.TrimSpace(out.References)
		refs := strings.Fields(references)
		inReplyTo = refs[len(refs)-1]
	}
	raw := buildOutgoingMail(from, to, cc, out.Subject, out.Body, out.BodyHTML, messageID, inReplyTo, references, out.Attachments, now)

	rcpts := make([]string, 0, len(to)+len(cc))
	for _, a := range append(append([]*mail.Address{}, to...), cc...) {
		rcpts = append(rcpts, a.Address)
	}
	sendErr := func() error {
		sc, err := u.smtpDialer(ctx, acc.SMTPHost, acc.SMTPPort, acc.SMTPTLS)
		if err != nil {
			return mailKindErr{kind: classifyMailDialErr(err), err: err}
		}
		defer func() { _ = sc.Close() }()
		if err := sc.Auth(acc.Username, password); err != nil {
			return mailKindErr{kind: classifySMTPErr(err), err: err}
		}
		if err := sc.Send(from.Address, rcpts, raw); err != nil {
			return mailKindErr{kind: classifySMTPErr(err), err: err}
		}
		return nil
	}()
	summary := fmt.Sprintf("to=%d cc=%d attachments=%d", len(to), len(cc), len(out.Attachments))
	if sendErr != nil {
		kind := MailErrKindProtocol
		if ke, ok := sendErr.(mailKindErr); ok {
			kind = ke.kind
		}
		u.recordAuditCtx(ctx, "send.mail_message", acc.ID, "ok=false "+summary+" err="+kind)
		return "", ErrMailUpstream.WithCause(fmt.Errorf("smtp send (%s): %w", kind, sendErr))
	}

	if acc.SaveSent {
		u.appendSentCopy(ctx, acc, password, raw)
	}
	sent := u.outgoingRow(acc, userID, from, to, cc, out, now)
	sent.Folder = MailFolderSent
	sent.MessageIDHdr = messageID
	sent.ReferencesHdr = references
	if _, err := u.messages.InsertMessageWithAttachmentsTx(ctx, sent); err != nil {
		// 信已经发出去了，只是本地没留底：记审计，不当失败返回（避免用户重发）。
		u.lg.Warn("mail sent but saving the sent copy failed", loggateway.Err(err))
	}
	if out.DraftID != "" {
		if err := u.messages.DeleteMailMessage(ctx, out.DraftID, userID); err != nil {
			u.lg.Warn("mail sent but deleting the draft failed", loggateway.Err(err))
		}
	}
	u.recordAuditCtx(ctx, "send.mail_message", acc.ID, "ok=true "+summary+" sent="+sent.ID)
	return sent.ID, nil
}

// SaveDraft 存草稿（替换同 id 的旧草稿）。收件人可以为空。
func (u *MailSyncUsecase) SaveDraft(ctx context.Context, userID string, out *MailOutgoing) (string, error) {
	if err := u.carryAttachments(ctx, userID, out); err != nil {
		return "", err
	}
	acc, from, to, cc, orig, err := u.prepareOutgoing(ctx, userID, out, false)
	if err != nil {
		return "", err
	}
	draft := u.outgoingRow(acc, userID, from, to, cc, out, u.now())
	draft.Folder = MailFolderDrafts
	_, draft.ReferencesHdr = replyHeaders(orig)
	if draft.ReferencesHdr == "" {
		draft.ReferencesHdr = strings.TrimSpace(out.References)
	}
	if out.SendAt != nil && out.SendAt.After(u.now()) {
		if len(to) == 0 {
			return "", ErrMailRecipientRequired
		}
		at := out.SendAt.UTC()
		draft.SendAt = &at
	}
	if err := u.messages.SaveMailDraftTx(ctx, userID, out.DraftID, draft); err != nil {
		return "", err
	}
	return draft.ID, nil
}

func (u *MailSyncUsecase) outgoingRow(acc *MailAccount, userID string, from *mail.Address, to, cc []*mail.Address, out *MailOutgoing, now time.Time) *MailMessage {
	body := strings.TrimSpace(out.Body)
	m := &MailMessage{
		ID:          "omm_" + uuid.NewString(),
		WorkspaceID: acc.WorkspaceID,
		UserID:      userID,
		AccountID:   acc.ID,
		Subject:     out.Subject,
		Sender:      displayMailAddrs([]*mail.Address{from}),
		Body:        body,
		Snippet:     mailSnippet(body),
		ReceivedAt:  &now,
		CreatedAt:   now,
		To:          displayMailAddrs(to),
		Cc:          displayMailAddrs(cc),
		Seen:        true,
	}
	if strings.TrimSpace(out.BodyHTML) != "" {
		m.BodyHTML = sanitizeMailHTML(out.BodyHTML)
	}
	for _, a := range out.Attachments {
		m.Attachments = append(m.Attachments, &MailAttachment{
			ID: "omat_" + uuid.NewString(), MessageID: m.ID, Filename: a.Filename,
			ContentType: a.ContentType, Size: int64(len(a.Content)), Content: a.Content, CreatedAt: now,
		})
	}
	m.HasAttachments = len(m.Attachments) > 0
	return m
}

type mailKindErr struct {
	kind string
	err  error
}

func (e mailKindErr) Error() string { return e.err.Error() }
func (e mailKindErr) Unwrap() error { return e.err }

// PushFlags 把已读 / 星标回写到服务器（尽力而为：失败只记日志，本地标记照旧）。
// 只管从收件箱同步来的信（有账号、有 UID）。
func (u *MailSyncUsecase) PushFlags(ctx context.Context, userID string, ids []string, mark MailMark) {
	if mark.Seen == nil && mark.Flagged == nil {
		return
	}
	byAccount := map[string][]int{}
	for _, id := range ids {
		m, err := u.messages.GetMailMessage(ctx, id, userID)
		if err != nil || m.AccountID == "" || m.UID <= 0 || (m.Folder != "" && m.Folder != MailFolderInbox) {
			continue
		}
		byAccount[m.AccountID] = append(byAccount[m.AccountID], int(m.UID))
	}
	for accID, uids := range byAccount {
		if err := u.pushFlagsOne(ctx, accID, userID, uids, mark); err != nil {
			u.lg.Warn("mail flag push failed", loggateway.Err(err))
		}
	}
}

func (u *MailSyncUsecase) pushFlagsOne(ctx context.Context, accID, userID string, uids []int, mark MailMark) error {
	acc, err := u.accounts.GetMailAccount(ctx, accID, userID)
	if err != nil {
		return err
	}
	password, err := u.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return err
	}
	cli, _, err := u.mailConnect(ctx, acc, password)
	if err != nil {
		return err
	}
	defer cli.Close()
	if _, err := cli.SelectInbox(); err != nil {
		return err
	}
	for _, uid := range uids {
		if mark.Seen != nil {
			if err := cli.StoreFlags(uid, *mark.Seen, `\Seen`); err != nil {
				return err
			}
		}
		if mark.Flagged != nil {
			if err := cli.StoreFlags(uid, *mark.Flagged, `\Flagged`); err != nil {
				return err
			}
		}
	}
	return nil
}
