package office

// SMTP 发信（邮件 P1，09-邮件.md §4「实现 SendMail：SMTP 发送 + 存进已发送 + 审计」）。
// 用标准库 net/smtp：465 直连 TLS / 587 STARTTLS / none（只给测试）；
// 认证优先 PLAIN，服务器只给 LOGIN 时用 LOGIN（部分国内企业邮只开 LOGIN）。

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/apierror"
)

// MailSmtpClient 发信的最小连接口（测试注入假实现）。
// Stability:evolving
type MailSmtpClient interface {
	Auth(username, password string) error
	Send(from string, rcpts []string, data []byte) error
	Close() error
}

// MailSmtpDialer 建连工厂（默认 DialMailSMTP）。
type MailSmtpDialer func(ctx context.Context, host string, port int, tlsMode string) (MailSmtpClient, error)

type smtpConn struct {
	c    *smtp.Client
	host string
	tls  bool
}

// DialMailSMTP 按 tlsMode 建连：tls=465 直连 TLS；starttls=明文起跳后升级；none=明文（仅测试）。
func DialMailSMTP(ctx context.Context, host string, port int, tlsMode string) (MailSmtpClient, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if tlsMode == MailTLSTLS {
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	sc := &smtpConn{c: c, host: host, tls: tlsMode == MailTLSTLS}
	if tlsMode == MailTLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			_ = c.Close()
			return nil, errors.New("smtp server does not offer STARTTLS")
		}
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			_ = c.Close()
			return nil, err
		}
		sc.tls = true
	}
	return sc, nil
}

func (s *smtpConn) Auth(username, password string) error {
	ok, mechs := s.c.Extension("AUTH")
	if !ok {
		return nil // 服务器不要求认证
	}
	if strings.Contains(strings.ToUpper(mechs), "PLAIN") {
		return s.c.Auth(smtp.PlainAuth("", username, password, s.host))
	}
	return s.c.Auth(&smtpLoginAuth{username: username, password: password})
}

func (s *smtpConn) Send(from string, rcpts []string, data []byte) error {
	if err := s.c.Mail(from); err != nil {
		return err
	}
	for _, r := range rcpts {
		if err := s.c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := s.c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (s *smtpConn) Close() error {
	_ = s.c.Quit()
	return s.c.Close()
}

// smtpLoginAuth AUTH LOGIN（net/smtp 只自带 PLAIN / CRAM-MD5）。只在 TLS 连接上用。
type smtpLoginAuth struct{ username, password string }

func (a *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("refusing AUTH LOGIN over an unencrypted connection")
	}
	return "LOGIN", nil, nil
}

func (a *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(string(fromServer))
	switch {
	case strings.Contains(prompt, "user"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "pass"):
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected AUTH LOGIN prompt")
}

// ---------- 组信 ----------

// MailOutgoing 一封要发出的信（或要存的草稿）。
type MailOutgoing struct {
	AccountID   string
	To          string // 逗号 / 分号分隔，可带名字：王敏 <wm@x.cn>
	Cc          string
	Subject     string
	Body        string // 纯文本
	ReplyToID   string // 回复的本地邮件 id（带 In-Reply-To / References）
	ForwardOfID string // 转发的本地邮件 id（带上原附件）
	DraftID     string // 从草稿发出时，发完删掉草稿
	Attachments []MailOutgoingAttachment
}

// MailOutgoingAttachment 随信附件。
type MailOutgoingAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// 发信上限：单个附件沿用 16MB；整封信 25MB（多数国内邮箱 SMTP 的上限在 50MB，留余量）。
const mailOutgoingMaxBytes = 25 << 20

var (
	ErrMailRecipientRequired = apierror.BadRequest(DomainOffice, "at least one recipient is required")
	ErrMailRecipientInvalid  = apierror.BadRequest(DomainOffice, "recipient address is invalid")
	ErrMailNoFromAddress     = apierror.BadRequest(DomainOffice, "account has no sender address; set its email first")
	ErrMailOutgoingTooLarge  = apierror.BadRequest(DomainOffice, "message with attachments exceeds 25MB")
	ErrMailSMTPNotConfigured = apierror.BadRequest(DomainOffice, "account has no SMTP server configured")
)

// parseMailAddrs 解析收件人列表（允许中文逗号 / 分号）。
func parseMailAddrs(list string) ([]*mail.Address, error) {
	list = strings.NewReplacer("；", ",", ";", ",", "，", ",").Replace(list)
	var parts []string
	for _, p := range strings.Split(list, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]*mail.Address, 0, len(parts))
	for _, p := range parts {
		a, err := mail.ParseAddress(p)
		if err != nil {
			return nil, ErrMailRecipientInvalid
		}
		out = append(out, a)
	}
	return out, nil
}

func joinMailAddrs(addrs []*mail.Address) string {
	s := make([]string, len(addrs))
	for i, a := range addrs {
		s[i] = a.String()
	}
	return strings.Join(s, ", ")
}

func displayMailAddrs(addrs []*mail.Address) string {
	s := make([]string, len(addrs))
	for i, a := range addrs {
		if a.Name != "" {
			s[i] = a.Name + " <" + a.Address + ">"
		} else {
			s[i] = a.Address
		}
	}
	return strings.Join(s, ", ")
}

// buildOutgoingMail 组 RFC822：纯文本正文 base64；有附件时 multipart/mixed。
// 附件名用带引号的 encoded-word（国内客户端认这个）。
func buildOutgoingMail(from *mail.Address, to, cc []*mail.Address, subject, body, messageID, inReplyTo, references string, atts []MailOutgoingAttachment, now time.Time) []byte {
	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", from.String())
	h("To", joinMailAddrs(to))
	if len(cc) > 0 {
		h("Cc", joinMailAddrs(cc))
	}
	h("Subject", mime.BEncoding.Encode("utf-8", subject))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", messageID)
	if inReplyTo != "" {
		h("In-Reply-To", inReplyTo)
	}
	if references != "" {
		h("References", references)
	}
	h("MIME-Version", "1.0")
	textPart := func() {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		writeWrappedBase64(&b, []byte(body))
	}
	if len(atts) == 0 {
		textPart()
		return b.Bytes()
	}
	boundary := "aranea-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	h("Content-Type", `multipart/mixed; boundary="`+boundary+`"`)
	b.WriteString("\r\n--" + boundary + "\r\n")
	textPart()
	for _, a := range atts {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		name := mime.BEncoding.Encode("utf-8", a.Filename)
		b.WriteString("\r\n--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ct + `; name="` + name + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		b.WriteString(`Content-Disposition: attachment; filename="` + name + "\"\r\n\r\n")
		writeWrappedBase64(&b, a.Content)
	}
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return b.Bytes()
}

// writeWrappedBase64 base64 按 76 列折行（RFC 2045）。
func writeWrappedBase64(b *bytes.Buffer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	if enc != "" {
		b.WriteString(enc + "\r\n")
	}
}

func mailDomainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

// classifySMTPErr 发信错误归类（认证 535 / 534 / 530 → auth）。
func classifySMTPErr(err error) string {
	var te *textproto.Error
	if errors.As(err, &te) {
		if te.Code == 535 || te.Code == 534 || te.Code == 530 {
			return MailErrKindAuth
		}
		return MailErrKindProtocol
	}
	return classifyMailDialErr(err)
}
