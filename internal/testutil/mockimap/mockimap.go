// Package mockimap 提供进程内 mock IMAP4rev1 服务端（E 阶段 W1d 邮件域
// 测试与冒烟证据用；移植自 workbench server/mockimap.go，语义逐行保持）。
//
// 支持：CAPABILITY / LOGIN / SELECT / LIST / SEARCH / FETCH (RFC822) /
// UID FETCH / APPEND（模拟新邮件送达）/ IDLE（真实推送 * n EXISTS）/
// NOOP / LOGOUT。账号任意非空即可登录。
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
	mu   sync.Mutex
	msgs [][]byte
	subs map[chan int]struct{}
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
		[]byte("From: alice@example.com\r\nTo: test@local\r\nSubject: Weekly sync notes\r\nDate: Mon, 02 Nov 2026 09:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n本周同步会议纪要：目标对齐、下周计划。\r\n"),
		[]byte("From: boss@example.com\r\nTo: test@local\r\nSubject: " + encSubject + "\r\nDate: Mon, 02 Nov 2026 10:30:00 +0800\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=MOCKBB\r\n\r\n--MOCKBB\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n请查收项目资金方案，详见附件 budget.txt。\r\n\r\n--MOCKBB\r\nContent-Type: text/plain; name=budget.txt\r\nContent-Disposition: attachment; filename=budget.txt\r\nContent-Transfer-Encoding: base64\r\n\r\n" + attContent + "\r\n\r\n--MOCKBB--\r\n"),
		[]byte("From: hr@example.com\r\nTo: test@local\r\nSubject: " + mime.BEncoding.Encode("utf-8", "团建报名截止提醒") + "\r\nDate: Mon, 02 Nov 2026 11:00:00 +0800\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n周五团建报名今天截止，请尽快填写表单。\r\n"),
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

// Server mock IMAP 服务端（IMAP 协议口 + 可选 HTTP 控制口）。
type Server struct {
	mb    *Mailbox
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
	s := &Server{mb: NewMailbox(), ln: ln, done: make(chan struct{})}
	go s.acceptLoop()
	return s, nil
}

// Addr IMAP 协议口实际地址。
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Mailbox 暴露邮箱（测试直接 Append 注入；冒烟走控制口 /inject）。
func (s *Server) Mailbox() *Mailbox { return s.mb }

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
		n := s.mb.Append(body)
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
		go handleConn(conn, s.mb)
	}
}

func imapWrite(w *bufio.Writer, s string) {
	_, _ = w.WriteString(s + "\r\n")
	_ = w.Flush()
}

func handleConn(conn net.Conn, mb *Mailbox) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	imapWrite(w, "* OK MockIMAP4rev1 ready")
	selected := false
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
			imapWrite(w, tag+" OK LOGIN completed")
		case "SELECT", "EXAMINE":
			imapWrite(w, fmt.Sprintf("* %d EXISTS", mb.Count()))
			imapWrite(w, "* OK [READ-WRITE] SELECT completed")
			imapWrite(w, tag+" OK SELECT completed")
			selected = true
		case "LIST":
			imapWrite(w, `* LIST (\HasNoChildren) "/" "INBOX"`)
			imapWrite(w, tag+" OK LIST completed")
		case "SEARCH":
			n := mb.Count()
			var ids []string
			for i := 1; i <= n; i++ {
				ids = append(ids, strconv.Itoa(i))
			}
			imapWrite(w, "* SEARCH "+strings.Join(ids, " "))
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
			imapWrite(w, fmt.Sprintf("* %d FETCH (UID %d RFC822.SIZE %d RFC822 {%d}", seq, seq, len(raw), len(raw)))
			_, _ = w.Write(raw)
			imapWrite(w, ")")
			imapWrite(w, tag+" OK FETCH completed")
		case "APPEND":
			// APPEND INBOX {N}
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
			mb.Append(raw)
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
