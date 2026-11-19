package fleet

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/aymanbagabas/go-pty"
)

const (
	scrollbackMax  = 1 << 20 // 每个终端保留 1MB 输出，重连/多窗口回放用
	scrollbackKeep = 768 << 10
	tailMax        = 6 << 10
	subBuffer      = 256
	// 判「等你」时看的屏幕末尾行数：够容纳一个带 3～4 个选项的确认框。
	needsInputLines = 6
	// 改尺寸后这段时间内的输出视为重绘。
	resizeRepaintWindow = 1500 * time.Millisecond
)

// sessionSpec 是起一个终端所需的全部参数（重启时原样复用）。
type sessionSpec struct {
	WorkspaceID string
	Agent       string
	Title       string
	Dir         string
	Prompt      string
	FullAccess  bool
	RunID       string
	TaskID      string
	Role        string
	Env         []string
}

// Frame 是推给订阅者的一帧：输出字节，或进程退出通知。
type Frame struct {
	Data     []byte
	Exit     bool
	ExitCode int
}

// Session 是一个跑在伪终端里的进程。
type Session struct {
	id    string
	token string
	spec  sessionSpec

	onStatus func(s *Session, old, next string)

	mu              sync.Mutex
	p               pty.Pty
	closePTY        func() // 每个伪终端只关一次：Windows ConPTY 重复关闭会让本进程崩溃
	cmd             *pty.Cmd
	buf             []byte
	subs            map[int]chan Frame
	nextSub         int
	status          string
	statusChangedAt time.Time
	startedAt       time.Time
	lastOutput      time.Time
	lastInput       time.Time
	windowBytes     int
	prevWindow      int
	tail            []byte
	title           string
	firstOutput     bool
	prompted        bool
	pendingPrompt   string
	exited          bool
	killed          bool // 由人或运行台主动结束：不发「已退出」通知
	cols, rows      int
	resizedAt       time.Time
	exitCode        int
	pid             int
}

func newSession(id, token string, spec sessionSpec, onStatus func(*Session, string, string)) *Session {
	return &Session{id: id, token: token, spec: spec, onStatus: onStatus, subs: map[int]chan Frame{}}
}

// start 起进程。argv[0] 是可执行文件。
func (s *Session) start(argv []string) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("empty command")
	}
	p, err := pty.New()
	if err != nil {
		return err
	}
	_ = p.Resize(120, 32)
	cmd := p.Command(argv[0], argv[1:]...)
	cmd.Dir = s.spec.Dir
	cmd.Env = s.spec.Env
	closeOnce := sync.OnceFunc(func() { _ = p.Close() })
	if err := cmd.Start(); err != nil {
		closeOnce()
		return err
	}
	now := time.Now()
	s.mu.Lock()
	s.p, s.cmd, s.closePTY = p, cmd, closeOnce
	s.startedAt, s.lastOutput, s.statusChangedAt = now, time.Time{}, now
	s.exited, s.killed, s.exitCode, s.firstOutput, s.prompted = false, false, 0, false, false
	s.cols, s.rows = 120, 32
	s.status = StatusStarting
	if s.spec.Agent == "shell" || s.spec.Agent == "" {
		s.status = StatusShell
	}
	s.pendingPrompt = s.spec.Prompt
	if cmd.Process != nil {
		s.pid = cmd.Process.Pid
	}
	s.mu.Unlock()
	go s.readLoop(p)
	go s.waitLoop(cmd, p, closeOnce)
	return nil
}

func (s *Session) readLoop(p pty.Pty) {
	buf := make([]byte, 32<<10)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			s.appendOutput(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) waitLoop(cmd *pty.Cmd, p pty.Pty, closePTY func()) {
	err := cmd.Wait()
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	} else if err != nil {
		code = -1
	}
	// 给 readLoop 一点时间把最后的输出读完再关 pty。
	time.Sleep(150 * time.Millisecond)
	closePTY()
	s.mu.Lock()
	if s.p != p { // 已被重启替换
		s.mu.Unlock()
		return
	}
	s.exited, s.exitCode = true, code
	old := s.status
	if code == 0 {
		s.status = StatusExited
	} else {
		s.status = StatusFailed
	}
	s.statusChangedAt = time.Now()
	next := s.status
	for id, ch := range s.subs {
		select {
		case ch <- Frame{Exit: true, ExitCode: code}:
		default:
		}
		close(ch)
		delete(s.subs, id)
	}
	s.mu.Unlock()
	if s.onStatus != nil && old != next {
		s.onStatus(s, old, next)
	}
}

