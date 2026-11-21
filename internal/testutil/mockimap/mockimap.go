// Package mockimap 提供进程内 mock IMAP4rev1 服务端（E 阶段 W1d 邮件域
// 测试与冒烟证据用；移植自 workbench server/mockimap.go，语义逐行保持）。
//
// 支持：CAPABILITY / LOGIN / SELECT / LIST / SEARCH / FETCH (RFC822) /
// 垃圾邮件文件夹（网易账号叫「垃圾邮件」且不带 \Junk，其他账号叫 Junk 带 \Junk，预播一封广告）/
// UID FETCH / APPEND（模拟新邮件送达）/ IDLE（真实推送 * n EXISTS）/
// ID / NOOP / LOGOUT。账号任意非空即可登录；口令含 "wrong" 时拒绝（测登录失败），
// 用户名是 @126.com / @163.com 时学网易：没报 ID 就 SELECT，回 "Unsafe Login"。
//
// 除 IMAP 协议口外提供 HTTP 控制口（EnableControl）：POST /inject
// （body=原始 .eml 字节 → 追加邮箱并触发 IDLE 推送，供 bash 冒烟经
// curl 注入新邮件）、GET /count（当前邮件数）。控制口仅监听回环。
package mockimap

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Mailbox 线程安全邮箱（序号从 1 起，与 IMAP message sequence 对齐）。
type Mailbox struct {
	mu      sync.Mutex
	msgs    [][]byte // 被挪走 / 清掉的位置是 nil（UID 不复用，和真实服务器一样）
	subs    map[chan int]struct{}
	seen    map[int]bool // 学真实服务器：取 RFC822 / BODY[]（非 PEEK）会顺带置 \Seen
	deleted map[int]bool // 标了 \Deleted、等 EXPUNGE 的
}

// Live 还在的信数（不含挪走 / 清掉的）。
func (m *Mailbox) Live() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, raw := range m.msgs {
		if raw != nil {
			n++
		}
	}
	return n
}

// remove 挪走一封（位置留空，UID 不复用）。
func (m *Mailbox) remove(uid int) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if uid < 1 || uid > len(m.msgs) {
		return nil
	}
	raw := m.msgs[uid-1]
	m.msgs[uid-1] = nil
	return raw
}

func (m *Mailbox) markDeleted(uid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleted == nil {
		m.deleted = map[int]bool{}
	}
	m.deleted[uid] = true
}

func (m *Mailbox) expunge() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for uid := range m.deleted {
		if uid >= 1 && uid <= len(m.msgs) {
			m.msgs[uid-1] = nil
		}
	}
	m.deleted = nil
}

// search UID SEARCH：「UID a:*」（照真实服务器：没有 >=a 的也回最大那个）、
// 「HEADER Message-ID <x>」、其他条件一律全部。只回还在的。
func (m *Mailbox) search(args string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := strings.Fields(args)
	var ids []string
	if len(f) >= 3 && strings.EqualFold(f[0], "HEADER") && strings.EqualFold(f[1], "Message-ID") {
		want := strings.Trim(strings.Join(f[2:], " "), `"`)
		for i, raw := range m.msgs {
			if raw != nil && strings.Contains(string(raw), "Message-ID: "+want) {
				ids = append(ids, strconv.Itoa(i+1))
			}
		}
		return ids
	}
	lo := 1
	if len(f) == 2 && strings.EqualFold(f[0], "UID") {
		if a, _, ok := strings.Cut(f[1], ":"); ok {
			if v, err := strconv.Atoi(a); err == nil {
				lo = v
			}
		}
	}
	last := 0
	for i, raw := range m.msgs {
		if raw == nil {
			continue
		}
		last = i + 1
		if i+1 >= lo {
			ids = append(ids, strconv.Itoa(i+1))
		}
	}
	if len(ids) == 0 && last > 0 {
		ids = append(ids, strconv.Itoa(last))
	}
	return ids
}

