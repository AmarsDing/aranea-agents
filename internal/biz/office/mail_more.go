package office

// 邮件第二轮补齐（2026-11-20，对照 126 / Gmail）：
//   - 删除 / 恢复 / 举报 / 不是垃圾邮件尽力回写到服务器（按 Message-ID 在源文件夹里找，UID MOVE）；
//     移回收件箱的信下次同步会以新 UID 出现，入库时按 Message-ID 去重（见仓储 InsertMessageWithAttachmentsTx）；
//   - 发出的信按账户设置 APPEND 一份到服务器「已发送」；
//   - 定时发送：到点的定时草稿由后台发出；「已删除」放满 30 天自动清掉；
//   - 会话（同一主题的往来）、收件人联想、全部标已读、拒收名单、标签。

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// MailTrashRetention「已删除」里放多久自动清掉（同 Gmail）。
const MailTrashRetention = 30 * 24 * time.Hour

// ErrMailLabelInvalid 标签名不合法。
var ErrMailLabelInvalid = apierror.BadRequest(DomainOffice, "label must be 1-20 characters without commas")

// ErrMailSignatureTooLong 签名最长 2000 字。
var ErrMailSignatureTooLong = apierror.BadRequest(DomainOffice, "signature is too long (max 2000 characters)")

// ErrMailAddressInvalid 拒收名单里的地址不合法。
var ErrMailAddressInvalid = apierror.BadRequest(DomainOffice, "invalid email address")

// ---------- 服务器文件夹：已删除 / 已发送 ----------

var (
	trashMailboxNames = map[string]bool{
		"trash": true, "deleted": true, "deleted messages": true, "deleted items": true,
		"已删除": true, "已删除邮件": true, "废件箱": true,
	}
	sentMailboxNames = map[string]bool{
		"sent": true, "sent messages": true, "sent items": true, "sent mail": true,
		"已发送": true, "已发送邮件": true,
	}
)

// findMailbox 先认 RFC 6154 属性，再按名字（解码修改版 UTF-7、取最后一段）认。
func findMailbox(boxes []MailImapMailbox, attr string, names map[string]bool) (string, bool) {
	for _, mb := range boxes {
		for _, a := range strings.Fields(mb.Attrs) {
			if strings.EqualFold(a, attr) {
				return mb.Name, true
			}
		}
	}
	for _, mb := range boxes {
		if strings.Contains(strings.ToLower(mb.Attrs), `\noselect`) {
			continue
		}
		n := strings.ToLower(decodeIMAPUTF7(mb.Name))
		segs := strings.FieldsFunc(n, func(r rune) bool { return r == '/' || r == '.' })
		if names[n] || (len(segs) > 0 && names[segs[len(segs)-1]]) {
			return mb.Name, true
		}
	}
	return "", false
}

// imapConnFor 解密口令、建连登录。
func (u *MailSyncUsecase) imapConnFor(ctx context.Context, acc *MailAccount) (MailImapClient, error) {
	password, err := u.crypto.DecryptSecret(ctx, acc.PasswordEnc)
	if err != nil {
		return nil, err
	}
	cli, _, err := u.mailConnect(ctx, acc, password)
	return cli, err
}

// appendSentCopy 发出的信存一份到服务器「已发送」（尽力而为，失败只记日志）。
func (u *MailSyncUsecase) appendSentCopy(ctx context.Context, acc *MailAccount, password string, raw []byte) {
	cli, _, err := u.mailConnect(ctx, acc, password)
	if err != nil {
		u.lg.Warn("mail save sent: connect failed", loggateway.Str("account_id", acc.ID), loggateway.Err(err))
		return
	}
	defer cli.Close()
	boxes, err := cli.ListMailboxes()
	if err != nil {
		u.lg.Warn("mail save sent: list failed", loggateway.Str("account_id", acc.ID), loggateway.Err(err))
		return
	}
	name, ok := findMailbox(boxes, `\Sent`, sentMailboxNames)
	if !ok {
		u.lg.Warn("mail save sent: no sent folder", loggateway.Str("account_id", acc.ID))
		return
	}
	if err := cli.Append(name, `\Seen`, raw); err != nil {
		u.lg.Warn("mail save sent: append failed", loggateway.Str("account_id", acc.ID), loggateway.Err(err))
	}
}

