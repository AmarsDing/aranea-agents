package office

// IMAP 客户端（自 workbench server/imap.go 逐语义平移，E 阶段 W1d）。
//
// 平移保真要点：
//   - tls/starttls/none 三模式：993 直连 TLS / 明文起跳后 STARTTLS 升级 /
//     明文（仅测试 mock）；10s 拨号超时；连接后先读 greeting；
//   - tagged 命令 a%03d 自增；readUntilTagged 自动吃掉 {N} 字面量
//     （N<1<<24=16MB 卡口，与附件上限 E-4 同值）；
//   - fetchRaw 按字面量声明字节数精确截取（RFC822 原文字节级保真）；
//   - parseMailRaw：mime.WordDecoder 解码 encoded-word 主题/发件人，
//     Date 头 RFC1123Z 主格式+备选格式，multipart 递归+base64 解码，
//     text/plain 优先为正文、Content-Disposition/FileName 判附件；
//   - idleWait：IDLE → "+ idling" → 读截止推送（* n EXISTS）→ DONE →
//     tagged OK；超时无推送正常退出（gotNew=false）。
//
// 变更（aranea 化）：
//   - 不直接持 DB——连接/解析纯函数，事务编排归 mail_sync.go；
//   - 口令只在 login() 调用瞬间经内存传递，任何错误路径不回显口令
//     （IMAP 错误响应本身不含口令；上层映射 ErrMailUpstream 防泄漏）。
//
// 注意：UID 列承载 IMAP 消息序号（seq 1..EXISTS，平移 workbench 口径），
// 非 IMAP UID 命令；mock 与真实服务器同序，幂等去重依赖此口径。

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// MailImapMessage 解析后的单封邮件（同步事务的写入单元）。
type MailImapMessage struct {
	UID         int
	Subject     string
	Sender      string
	Date        time.Time
	Body        string
	Attachments []MailImapAttachment
	// 邮件 P1
	To         string
	Cc         string
	HTML       string // 已清洗
	MessageID  string
	References string
	Seen       bool // 服务器上的 \Seen（抓取时带回）
	Flagged    bool // 服务器上的 \Flagged
}

// MailImapAttachment 解析后的附件。
type MailImapAttachment struct {
	Filename    string
	ContentType string
	Content     []byte
	ContentID   string // 正文里 <img src="cid:..."> 引用的内嵌图片（解析时用，不落库）
}

// MailImapClient 是同步/IDLE 消费的最小连接口（测试可注入 mock 直连实现）。
// Stability:evolving
type MailImapClient interface {
	Login(username, password string) error
	// SelectInbox 选 INBOX 并返回 EXISTS 总数。
	SelectInbox() (int, error)
	// FetchRaw 抓取第 seq 封 RFC822 原文。
	FetchRaw(seq int) ([]byte, error)
	// SearchUIDs UID SEARCH（criteria 如 "UID 101:*"、"SINCE 1-Nov-2026"、"ALL"）。
	SearchUIDs(criteria string) ([]int, error)
	// FetchUID 按 IMAP UID 抓原文，并带回 FLAGS 原文（如 `\Seen \Flagged`）。
	FetchUID(uid int) (raw []byte, flags string, err error)
	// StoreFlags 给 UID 加 / 去标记（flag 如 `\Seen`）。
	StoreFlags(uid int, add bool, flag string) error
	// IdleWait IDLE 等待推送；返回 (新邮件总数, 是否有新邮件)。
	IdleWait(timeout time.Duration) (int, bool, error)
	Close()
}

// MailImapDialer 建连工厂（wire 默认 DialMailIMAP；测试注入 mock）。
type MailImapDialer func(ctx context.Context, host string, port int, tlsMode string) (MailImapClient, error)

// errMailIdleUnsupported 服务器拒绝 IDLE（网易 126/163 就不支持），这类账号只靠轮询收信。
var errMailIdleUnsupported = errors.New("imap IDLE refused")

// imapLiteralCap 字面量上限（16MB，E-4 同值）。
const imapLiteralCap = 1 << 24

type mailImapConn struct {
	conn net.Conn
	r    *textproto.Reader
	tag  int
}