// Seen 第 i 封是否带 \Seen。
func (m *Mailbox) Seen(i int) bool { m.mu.Lock(); defer m.mu.Unlock(); return m.seen[i] }

func (m *Mailbox) setSeen(i int, v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen == nil {
		m.seen = map[int]bool{}
	}
	m.seen[i] = v
}

// NewMailbox 建仓并播三封种子邮件：纯文本 / 带 base64 附件（budget.txt）/
// 中文主题（encoded-word）。
func NewMailbox() *Mailbox {
	m := &Mailbox{subs: map[chan int]struct{}{}}
	m.msgs = append(m.msgs, SeedMails()...)
	return m
}

// SeedMails 三封种子邮件原始字节（encoded-word 中文主题 + base64 附件，
// 覆盖验收 #1/#2/#3 证据面）。
func SeedMails() [][]byte {
	encSubject := mime.BEncoding.Encode("utf-8", "项目资金方案与排期表")
	attContent := base64.StdEncoding.EncodeToString([]byte("项目资金方案 v1：总预算 80 万，凤凰计划三期专款。"))
	return [][]byte{
		[]byte("From: alice@example.com\r\nMessage-ID: <seed-1@mock.local>\r\nTo: test@local\r\nSubject: Weekly sync notes\r\nDate: Mon, 02 Nov 2026 09:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n本周同步会议纪要：目标对齐、下周计划。\r\n"),
		[]byte("From: boss@example.com\r\nMessage-ID: <seed-2@mock.local>\r\nTo: test@local\r\nSubject: " + encSubject + "\r\nDate: Mon, 02 Nov 2026 10:30:00 +0800\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=MOCKBB\r\n\r\n--MOCKBB\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n请查收项目资金方案，详见附件 budget.txt。\r\n\r\n--MOCKBB\r\nContent-Type: text/plain; name=budget.txt\r\nContent-Disposition: attachment; filename=budget.txt\r\nContent-Transfer-Encoding: base64\r\n\r\n" + attContent + "\r\n\r\n--MOCKBB--\r\n"),
		[]byte("From: hr@example.com\r\nMessage-ID: <seed-3@mock.local>\r\nTo: test@local\r\nSubject: " + mime.BEncoding.Encode("utf-8", "团建报名截止提醒") + "\r\nDate: Mon, 02 Nov 2026 11:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n周五团建报名今天截止，请尽快填写表单。\r\n"),
	}
}

// Count 当前邮件数。
func (m *Mailbox) Count() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.msgs) }

func (m *Mailbox) get(i int) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 1 || i > len(m.msgs) {
		return nil
	}
	return m.msgs[i-1]
}

// Append 追加一封原始邮件并广播 IDLE 订阅者（返回新序号）。
func (m *Mailbox) Append(raw []byte) int {
	m.mu.Lock()
	m.msgs = append(m.msgs, raw)
	n := len(m.msgs)
	for ch := range m.subs {
		select {
		case ch <- n:
		default:
		}
	}
	m.mu.Unlock()
	return n
}

func (m *Mailbox) subscribe() chan int {
	ch := make(chan int, 4)
	m.mu.Lock()
	m.subs[ch] = struct{}{}
	m.mu.Unlock()
	return ch
}

func (m *Mailbox) unsubscribe(ch chan int) {
	m.mu.Lock()
	delete(m.subs, ch)
	m.mu.Unlock()
}

// 网易文件夹名（修改版 UTF-7）：垃圾邮件 / 已删除 / 已发送。
const (
	JunkNameNetEase  = "&V4NXPpCuTvY-"
	TrashNameNetEase = "&XfJSIJZk-"
	SentNameNetEase  = "&XfJT0ZAB-"
)

// SeedJunk 垃圾邮件文件夹的种子：一封广告。
func SeedJunk() [][]byte {
	return [][]byte{
		[]byte("From: promo@spam.example\r\nMessage-ID: <junk-1@mock.local>\r\nTo: test@local\r\nSubject: " + mime.BEncoding.Encode("utf-8", "恭喜中奖，点此领取") + "\r\nDate: Mon, 02 Nov 2026 08:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n限时领取，过期作废。\r\n"),
	}
}

