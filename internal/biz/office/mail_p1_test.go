package office

// 邮件 P1 单测：解析（嵌套 multipart / QP / GBK / HTML 清洗）、按 UID 增量同步、
// 发信组信与留底、草稿带附件、服务商识别、连接测试。

import (
	"context"
	"encoding/base64"
	"errors"
	"mime"
	"net"
	"net/textproto"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// ---------- fake 仓储补齐（邮件 P1 新口） ----------

func mailFolderIs(m *MailMessage, folder string) bool {
	f := m.Folder
	if f == "" {
		f = MailFolderInbox
	}
	return f == folder
}

func (f *mailFakeRepos) UpdateMailAccountStatus(_ context.Context, id, status, lastErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.accounts[id]; ok {
		a.Status, a.LastError = status, lastErr
	}
	return nil
}

func (f *mailFakeRepos) QueryMailMessages(_ context.Context, userID string, q MailQuery) ([]*MailMessage, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var pool []*MailMessage
	for _, m := range f.messages {
		if m.UserID != userID || (q.AccountID != "" && m.AccountID != q.AccountID) {
			continue
		}
		inbox := mailFolderIs(m, MailFolderInbox) && m.ArchivedAt == nil
		ok := false
		switch q.View {
		case MailViewUnread:
			ok = inbox && !m.Seen
		case MailViewSent:
			ok = mailFolderIs(m, MailFolderSent)
		case MailViewDrafts:
			ok = mailFolderIs(m, MailFolderDrafts)
		case MailViewArchived:
			ok = m.ArchivedAt != nil
		case MailViewFlagged:
			ok = m.Flagged && !mailFolderIs(m, MailFolderDrafts)
		default:
			ok = inbox
		}
		if ok && q.Keyword != "" {
			ok = strings.Contains(m.Subject+m.Sender+m.To+m.Cc+m.Body, q.Keyword)
		}
		if ok {
			pool = append(pool, cloneMessage(m))
		}
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
	return pool, int64(len(pool)), nil
}

func (f *mailFakeRepos) CountMailbox(_ context.Context, userID string) (*MailboxCounts, error) {
	return &MailboxCounts{AccountUnread: map[string]int64{}}, nil
}

func (f *mailFakeRepos) MarkMailMessages(_ context.Context, userID string, ids []string, mk MailMark) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	now := time.Now()
	for _, id := range ids {
		m, ok := f.messages[id]
		if !ok || m.UserID != userID {
			continue
		}
		if mk.Seen != nil {
			m.Seen = *mk.Seen
		}
		if mk.Flagged != nil {
			m.Flagged = *mk.Flagged
		}
		if mk.Archived != nil {
			if *mk.Archived {
				m.ArchivedAt = &now
			} else {
				m.ArchivedAt = nil
			}
		}
		n++
	}
	return n, nil
}

func (f *mailFakeRepos) SaveMailDraftTx(_ context.Context, userID, replaceID string, m *MailMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.messages[replaceID]; ok && old.UserID == userID && mailFolderIs(old, MailFolderDrafts) {
		delete(f.messages, replaceID)
		for aid, a := range f.attachments {
			if a.MessageID == replaceID {
				delete(f.attachments, aid)
			}
		}
	}
	f.messages[m.ID] = cloneMessage(m)
	for _, a := range m.Attachments {
		ac := *a
		f.attachments[a.ID] = &ac
	}
	return nil
}

// ---------- fake SMTP ----------

type fakeSMTP struct {
	mu       sync.Mutex
	authErr  error
	sendErr  error
	from     string
	rcpts    []string
	data     []byte
	authUser string
	sent     int
}

func (f *fakeSMTP) dialer() MailSmtpDialer {
	return func(context.Context, string, int, string) (MailSmtpClient, error) { return f, nil }
}
func (f *fakeSMTP) Auth(u, _ string) error { f.authUser = u; return f.authErr }
func (f *fakeSMTP) Send(from string, rcpts []string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.from, f.rcpts, f.data = from, rcpts, data
	f.sent++
	return nil
}
func (f *fakeSMTP) Close() error { return nil }

func newSendFixture(t *testing.T) (context.Context, *mailFakeRepos, *MailSyncUsecase, *fakeSMTP, *MailAccount, *mailFakeAudit) {
	t.Helper()
	ctx := context.Background()
	repo := newMailFakeRepos()
	audit := &mailFakeAudit{}
	acc, err := NewMailAccountUsecase(repo, mailFakeCrypto{}, nil).Create(ctx, &MailAccount{
		WorkspaceID: "ws", UserID: "u-1", Name: "张三", Host: "imap.qq.com", TLSMode: MailTLSTLS,
		Username: "zs@qq.com", Provider: "qq",
	}, "auth-code")
	if err != nil {
		t.Fatal(err)
	}
	smtp := &fakeSMTP{}
	uc := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, audit, nil).WithAttachments(repo)
	uc.smtpDialer = smtp.dialer()
	return ctx, repo, uc, smtp, acc, audit
}