// DialMailIMAP 按 tlsMode 建连：tls=直连 TLS / starttls=明文起跳后升级 /
// none=明文（仅测试 mock）。
func DialMailIMAP(_ context.Context, host string, port int, tlsMode string) (MailImapClient, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var conn net.Conn
	var err error
	switch tlsMode {
	case MailTLSTLS:
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, &tls.Config{ServerName: host})
	default: // none / starttls（starttls 在登录前升级）
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}
	if err != nil {
		return nil, err
	}
	c := &mailImapConn{conn: conn, r: textproto.NewReader(bufio.NewReader(conn))}
	if _, err := c.r.ReadLine(); err != nil { // greeting
		_ = conn.Close()
		return nil, fmt.Errorf("imap greeting: %w", err)
	}
	if tlsMode == MailTLSStartTLS {
		if _, err := c.cmd("STARTTLS"); err != nil {
			_ = conn.Close()
			return nil, err
		}
		tconn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tconn.Handshake(); err != nil {
			_ = conn.Close()
			return nil, err
		}
		c.conn = tconn
		c.r = textproto.NewReader(bufio.NewReader(tconn))
	}
	return c, nil
}

// cmd 发送命令并读取到 tagged 响应；期间自动吃掉字面量（拼进返回串）。
func (c *mailImapConn) cmd(command string) (string, error) {
	c.tag++
	tag := fmt.Sprintf("a%03d", c.tag)
	if _, err := fmt.Fprintf(c.conn, "%s %s\r\n", tag, command); err != nil {
		return "", err
	}
	return c.readUntilTagged(tag)
}

func (c *mailImapConn) readUntilTagged(tag string) (string, error) {
	var sb strings.Builder
	for {
		line, err := c.r.ReadLine()
		if err != nil {
			return sb.String(), err
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		// 字面量 {N}：继续读 N 字节（16MB 卡口）
		if idx := strings.LastIndex(line, "{"); idx >= 0 && strings.HasSuffix(line, "}") {
			if n, err := strconv.Atoi(line[idx+1 : len(line)-1]); err == nil && n > 0 && n < imapLiteralCap {
				buf := make([]byte, n)
				if _, err := io.ReadFull(c.r.R, buf); err != nil {
					return sb.String(), err
				}
				sb.Write(buf)
				continue
			}
		}
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(line, " OK") {
				return sb.String(), nil
			}
			return sb.String(), fmt.Errorf("imap command failed: %s", line)
		}
	}
}

func (c *mailImapConn) Login(username, password string) error {
	q := func(s string) string {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	if _, err := c.cmd("LOGIN " + q(username) + " " + q(password)); err != nil {
		return err
	}
	// 网易（163/126/yeah/企业邮）不认没报 ID（RFC 2971）的客户端：登录成功，SELECT 却回
	// "Unsafe Login"。不支持 ID 的服务器回 BAD，忽略即可。
	_, _ = c.cmd(`ID ("name" "aranea" "version" "1.0" "vendor" "aranea")`)
	return nil
}

func (c *mailImapConn) SelectInbox() (int, error) {
	resp, err := c.cmd("SELECT INBOX")
	if err != nil {
		return 0, err
	}
	exists := 0
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "* ") && strings.HasSuffix(line, " EXISTS") {
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, "* "), "%d", &exists)
		}
	}
	return exists, nil
}

// FetchRaw 抓取第 seq 封邮件完整 RFC822 原文（按字面量声明字节数精确截取）。
// 取信一律 BODY.PEEK[]：RFC822 / BODY[] 会让服务器顺带置 \Seen（RFC 3501 §6.4.5），
// 同步一次就把手机上没看的信全变成已读。
func (c *mailImapConn) FetchRaw(seq int) ([]byte, error) {
	resp, err := c.cmd(fmt.Sprintf("FETCH %d (BODY.PEEK[])", seq))
	if err != nil {
		return nil, err
	}
	return imapFetchLiteral(resp)
}