// Server mock IMAP 服务端（IMAP 协议口 + 可选 HTTP 控制口）。
type Server struct {
	mb    *Mailbox
	junk  *Mailbox
	trash *Mailbox
	sent  *Mailbox
	ln    net.Listener
	ctlLn net.Listener
	done  chan struct{}
}

// Listen 在 imapAddr 起 IMAP 协议口（"127.0.0.1:0" 随机端口可用，Addr()
// 取实际地址）。邮箱预播三封种子邮件。
func Listen(imapAddr string) (*Server, error) {
	ln, err := net.Listen("tcp", imapAddr)
	if err != nil {
		return nil, fmt.Errorf("mockimap listen %s: %w", imapAddr, err)
	}
	junk := &Mailbox{subs: map[chan int]struct{}{}, msgs: SeedJunk()}
	s := &Server{mb: NewMailbox(), junk: junk, trash: &Mailbox{subs: map[chan int]struct{}{}},
		sent: &Mailbox{subs: map[chan int]struct{}{}}, ln: ln, done: make(chan struct{})}
	go s.acceptLoop()
	return s, nil
}

// Addr IMAP 协议口实际地址。
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Mailbox 暴露邮箱（测试直接 Append 注入；冒烟走控制口 /inject）。
func (s *Server) Mailbox() *Mailbox { return s.mb }

// Junk 暴露垃圾邮件文件夹。
func (s *Server) Junk() *Mailbox { return s.junk }

// Trash 暴露已删除文件夹。
func (s *Server) Trash() *Mailbox { return s.trash }

// Sent 暴露已发送文件夹。
func (s *Server) Sent() *Mailbox { return s.sent }

// EnableControl 起 HTTP 控制口（回环）：POST /inject（raw eml → Append）、
// GET /count。返回实际地址。
func (s *Server) EnableControl(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("mockimap control listen %s: %w", addr, err)
	}
	s.ctlLn = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/inject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<24))
		if err != nil || len(body) == 0 {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}
		box := s.mb
		if r.URL.Query().Get("folder") == "junk" {
			box = s.junk
		}
		n := box.Append(body)
		_, _ = fmt.Fprintf(w, `{"seq":%d}`, n)
	})
	mux.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"count":%d}`, s.mb.Count())
	})
	go func() { _ = http.Serve(ln, mux) }()
	return ln.Addr().String(), nil
}

// Close 关停（连接中的会话随 read 失败退出）。
func (s *Server) Close() error {
	close(s.done)
	if s.ctlLn != nil {
		_ = s.ctlLn.Close()
	}
	return s.ln.Close()
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn, s)
	}
}

func imapWrite(w *bufio.Writer, s string) {
	_, _ = w.WriteString(s + "\r\n")
	_ = w.Flush()
}

// box 按名字找文件夹（网易名、通用名都认）。
func (s *Server) box(name string) *Mailbox {
	switch strings.Trim(strings.TrimSpace(name), `"`) {
	case "INBOX", "inbox", "Inbox":
		return s.mb
	case "Junk", JunkNameNetEase:
		return s.junk
	case "Trash", TrashNameNetEase:
		return s.trash
	case "Sent", SentNameNetEase:
		return s.sent
	}
	return nil
}