// PushMoves 删除 / 恢复 / 举报 / 不是垃圾邮件回写到服务器（尽力而为：失败只记日志，本地照旧）。
// 只管有账户、有 Message-ID 的信；按 Message-ID 在源文件夹里找（UID 在信被别处挪过之后不可靠）。
func (u *MailSyncUsecase) PushMoves(ctx context.Context, userID string, ids []string, mark MailMark) {
	if mark.Trashed == nil && mark.Spam == nil {
		return
	}
	type move struct{ msgID, from, to string }
	byAccount := map[string][]move{}
	for _, id := range ids {
		m, err := u.messages.GetMailMessage(ctx, id, userID)
		if err != nil || m.AccountID == "" || m.MessageIDHdr == "" || m.Folder == MailFolderSent || m.Folder == MailFolderDrafts {
			continue
		}
		// 本地标记已经改过了，从动作推出服务器上从哪挪到哪（垃圾邮件在服务器垃圾箱里，其余在收件箱）
		home := "inbox"
		if m.Spam {
			home = "junk"
		}
		var mv move
		switch {
		case mark.Trashed != nil && *mark.Trashed:
			mv = move{m.MessageIDHdr, home, "trash"}
		case mark.Trashed != nil:
			mv = move{m.MessageIDHdr, "trash", home}
		case *mark.Spam:
			mv = move{m.MessageIDHdr, "inbox", "junk"}
		default:
			mv = move{m.MessageIDHdr, "junk", "inbox"}
		}
		byAccount[m.AccountID] = append(byAccount[m.AccountID], mv)
	}
	for accID, moves := range byAccount {
		acc, err := u.accounts.GetMailAccount(ctx, accID, userID)
		if err != nil {
			continue
		}
		cli, err := u.imapConnFor(ctx, acc)
		if err != nil {
			u.lg.Warn("mail move push: connect failed", loggateway.Str("account_id", accID), loggateway.Err(err))
			continue
		}
		boxes, err := cli.ListMailboxes()
		if err != nil {
			cli.Close()
			continue
		}
		names := map[string]string{"inbox": "INBOX"}
		if n, ok := findJunkMailbox(boxes); ok {
			names["junk"] = n
		}
		if n, ok := findMailbox(boxes, `\Trash`, trashMailboxNames); ok {
			names["trash"] = n
		}
		for _, mv := range moves {
			if err := moveByMessageID(cli, names[mv.from], names[mv.to], mv.msgID); err != nil {
				u.lg.Warn("mail move push failed", loggateway.Str("account_id", accID),
					loggateway.Str("from", mv.from), loggateway.Str("to", mv.to), loggateway.Err(err))
			}
		}
		cli.Close()
	}
}

// moveByMessageID 在 from 里按 Message-ID 找到信，UID MOVE 到 to。文件夹缺一个就算了。
func moveByMessageID(cli MailImapClient, from, to, msgID string) error {
	if from == "" || to == "" {
		return nil
	}
	if _, err := cli.Select(from); err != nil {
		return fmt.Errorf("select %s: %w", from, err)
	}
	uids, err := cli.SearchUIDs(`HEADER Message-ID ` + imapQuote(msgID))
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	for _, uid := range uids {
		if err := cli.UIDMove(uid, to); err != nil {
			return fmt.Errorf("move uid %d: %w", uid, err)
		}
	}
	return nil
}

// SendDueDrafts 发出到点的定时草稿。失败的退回普通草稿（不每分钟重试一次地骚扰服务器），记审计。
func (u *MailSyncUsecase) SendDueDrafts(ctx context.Context) {
	due, err := u.messages.ListDueScheduledDrafts(ctx, u.now(), 20)
	if err != nil {
		u.lg.Warn("mail scheduled send: scan failed", loggateway.Err(err))
		return
	}
	for _, d := range due {
		_, err := u.Send(ctx, d.UserID, &MailOutgoing{
			AccountID: d.AccountID, To: d.To, Cc: d.Cc, Subject: d.Subject, Body: d.Body, BodyHTML: d.BodyHTML,
			DraftID: d.ID, References: d.ReferencesHdr,
		})
		if err != nil {
			u.lg.Warn("mail scheduled send failed", loggateway.Str("draft_id", d.ID), loggateway.Err(err))
			u.recordAuditCtx(ctx, "send.mail_scheduled", d.AccountID, "ok=false draft="+d.ID+" err="+mailErrSummary(err))
			if err := u.messages.ClearDraftSendAt(ctx, d.ID); err != nil {
				u.lg.Warn("mail scheduled send: clear send_at failed", loggateway.Err(err))
			}
		}
	}
}

// PurgeOldTrash 清掉在「已删除」里放满 30 天的信。
func (u *MailSyncUsecase) PurgeOldTrash(ctx context.Context) {
	if n, err := u.messages.PurgeTrashedBefore(ctx, u.now().Add(-MailTrashRetention)); err != nil {
		u.lg.Warn("mail trash purge failed", loggateway.Err(err))
	} else if n > 0 {
		u.lg.Info("mail trash purged", loggateway.Int64("deleted", n))
	}
}