// SearchUIDs 发 UID SEARCH，收 "* SEARCH 1 2 3"。
func (c *mailImapConn) SearchUIDs(criteria string) ([]int, error) {
	resp, err := c.cmd("UID SEARCH " + criteria)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "* SEARCH") {
			continue
		}
		for _, f := range strings.Fields(line[len("* SEARCH"):]) {
			if n, err := strconv.Atoi(f); err == nil {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// FetchUID 按 UID 抓 FLAGS + 原文（BODY.PEEK[]，不改服务器上的已读状态）。
func (c *mailImapConn) FetchUID(uid int) ([]byte, string, error) {
	resp, err := c.cmd(fmt.Sprintf("UID FETCH %d (FLAGS BODY.PEEK[])", uid))
	if err != nil {
		return nil, "", err
	}
	start, n, err := imapLiteralBounds(resp)
	if err != nil {
		return nil, "", err
	}
	// FLAGS 可能在字面量前，也可能在后（服务器不保证顺序）；字面量里的正文不能拿来找
	flags, ok := imapFlagsIn(resp[:start])
	if !ok {
		flags, _ = imapFlagsIn(resp[start+n:])
	}
	return []byte(resp[start : start+n]), flags, nil
}

// StoreFlags UID STORE +/-FLAGS.SILENT。
func (c *mailImapConn) StoreFlags(uid int, add bool, flag string) error {
	op := "-"
	if add {
		op = "+"
	}
	_, err := c.cmd(fmt.Sprintf("UID STORE %d %sFLAGS.SILENT (%s)", uid, op, flag))
	return err
}

// imapFlagsIn 取 s 里 FLAGS (...) 括号中的内容。
func imapFlagsIn(s string) (string, bool) {
	i := strings.Index(strings.ToUpper(s), "FLAGS (")
	if i < 0 {
		return "", false
	}
	rest := s[i+len("FLAGS ("):]
	if j := strings.Index(rest, ")"); j >= 0 {
		return rest[:j], true
	}
	return "", false
}

// imapFetchLiteral 按字面量 {N} 声明的字节数截出邮件原文。
func imapFetchLiteral(resp string) ([]byte, error) {
	start, n, err := imapLiteralBounds(resp)
	if err != nil {
		return nil, err
	}
	return []byte(resp[start : start+n]), nil
}

// imapLiteralBounds 第一个字面量 {N} 的起点与长度。
func imapLiteralBounds(resp string) (int, int, error) {
	idx := strings.Index(resp, "{")
	if idx < 0 {
		return 0, 0, fmt.Errorf("imap FETCH response has no literal")
	}
	end := strings.Index(resp[idx:], "}")
	if end < 0 {
		return 0, 0, fmt.Errorf("imap FETCH literal malformed")
	}
	n, err := strconv.Atoi(resp[idx+1 : idx+end])
	if err != nil {
		return 0, 0, fmt.Errorf("imap FETCH literal length: %w", err)
	}
	nl := strings.Index(resp[idx:], "\n")
	if nl < 0 {
		return 0, 0, fmt.Errorf("imap FETCH response missing newline")
	}
	start := idx + nl + 1
	if start+n > len(resp) {
		return 0, 0, fmt.Errorf("imap FETCH literal truncated: declared %d got %d", n, len(resp)-start)
	}
	return start, n, nil
}

// IdleWait 发送 IDLE 并等待新邮件推送；timeout 到点无推送则正常 DONE 退出。
func (c *mailImapConn) IdleWait(timeout time.Duration) (int, bool, error) {
	c.tag++
	tag := fmt.Sprintf("a%03d", c.tag)
	if _, err := fmt.Fprintf(c.conn, "%s IDLE\r\n", tag); err != nil {
		return 0, false, err
	}
	// 期待 "+ idling"
	line, err := c.r.ReadLine()
	if err != nil {
		return 0, false, err
	}
	if !strings.HasPrefix(line, "+") {
		return 0, false, fmt.Errorf("%w: %s", errMailIdleUnsupported, line)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	gotNew := false
	newCount := 0
	for !gotNew {
		l, err := c.r.ReadLine()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			_ = c.conn.SetReadDeadline(time.Time{})
			return 0, false, err
		}
		if strings.HasPrefix(l, "* ") && strings.HasSuffix(l, " EXISTS") {
			_, _ = fmt.Sscanf(strings.TrimPrefix(l, "* "), "%d", &newCount)
			gotNew = true
		}
	}
	_ = c.conn.SetReadDeadline(time.Time{})
	if _, err := fmt.Fprintf(c.conn, "DONE\r\n"); err != nil {
		return newCount, gotNew, err
	}
	// 读 tagged OK
	for {
		l, err := c.r.ReadLine()
		if err != nil {
			return newCount, gotNew, err
		}
		if strings.HasPrefix(l, tag+" ") {
			break
		}
	}
	return newCount, gotNew, nil
}

func (c *mailImapConn) Close() {
	_, _ = c.cmd("LOGOUT")
	_ = c.conn.Close()
}