func handleConn(conn net.Conn, s *Server) {
	defer conn.Close()
	mb := s.mb
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	imapWrite(w, "* OK MockIMAP4rev1 ready")
	selected := false
	netease, sentID := false, false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			imapWrite(w, "* BAD empty command")
			continue
		}
		tag, rest := line[:sp], strings.TrimSpace(line[sp+1:])
		sp2 := strings.IndexByte(rest, ' ')
		var verb, args string
		if sp2 < 0 {
			verb, args = strings.ToUpper(rest), ""
		} else {
			verb, args = strings.ToUpper(rest[:sp2]), rest[sp2+1:]
		}
		switch verb {
		case "CAPABILITY":
			imapWrite(w, "* CAPABILITY IMAP4rev1 IDLE")
			imapWrite(w, tag+" OK CAPABILITY done")
		case "LOGIN":
			if strings.TrimSpace(args) == "" {
				imapWrite(w, tag+" BAD empty credentials")
				continue
			}
			if strings.Contains(args, "wrong") {
				imapWrite(w, tag+" NO [AUTHENTICATIONFAILED] Invalid credentials")
				continue
			}
			netease = strings.Contains(args, "@126.com") || strings.Contains(args, "@163.com")
			imapWrite(w, tag+" OK LOGIN completed")
		case "ID":
			sentID = true
			imapWrite(w, `* ID ("name" "MockIMAP")`)
			imapWrite(w, tag+" OK ID completed")
		case "SELECT", "EXAMINE":
			if netease && !sentID {
				imapWrite(w, tag+" NO SELECT Unsafe Login. Please contact kefu@188.com for help")
				continue
			}
			box := s.box(args)
			if box == nil {
				imapWrite(w, tag+" NO [NONEXISTENT] no such mailbox")
				continue
			}
			mb = box
			imapWrite(w, fmt.Sprintf("* %d EXISTS", mb.Count()))
			imapWrite(w, "* OK [READ-WRITE] SELECT completed")
			imapWrite(w, tag+" OK SELECT completed")
			selected = true
		case "LIST":
			imapWrite(w, `* LIST (\HasNoChildren) "/" "INBOX"`)
			if netease {
				imapWrite(w, `* LIST () "/" "`+JunkNameNetEase+`"`)
				imapWrite(w, `* LIST () "/" "`+TrashNameNetEase+`"`)
				imapWrite(w, `* LIST () "/" "`+SentNameNetEase+`"`)
			} else {
				imapWrite(w, `* LIST (\HasNoChildren \Junk) "/" "Junk"`)
				imapWrite(w, `* LIST (\HasNoChildren \Trash) "/" "Trash"`)
				imapWrite(w, `* LIST (\HasNoChildren \Sent) "/" "Sent"`)
			}
			imapWrite(w, tag+" OK LIST completed")
		case "SEARCH":
			imapWrite(w, "* SEARCH "+strings.Join(mb.search("ALL"), " "))
			imapWrite(w, tag+" OK SEARCH completed")
		case "FETCH", "UID":
			fetchCmd := verb
			if verb == "UID" {
				sp3 := strings.IndexByte(args, ' ')
				if sp3 < 0 {
					imapWrite(w, tag+" BAD UID needs subcommand")
					continue
				}
				fetchCmd = strings.ToUpper(args[:sp3])
				args = strings.TrimSpace(args[sp3+1:])
			}
			// UID 与序号相同（mock 从不删信）。
			if verb == "UID" && fetchCmd == "SEARCH" {
				imapWrite(w, "* SEARCH "+strings.Join(mb.search(args), " "))
				imapWrite(w, tag+" OK SEARCH completed")
				continue
			}
			if verb == "UID" && fetchCmd == "STORE" {
				// "<uid> +FLAGS.SILENT (\Seen)"：记 \Seen 和 \Deleted
				if f := strings.Fields(args); len(f) >= 3 {
					if uid, err := strconv.Atoi(f[0]); err == nil {
						if strings.Contains(args, `\Seen`) {
							mb.setSeen(uid, strings.HasPrefix(f[1], "+"))
						}
						if strings.Contains(args, `\Deleted`) && strings.HasPrefix(f[1], "+") {
							mb.markDeleted(uid)
						}
					}
				}
				imapWrite(w, tag+" OK STORE completed")
				continue
			}
			if verb == "UID" && (fetchCmd == "MOVE" || fetchCmd == "COPY") {
				// "<uid> <mailbox>"
				f := strings.SplitN(args, " ", 2)
				uid, err := strconv.Atoi(f[0])
				var dst *Mailbox
				if len(f) == 2 {
					dst = s.box(f[1])
				}
				if err != nil || dst == nil || mb.get(uid) == nil {
					imapWrite(w, tag+" NO [TRYCREATE] no such message or mailbox")
					continue
				}
				raw := mb.get(uid)
				if fetchCmd == "MOVE" {
					mb.remove(uid)
				}
				dst.Append(raw)
				imapWrite(w, tag+" OK "+fetchCmd+" completed")
				continue
			}
			if fetchCmd != "FETCH" {
				imapWrite(w, tag+" BAD unsupported UID subcommand")
				continue
			}
			if !selected {
				imapWrite(w, tag+" BAD no mailbox selected")
				continue
			}
			// args: <seq> (<items>)
			sp4 := strings.IndexByte(args, ' ')
			seqStr := args
			if sp4 > 0 {
				seqStr = args[:sp4]
			}
			seq, err := strconv.Atoi(strings.TrimSpace(seqStr))
			if err != nil || mb.get(seq) == nil {
				imapWrite(w, tag+" NO no such message")
				continue
			}
			raw := mb.get(seq)
			if !strings.Contains(strings.ToUpper(args), "PEEK") {
				mb.setSeen(seq, true)
			}
			flags := ""
			if mb.Seen(seq) {
				flags = `\Seen`
			}
			imapWrite(w, fmt.Sprintf("* %d FETCH (UID %d RFC822.SIZE %d BODY[] {%d}", seq, seq, len(raw), len(raw)))
			_, _ = w.Write(raw)
			// FLAGS 放在原文后面：真实服务器不保证 FLAGS 在字面量之前
			imapWrite(w, " FLAGS ("+flags+"))")
			imapWrite(w, tag+" OK FETCH completed")
		case "EXPUNGE":
			mb.expunge()
			imapWrite(w, tag+" OK EXPUNGE completed")
		case "APPEND":
			// APPEND <mailbox> [(flags)] {N}
			idx := strings.LastIndex(args, "{")
			if idx < 0 || !strings.HasSuffix(args, "}") {
				imapWrite(w, tag+" BAD APPEND needs literal")
				continue
			}
			n, err := strconv.Atoi(args[idx+1 : len(args)-1])
			if err != nil {
				imapWrite(w, tag+" BAD bad literal")
				continue
			}
			imapWrite(w, "+ Ready for literal data")
			raw := make([]byte, n)
			if _, err := readFull(r, raw); err != nil {
				return
			}
			_, _ = r.ReadString('\n') // trailing CRLF
			dst := s.box(strings.Fields(args)[0])
			if dst == nil {
				dst = s.mb
			}
			dst.Append(raw)
			imapWrite(w, tag+" OK APPEND completed")
		case "IDLE":
			imapWrite(w, "+ idling")
			sub := mb.subscribe()
			done := make(chan string, 1)
			go func() {
				l, err := r.ReadString('\n')
				if err != nil {
					done <- ""
					return
				}
				done <- strings.TrimSpace(l)
			}()
			var clientLine string
			select {
			case n := <-sub:
				imapWrite(w, fmt.Sprintf("* %d EXISTS", n))
				imapWrite(w, fmt.Sprintf("* %d RECENT", n))
				clientLine = <-done // 等客户端 DONE
			case clientLine = <-done:
			case <-time.After(10 * time.Minute):
				clientLine = ""
			}
			mb.unsubscribe(sub)
			_ = clientLine
			imapWrite(w, tag+" OK IDLE terminated")
		case "NOOP":
			imapWrite(w, tag+" OK NOOP completed")
		case "LOGOUT":
			imapWrite(w, "* BYE MockIMAP signing off")
			imapWrite(w, tag+" OK LOGOUT completed")
			return
		default:
			imapWrite(w, tag+" BAD unsupported: "+verb)
		}
	}
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