// blockedSet 拒收名单（小写地址）。读不到就当空的，不挡同步。
func (u *MailSyncUsecase) blockedSet(ctx context.Context, userID string) map[string]bool {
	list, err := u.messages.ListBlockedSenders(ctx, userID)
	if err != nil {
		u.lg.Warn("mail blocked list read failed", loggateway.Err(err))
		return nil
	}
	out := make(map[string]bool, len(list))
	for _, a := range list {
		out[a] = true
	}
	return out
}

// senderAddr 发件人原文里的地址（小写）；解析不了时取尖括号里的或整串。
func senderAddr(s string) string {
	if a, err := mail.ParseAddress(s); err == nil {
		return strings.ToLower(a.Address)
	}
	if i, j := strings.LastIndex(s, "<"), strings.LastIndex(s, ">"); i >= 0 && j > i {
		return strings.ToLower(strings.TrimSpace(s[i+1 : j]))
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// ---------- MailMessageUsecase：会话 / 联想 / 全部已读 / 拒收 / 标签 ----------

var subjectPrefix = regexp.MustCompile(`(?i)^\s*((re|fw|fwd|aw|回复|答复|转发)\s*[:：]\s*)+`)

// threadKey 会话键：主题去掉回复 / 转发前缀、去两端空白、小写。
func threadKey(subject string) string {
	return strings.ToLower(strings.TrimSpace(subjectPrefix.ReplaceAllString(subject, "")))
}

// Thread 同一会话的往来（主题去前缀后相同）。主题为空时只有它自己。
func (u *MailMessageUsecase) Thread(ctx context.Context, userID, id string) ([]*MailMessage, error) {
	m, err := u.messages.GetMailMessage(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	key := threadKey(m.Subject)
	if key == "" {
		m.Body, m.BodyHTML = "", ""
		return []*MailMessage{m}, nil
	}
	return u.messages.ListThread(ctx, userID, key, 50)
}

// MailContactHit 联想结果。
type MailContactHit struct {
	Name    string
	Address string
	Count   int
}

// SuggestContacts 从最近 2000 封来往里统计地址，按出现次数排；q 匹配名字或地址。
func (u *MailMessageUsecase) SuggestContacts(ctx context.Context, userID, q string, limit int) ([]MailContactHit, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	fields, err := u.messages.ListRecentAddressFields(ctx, userID, 2000)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	byAddr := map[string]*MailContactHit{}
	for _, f := range fields {
		addrs, err := parseMailAddrs(f)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			key := strings.ToLower(a.Address)
			h := byAddr[key]
			if h == nil {
				h = &MailContactHit{Address: a.Address}
				byAddr[key] = h
			}
			if h.Name == "" && a.Name != "" {
				h.Name = a.Name
			}
			h.Count++
		}
	}
	out := make([]MailContactHit, 0, len(byAddr))
	for _, h := range byAddr {
		if q == "" || strings.Contains(strings.ToLower(h.Address), q) || strings.Contains(strings.ToLower(h.Name), q) {
			out = append(out, *h)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Address < out[j].Address
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkAllRead 视图（可限定账户）里的未读全部标已读，返回改到的 id（调用方回写服务器）。
func (u *MailMessageUsecase) MarkAllRead(ctx context.Context, userID, view, accountID string) ([]string, error) {
	if view == "" {
		view = MailViewInbox
	}
	switch view {
	case MailViewInbox, MailViewUnread, MailViewFlagged, MailViewTask, MailViewAttachments, MailViewArchived, MailViewSpam:
	default:
		return nil, ErrMailViewInvalid
	}
	return u.messages.MarkViewSeen(ctx, userID, view, accountID)
}

func (u *MailMessageUsecase) ListBlocked(ctx context.Context, userID string) ([]string, error) {
	return u.messages.ListBlockedSenders(ctx, userID)
}

// Block 拒收：名单里加上，并把这个发件人已有的收件箱邮件标成垃圾邮件。
func (u *MailMessageUsecase) Block(ctx context.Context, userID, address string) (int64, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(address))
	if err != nil {
		return 0, ErrMailAddressInvalid
	}
	return u.messages.BlockSender(ctx, userID, strings.ToLower(a.Address))
}

func (u *MailMessageUsecase) Unblock(ctx context.Context, userID, address string) error {
	return u.messages.UnblockSender(ctx, userID, strings.ToLower(strings.TrimSpace(address)))
}

// validLabel 标签：1-20 字，不含逗号。
func validLabel(s string) bool {
	n := len([]rune(s))
	return n >= 1 && n <= 20 && !strings.Contains(s, ",") && strings.TrimSpace(s) == s
}