func (s *Session) appendOutput(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, data...)
	if len(s.buf) > scrollbackMax {
		s.buf = append([]byte(nil), s.buf[len(s.buf)-scrollbackKeep:]...)
	}
	s.tail = append(s.tail, data...)
	if len(s.tail) > tailMax {
		s.tail = append([]byte(nil), s.tail[len(s.tail)-tailMax/2:]...)
	}
	if t := parseOSCTitle(data); t != "" {
		s.title = t
	}
	s.lastOutput = time.Now()
	s.firstOutput = true
	// 改尺寸后终端会重绘一屏：这段输出不是 agent 在干活，不计入「工作中」的判定。
	if time.Since(s.resizedAt) > resizeRepaintWindow {
		s.windowBytes += len(data)
	}
	for id, ch := range s.subs {
		select {
		case ch <- Frame{Data: data}:
		default: // 订阅者太慢：断开，前端重连时从缓冲回放
			close(ch)
			delete(s.subs, id)
		}
	}
}

// Subscribe 返回当前缓冲快照与后续输出流。
func (s *Session) Subscribe() ([]byte, <-chan Frame, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := append([]byte(nil), s.buf...)
	ch := make(chan Frame, subBuffer)
	if s.exited {
		ch <- Frame{Exit: true, ExitCode: s.exitCode}
		close(ch)
		return snap, ch, func() {}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	return snap, ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if c, ok := s.subs[id]; ok {
			close(c)
			delete(s.subs, id)
		}
	}
}

// Write 把键盘输入写进终端。
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	p, exited := s.p, s.exited
	if !exited {
		s.lastInput = time.Now()
		if bytes.ContainsAny(data, "\r\n") {
			s.prompted = true
		}
	}
	s.mu.Unlock()
	if p == nil || exited {
		return errors.New("terminal is not running")
	}
	_, err := p.Write(data)
	return err
}

// Paste 用括号粘贴模式送一段文本（多行也不会被当成多次回车），submit 时随后补回车。
func (s *Session) Paste(text string, submit bool) error {
	if err := s.Write([]byte("\x1b[200~" + text + "\x1b[201~")); err != nil {
		return err
	}
	if submit {
		go func() {
			time.Sleep(250 * time.Millisecond)
			_ = s.Write([]byte("\r"))
		}()
	}
	return nil
}

// Resize 调整终端尺寸。
func (s *Session) Resize(cols, rows int) {
	if cols < 10 || rows < 3 || cols > 1000 || rows > 500 {
		return
	}
	s.mu.Lock()
	p, exited := s.p, s.exited
	same := cols == s.cols && rows == s.rows
	if !same {
		s.cols, s.rows, s.resizedAt = cols, rows, time.Now()
	}
	s.mu.Unlock()
	if p != nil && !exited && !same {
		_ = p.Resize(cols, rows)
	}
}

// Kill 结束进程（关闭伪终端会让依附其上的子进程一并退出）。
func (s *Session) Kill() {
	s.mu.Lock()
	cmd, closePTY, exited := s.cmd, s.closePTY, s.exited
	s.killed = !exited
	s.mu.Unlock()
	if exited {
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if closePTY != nil {
		closePTY()
	}
}

// Info 返回对外快照。
func (s *Session) Info() TerminalInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return TerminalInfo{
		ID: s.id, WorkspaceID: s.spec.WorkspaceID, Agent: s.spec.Agent, Title: s.displayTitle(),
		Status: s.status, FullAccess: s.spec.FullAccess, PID: s.pid, StartedAt: s.startedAt,
		ExitCode: s.exitCode, LastActivityAt: s.lastOutput, StatusChangedAt: s.statusChangedAt,
		Preview: previewLines(s.tail, 3), RunID: s.spec.RunID, TaskID: s.spec.TaskID, Role: s.spec.Role,
	}
}

func (s *Session) displayTitle() string {
	if s.spec.Title != "" {
		return s.spec.Title
	}
	t := strings.TrimSpace(strings.TrimLeftFunc(s.title, func(r rune) bool { return isSpinner(r) || r == '✳' || unicode.IsSpace(r) }))
	if strings.HasSuffix(strings.ToLower(t), ".exe") { // Windows 控制台的默认标题是可执行文件全路径，不是有用的名字
		return ""
	}
	return t
}