// ---------- 用例 ----------

func TestParseMailRaw_NestedQPAndGBK(t *testing.T) {
	gbk := func(s string) string {
		out, _ := simplifiedchinese.GBK.NewEncoder().String(s)
		return out
	}
	subj := mime.BEncoding.Encode("gbk", gbk("合同第 3 条付款节点"))
	raw := "From: =?utf-8?B?" + "5p2O5Y2T" + "?= <li@partner.cn>\r\n" + // 李卓
		"To: me@company.cn, =?utf-8?B?546L5pWP?= <wm@company.cn>\r\n" + // 王敏
		"Cc: boss@company.cn\r\n" +
		"Subject: " + subj + "\r\n" +
		"Date: Thu, 19 Nov 2026 15:42:00 +0800\r\n" +
		"Message-ID: <abc@partner.cn>\r\n" +
		"References: <r1@x> <r2@x>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=OUT\r\n\r\n" +
		"--OUT\r\nContent-Type: multipart/alternative; boundary=IN\r\n\r\n" +
		"--IN\r\nContent-Type: text/plain; charset=gbk\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		b64(gbk("附件是修订后的合同")) + "\r\n" +
		"--IN\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<p onclick=3D\"x()\">=E9=99=84=E4=BB=B6</p><script>alert(1)</script><img src=3D\"https://t.example/p.gif\">" +
		"<a href=3D\"javascript:alert(1)\">bad</a><a href=3D\"https://ok.example\">ok</a>\r\n" +
		"--IN--\r\n" +
		"--OUT\r\nContent-Type: application/pdf; name=\"" + mime.BEncoding.Encode("utf-8", "对照表.pdf") + "\"\r\n" +
		"Content-Disposition: attachment\r\nContent-Transfer-Encoding: base64\r\n\r\n" + b64("%PDF-1.4") + "\r\n--OUT--\r\n"

	m := ParseMailRaw([]byte(raw))
	if m.Subject != "合同第 3 条付款节点" {
		t.Fatalf("gbk subject: %q", m.Subject)
	}
	if !strings.Contains(m.Sender, "李卓") || !strings.Contains(m.To, "王敏") || m.Cc != "boss@company.cn" {
		t.Fatalf("addresses: from=%q to=%q cc=%q", m.Sender, m.To, m.Cc)
	}
	if m.Body != "附件是修订后的合同" {
		t.Fatalf("gbk base64 body: %q", m.Body)
	}
	if m.MessageID != "<abc@partner.cn>" || m.References != "<r1@x> <r2@x>" {
		t.Fatalf("threading headers: %q %q", m.MessageID, m.References)
	}
	for _, bad := range []string{"<script", "alert(1)", "onclick", "javascript:", ` src="https://t.example`} {
		if strings.Contains(m.HTML, bad) {
			t.Fatalf("html not sanitized (%s): %s", bad, m.HTML)
		}
	}
	for _, good := range []string{"<p>附件</p>", `data-mail-src="https://t.example/p.gif"`, `href="https://ok.example" target="_blank" rel="noopener noreferrer"`} {
		if !strings.Contains(m.HTML, good) {
			t.Fatalf("html missing %q: %s", good, m.HTML)
		}
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "对照表.pdf" || string(m.Attachments[0].Content) != "%PDF-1.4" {
		t.Fatalf("attachment: %+v", m.Attachments)
	}
}

func TestParseMailRaw_HTMLOnlyBodyFromText(t *testing.T) {
	raw := "From: a@b\r\nSubject: hi\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><head><style>p{}</style></head><body><p>第一行</p><p>第二行</p></body></html>"
	m := ParseMailRaw([]byte(raw))
	if m.Body != "第一行\n\n第二行" && m.Body != "第一行\n第二行" {
		t.Fatalf("html-only body: %q", m.Body)
	}
}

// 正文里 cid: 引用的内嵌图片要换成 data:，并且不再列成附件；真正的附件照旧。
func TestParseMailRaw_InlineCIDImage(t *testing.T) {
	raw := "From: a@b\r\nSubject: logo\r\nContent-Type: multipart/mixed; boundary=MX\r\n\r\n" +
		"--MX\r\nContent-Type: multipart/related; boundary=RL\r\n\r\n" +
		"--RL\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>hi</p><img src=\"cid:logo@x\">\r\n" +
		"--RL\r\nContent-Type: image/png; name=logo.png\r\nContent-Id: <logo@x>\r\nContent-Transfer-Encoding: base64\r\n\r\n" + b64("PNGDATA") + "\r\n--RL--\r\n" +
		"--MX\r\nContent-Type: application/pdf; name=rules.pdf\r\nContent-Disposition: attachment; filename=rules.pdf\r\nContent-Transfer-Encoding: base64\r\n\r\n" + b64("%PDF") + "\r\n--MX--\r\n"
	m := ParseMailRaw([]byte(raw))
	if !strings.Contains(m.HTML, `src="data:image/png;base64,`+b64("PNGDATA")) {
		t.Fatalf("cid image not inlined: %q", m.HTML)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Filename != "rules.pdf" {
		t.Fatalf("attachments: %+v", m.Attachments)
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestMailSync_IncrementalByUID(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	syncer := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	if n, _, err := syncer.Sync(ctx, acc.ID, "u-1"); err != nil || n != 3 {
		t.Fatalf("first sync n=%d err=%v", n, err)
	}
	srv.Mailbox().Append([]byte("From: x@y\r\nTo: test@local\r\nCc: c@y\r\nSubject: fourth\r\nContent-Type: text/plain\r\n\r\nhello\r\n"))
	n, skipped, err := syncer.Sync(ctx, acc.ID, "u-1")
	if err != nil || n != 1 || skipped != 0 {
		t.Fatalf("incremental sync n=%d skipped=%d err=%v", n, skipped, err)
	}
	cur, _ := repo.GetMailAccount(ctx, acc.ID, "u-1")
	if cur.LastUID != 4 || cur.Status != MailStatusOK {
		t.Fatalf("cursor=%d status=%q", cur.LastUID, cur.Status)
	}
	msgs, _, _ := repo.QueryMailMessages(ctx, "u-1", MailQuery{View: MailViewUnread})
	var fourth *MailMessage
	for _, m := range msgs {
		if m.Subject == "fourth" {
			fourth = m
		}
	}
	if fourth == nil || fourth.UID != 4 || fourth.Cc != "c@y" || fourth.Seen {
		t.Fatalf("fourth mail: %+v", fourth)
	}
}

func TestMailSend_BuildsMailStoresSentAndDeletesDraft(t *testing.T) {
	ctx, repo, uc, smtp, acc, audit := newSendFixture(t)
	if acc.SMTPHost != "smtp.qq.com" || acc.SMTPPort != 465 || acc.Email != "zs@qq.com" {
		t.Fatalf("smtp defaults from preset: %+v", acc)
	}
	// 被回复的原信
	orig := &MailMessage{ID: "omm_orig", UserID: "u-1", AccountID: acc.ID, UID: 9, Subject: "合同", MessageIDHdr: "<o@p>", ReferencesHdr: "<r0@p>"}
	if _, err := repo.InsertMessageWithAttachmentsTx(ctx, orig); err != nil {
		t.Fatal(err)
	}
	draftID, err := uc.SaveDraft(ctx, "u-1", &MailOutgoing{AccountID: acc.ID, To: "李卓 <li@partner.cn>", Subject: "Re: 合同", Body: "草稿",
		Attachments: []MailOutgoingAttachment{{Filename: "报价.xlsx", ContentType: "application/vnd.ms-excel", Content: []byte("xls")}}})
	if err != nil {
		t.Fatal(err)
	}
	sentID, err := uc.Send(ctx, "u-1", &MailOutgoing{
		AccountID: acc.ID, To: "李卓 <li@partner.cn>；wm@company.cn", Cc: "boss@company.cn",
		Subject: "Re: 合同", Body: "收到，周五前确认。", ReplyToID: orig.ID, DraftID: draftID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if smtp.from != "zs@qq.com" || strings.Join(smtp.rcpts, ",") != "li@partner.cn,wm@company.cn,boss@company.cn" || smtp.authUser != "zs@qq.com" {
		t.Fatalf("envelope: from=%q rcpts=%v auth=%q", smtp.from, smtp.rcpts, smtp.authUser)
	}
	parsed := ParseMailRaw(smtp.data)
	if parsed.Subject != "Re: 合同" || parsed.Body != "收到，周五前确认。" || !strings.Contains(parsed.Sender, "张三") {
		t.Fatalf("round trip: subj=%q body=%q from=%q", parsed.Subject, parsed.Body, parsed.Sender)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].Filename != "报价.xlsx" {
		t.Fatalf("draft attachment not carried: %+v", parsed.Attachments)
	}
	hdr := string(smtp.data)
	if !strings.Contains(hdr, "In-Reply-To: <o@p>") || !strings.Contains(hdr, "References: <r0@p> <o@p>") {
		t.Fatalf("reply headers missing:\n%s", hdr[:400])
	}
	sent, err := repo.GetMailMessage(ctx, sentID, "u-1")
	if err != nil || sent.Folder != MailFolderSent || !sent.Seen || !strings.Contains(sent.To, "li@partner.cn") {
		t.Fatalf("sent copy: %+v err=%v", sent, err)
	}
	if _, err := repo.GetMailMessage(ctx, draftID, "u-1"); !errors.Is(err, ErrMailMessageNotFound) {
		t.Fatalf("draft should be deleted after send, err=%v", err)
	}
	var ok bool
	for _, r := range audit.snapshot() {
		if r.action == "send.mail_message" && strings.Contains(r.summary, "ok=true to=2 cc=1 attachments=1") {
			ok = true
		}
		if strings.Contains(r.summary, "li@partner.cn") {
			t.Fatal("audit must not carry addresses")
		}
	}
	if !ok {
		t.Fatalf("missing send audit: %+v", audit.snapshot())
	}
}

func TestMailSend_Validation(t *testing.T) {
	ctx, _, uc, smtp, acc, _ := newSendFixture(t)
	if _, err := uc.Send(ctx, "u-1", &MailOutgoing{AccountID: acc.ID, Subject: "x"}); !errors.Is(err, ErrMailRecipientRequired) {
		t.Fatalf("no recipient: %v", err)
	}
	if _, err := uc.Send(ctx, "u-1", &MailOutgoing{AccountID: acc.ID, To: "not an address"}); !errors.Is(err, ErrMailRecipientInvalid) {
		t.Fatalf("bad recipient: %v", err)
	}
	smtp.authErr = &textproto.Error{Code: 535, Msg: "auth failed"}
	_, err := uc.Send(ctx, "u-1", &MailOutgoing{AccountID: acc.ID, To: "a@b.cn"})
	if !errors.Is(err, ErrMailUpstream) {
		t.Fatalf("smtp auth failure: %v", err)
	}
	if smtp.sent != 0 {
		t.Fatal("nothing should be sent")
	}
}

func TestDetectMailProvider(t *testing.T) {
	ctx := context.Background()
	if p, how := DetectMailProvider(ctx, "a@163.com", nil); p == nil || p.Key != "163" || how != "domain" {
		t.Fatalf("163: %+v %s", p, how)
	}
	mx := func(_ context.Context, domain string) ([]*net.MX, error) {
		if domain == "partner.cn" {
			return []*net.MX{{Host: "mxbiz1.qq.com.", Pref: 5}}, nil
		}
		return nil, errors.New("no mx")
	}
	if p, how := DetectMailProvider(ctx, "li@partner.cn", mx); p == nil || p.Key != "exmail" || how != "mx" {
		t.Fatalf("exmail via mx: %+v %s", p, how)
	}
	if p, _ := DetectMailProvider(ctx, "x@unknown.cn", mx); p != nil {
		t.Fatalf("unknown should be nil, got %+v", p)
	}
	if p, _ := DetectMailProvider(ctx, "a@outlook.com", nil); p == nil || p.Supported {
		t.Fatalf("outlook must be marked unsupported (OAuth): %+v", p)
	}
}

func TestMailTestConnection_ReportsEachStep(t *testing.T) {
	srv := startMockIMAPServer(t)
	repo := newMailFakeRepos()
	ctx := context.Background()
	acc := newMailTestAccount(t, ctx, repo, srv, "u-1")
	smtp := &fakeSMTP{authErr: &textproto.Error{Code: 535, Msg: "bad"}}
	uc := NewMailSyncUsecase(repo, repo, mailFakeCrypto{}, nil, nil, nil)
	uc.smtpDialer = smtp.dialer()
	acc.SMTPHost, acc.SMTPPort, acc.SMTPTLS = "smtp.local", 465, MailTLSTLS
	res := uc.TestConnection(ctx, acc, "pw")
	if !res.IMAPOK || res.InboxCount != 3 || res.SMTPOK || res.SMTPErr != MailErrKindAuth {
		t.Fatalf("test result: %+v", res)
	}
	acc.Port = 1 // 不可达
	if res := uc.TestConnection(ctx, acc, "pw"); res.IMAPOK || res.IMAPErr != MailErrKindUnreachable {
		t.Fatalf("unreachable: %+v", res)
	}
}