// ReadText 返回去掉控制字符后的最近 n 行输出（编排里 worker-read 用）。
func (s *Session) ReadText(n int) string {
	s.mu.Lock()
	data := s.buf
	if len(data) > 64<<10 {
		data = data[len(data)-64<<10:]
	}
	text := StripANSI(string(data))
	s.mu.Unlock()
	lines := nonEmptyLines(text)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// tick 每秒调用一次，按输出节奏、终端标题和屏幕尾部判定状态。
func (s *Session) tick(now time.Time) {
	s.mu.Lock()
	if s.exited || s.status == StatusShell {
		s.windowBytes = 0
		s.mu.Unlock()
		return
	}
	recent := s.windowBytes + s.prevWindow
	s.prevWindow, s.windowBytes = s.windowBytes, 0
	old := s.status
	next := old
	quiet := now.Sub(s.lastOutput)
	// 只看屏幕最后几行：确认提示答过之后仍留在上方，不能再算「等你」。
	lines := nonEmptyLines(StripANSI(string(s.tail)))
	if len(lines) > needsInputLines {
		lines = lines[len(lines)-needsInputLines:]
	}
	needs := matchNeedsInput(strings.ToLower(strings.Join(lines, "\n")))
	spinning := titleSpinning(s.title)
	echoOnly := now.Sub(s.lastInput) < 600*time.Millisecond && recent < 2000

	switch {
	case !s.firstOutput:
		if now.Sub(s.startedAt) > time.Minute {
			next = StatusIdle
		}
	case spinning || (recent > 300 && !echoOnly && quiet < 1500*time.Millisecond):
		next = StatusWorking
	case needs && quiet > time.Second:
		next = StatusNeedsInput
	case old == StatusWorking && quiet >= 3*time.Second:
		next = StatusIdle
		if s.prompted {
			next = StatusDone
		}
	case old == StatusStarting && quiet >= 1500*time.Millisecond:
		next = StatusIdle
	case old == StatusNeedsInput && !needs && quiet > time.Second:
		next = StatusIdle
		if s.prompted {
			next = StatusDone
		}
	case old == StatusDone && now.Sub(s.statusChangedAt) > 30*time.Minute:
		next = StatusIdle
	}
	var prompt string
	if next != old {
		s.status, s.statusChangedAt = next, now
	}
	if s.pendingPrompt != "" && (next == StatusIdle || next == StatusDone) && !needs {
		prompt, s.pendingPrompt = s.pendingPrompt, ""
	}
	s.mu.Unlock()
	if prompt != "" {
		_ = s.Paste(prompt, true)
	}
	if next != old && s.onStatus != nil {
		s.onStatus(s, old, next)
	}
}

// ---------- 文本处理 ----------

var (
	sgrRe   = regexp.MustCompile(`\x1b\[[0-9;:]*m`)
	cufRe   = regexp.MustCompile(`\x1b\[[0-9]*C`)
	csiRe   = regexp.MustCompile(`\x1b\[[0-9;?<>=!]*[ -/]*[@-~]`)
	oscRe   = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	escRe   = regexp.MustCompile(`\x1b[()*+][0-9A-Za-z]|\x1b[=>78DEHMNOPZc\\]`)
	ctrlRe  = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	titleRe = regexp.MustCompile(`\x1b\][02];([^\x07\x1b]*)(?:\x07|\x1b\\)`)
)

// StripANSI 去掉终端控制序列；回车覆写只保留最后一段。
func StripANSI(s string) string {
	s = oscRe.ReplaceAllString(s, "")
	s = sgrRe.ReplaceAllString(s, "")
	s = cufRe.ReplaceAllString(s, " ")
	s = csiRe.ReplaceAllString(s, "\n")
	s = escRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.LastIndex(l, "\r"); j >= 0 && j < len(l)-1 {
			l = l[j+1:]
		}
		lines[i] = ctrlRe.ReplaceAllString(strings.ReplaceAll(l, "\r", ""), "")
	}
	return strings.Join(lines, "\n")
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" && !isBoxDrawing(t) {
			out = append(out, t)
		}
	}
	return out
}

func previewLines(tail []byte, n int) string {
	lines := nonEmptyLines(StripANSI(string(tail)))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		if r := []rune(l); len(r) > 160 {
			lines[i] = string(r[:160]) + "…"
		}
	}
	return strings.Join(lines, "\n")
}

func isBoxDrawing(s string) bool {
	for _, r := range s {
		if !(r >= 0x2500 && r <= 0x259F) && r != ' ' && r != '│' && r != '─' {
			return false
		}
	}
	return true
}

func parseOSCTitle(data []byte) string {
	m := titleRe.FindAllSubmatch(data, -1)
	if len(m) == 0 {
		return ""
	}
	return string(m[len(m)-1][1])
}

func isSpinner(r rune) bool {
	return (r >= 0x2800 && r <= 0x28FF) || strings.ContainsRune("◐◓◑◒⣾⣽⣻⢿⡿⣟⣯⣷", r)
}

// titleSpinning：Claude Code 等会在终端标题前放转圈字符表示在干活。
func titleSpinning(title string) bool {
	for _, r := range strings.TrimSpace(title) {
		return isSpinner(r) && r != 0x2800
	}
	return false
}

// 屏幕尾部出现这些字样且输出停住，判为「等你」（权限确认、选项、y/n）。
var needsInputPatterns = []string{
	"do you want to", "would you like to", "do you trust", "allow this", "allow once",
	"(y/n)", "[y/n]", "(yes/no)", "press enter to continue", "yes, and don't ask",
	"waiting for approval", "approve this", "requires approval", "❯ 1. yes", "› 1. yes",
	// Claude Code 的选择框（信任目录等）：实测 2026-11-18，v2.1.318。
	"enter to confirm", "i trust this folder",
	"是否继续", "是否允许", "请确认",
}

func matchNeedsInput(text string) bool {
	for _, p := range needsInputPatterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
