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
	Mode        string   // 权限档位（PermManual 等）；startTerminal 按 launchMode 定下来
	Args        []string // 附加启动参数（工具设置或编码 agent 登记）
	TrustKey    string   // 非空 = 屏幕出现目录信任框时自动按一次（只给运行台自己建的 worktree）
	RunID       string
	TaskID      string
	Role        string
	Env         []string
	Argv        []string // 非空 = 原样启动（登录子命令等），不加档位参数
	Cols        int      // 初始列数；0 = 120。登录终端开宽，长链接不被折行
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
	trustSent       bool
	tailSig         string    // 屏幕最后几行的文字：停在确认菜单上还在不停重画的工具（CodeBuddy），
	tailSigSince    time.Time // 字节一直在来、文字却不变，靠它和这个时间认出「等你」
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
	cols := s.spec.Cols
	if cols <= 0 {
		cols = 120
	}
	_ = p.Resize(cols, 32)
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
	s.exited, s.killed, s.exitCode, s.firstOutput, s.prompted, s.trustSent = false, false, 0, false, false, false
	s.cols, s.rows = cols, 32
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
	if code == 0 || s.killed { // 人或运行台主动结束的不算异常（被杀进程的退出码非 0）
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
	if t := parseOSCTitle(data); t != "" {
		s.title = t
	}
	// 只改了终端标题的一段（CodeBuddy 停在确认菜单上还每 200ms 换一次标题里的转圈字）：
	// 留在回放里，但不算屏幕上的新内容——否则既显得一直在忙，又会把确认菜单挤出判定用的尾部缓冲
	if !oscOnly(data) {
		s.tail = append(s.tail, data...)
		if len(s.tail) > tailMax {
			s.tail = append([]byte(nil), s.tail[len(s.tail)-tailMax/2:]...)
		}
		s.lastOutput = time.Now()
		s.firstOutput = true
		// 改尺寸后终端会重绘一屏：这段输出不是 agent 在干活，不计入「工作中」的判定。
		if time.Since(s.resizedAt) > resizeRepaintWindow {
			s.windowBytes += len(data)
		}
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

// plainTypeAgents 把一段连续输入当成「粘贴」、转去读系统剪贴板的工具：豆包 CLI 2.37.2 收到括号粘贴、
// 或者一次写进去的一整段字时，都去读剪贴板——剪贴板里是截图就插成「[Image #1]」，是空的就发出「(null)」，
// 真正的文字丢掉（2026-11-21 派活实测）。这些工具改成一个字一个字地打（换行换成空格：打字时换行就是回车）。
var plainTypeAgents = map[string]bool{"doubao": true}

// typeGap 是逐字打字时两个字之间的间隔：豆包按输入节奏区分打字和粘贴。
var typeGap = 6 * time.Millisecond

// Paste 用括号粘贴模式送一段文本（多行也不会被当成多次回车），submit 时随后补回车。
func (s *Session) Paste(text string, submit bool) error {
	if plainTypeAgents[s.spec.Agent] {
		go func() {
			for _, r := range strings.Join(strings.Fields(text), " ") {
				if s.Write([]byte(string(r))) != nil {
					return
				}
				time.Sleep(typeGap)
			}
			if submit {
				// 豆包加载时写着「You can type; sending is paused」：能打字，这时回车却会发出空消息「(null)」，等它加载完再回车
				time.Sleep(300 * time.Millisecond)
				for i := 0; i < 120 && s.screenStarting(); i++ {
					time.Sleep(500 * time.Millisecond)
				}
				_ = s.Write([]byte("\r"))
			}
		}()
		return nil
	}
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
		Status: s.status, FullAccess: s.spec.FullAccess, Mode: s.spec.Mode, PID: s.pid, StartedAt: s.startedAt,
		ExitCode: s.exitCode, LastActivityAt: s.lastOutput, StatusChangedAt: s.statusChangedAt,
		Preview: previewLines(s.tail, 5), RunID: s.spec.RunID, TaskID: s.spec.TaskID, Role: s.spec.Role,
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
	// 截掉的开头可能切在一个汉字中间：清掉残字节（这段文字会进任务的摘要、事件和 proto）
	text := StripANSI(strings.ToValidUTF8(string(data), ""))
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
	var trustKey string
	if needs && s.spec.TrustKey != "" && !s.trustSent && classifyWaiting(strings.Join(lines, "\n")) == ReadyNeedTrust {
		s.trustSent, trustKey = true, s.spec.TrustKey
	}
	spinning := titleSpinning(s.title)
	echoOnly := now.Sub(s.lastInput) < 600*time.Millisecond && recent < 2000
	if sig := strings.Join(lines, "\n"); sig != s.tailSig {
		s.tailSig, s.tailSigSince = sig, now
	}
	steady := now.Sub(s.tailSigSince) >= 1500*time.Millisecond

	switch {
	case !s.firstOutput:
		if now.Sub(s.startedAt) > time.Minute {
			next = StatusIdle
		}
	case needs && steady:
		// 确认菜单停在屏幕上不动了：哪怕它还在重画、标题还在转圈（CodeBuddy 两样都做），也是在等人
		next = StatusNeedsInput
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
	// 首句提示等启动满 8 秒、安静 3 秒再发：Cursor CLI 输出停了还在后台初始化，启动约 5 秒时粘贴进去的回车会被吞掉，
	// 7.5 秒后正常（2026-11-20 实测）
	// 屏幕上还写着在初始化 / 暂停发送（豆包 CLI 2.37.2：安静地停在「directory: Initializing…」「sending is paused」，
	// 这时打进去的字会丢、回车发出空消息，2026-11-21 实测）也要再等
	if s.pendingPrompt != "" && (next == StatusIdle || next == StatusDone) && !needs && !stillStarting(lines) &&
		quiet >= 3*time.Second && now.Sub(s.startedAt) >= 8*time.Second {
		prompt, s.pendingPrompt = s.pendingPrompt, ""
	}
	s.mu.Unlock()
	if trustKey != "" {
		_ = s.Write([]byte(trustKey))
	}
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
	// tail 是截断的字节缓冲，开头可能切在一个汉字中间：不清掉的话 proto 序列化整张终端列表都会失败（500）
	lines := nonEmptyLines(StripANSI(strings.ToValidUTF8(string(tail), "")))
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
	// 以下取自 awslabs/cli-agent-orchestrator 各 provider 的等待字样（2026-11-20 核对）：
	// Claude Code 工具权限框页脚、Codex 审批框、Cursor 目录信任、Grok 审批选项。
	"esc to cancel · tab to amend", "↑/↓ to navigate", "command approval required",
	"confirm folder trust", "yes, proceed", "no, reject",
	// 2026-11-20 九家 CLI 实测：选择菜单页脚、目录信任、登录等待（Gemini / Codex / Qoder / Cursor / 豆包 / CodeBuddy）。
	"enter to select", "enter select", "enter continue", "enter to login", "don't trust", "trust and continue",
	"press any key to log in", "sign in to continue", "扫码",
	// Cursor CLI 2026.11.19 的命令审批框与拒绝后的追问
	"run this command?", "not in allowlist", "what to do instead",
	// CodeBuddy 的权限菜单用「> 1. Yes」做光标，第二项是「Yes, and don't ask again this session」（2026-11-21 实测）
	"> 1. yes", "don't ask again",
	"是否继续", "是否允许", "请确认",
}

// screenStarting 屏幕最后几行还说在初始化 / 暂停发送。
func (s *Session) screenStarting() bool {
	s.mu.Lock()
	lines := nonEmptyLines(StripANSI(strings.ToValidUTF8(string(s.tail), "")))
	s.mu.Unlock()
	if len(lines) > needsInputLines {
		lines = lines[len(lines)-needsInputLines:]
	}
	return stillStarting(lines)
}

// oscOnly 这段输出只有 OSC 控制序列（改标题等），屏幕上没有新字。
func oscOnly(data []byte) bool {
	return len(data) > 0 && strings.TrimSpace(oscRe.ReplaceAllString(string(data), "")) == ""
}

// stillStarting 屏幕尾部说它还在初始化 / 暂停发送：首句提示再等等。
func stillStarting(lines []string) bool {
	tail := strings.ToLower(strings.Join(lines, " "))
	return strings.Contains(tail, "initializing") || strings.Contains(tail, "sending is paused")
}

func matchNeedsInput(text string) bool {
	// 光标移动重绘会把一句话拆到几行（Qoder 的「Enter select」实测如此），先把空白归一
	text = strings.Join(strings.Fields(text), " ")
	for _, p := range needsInputPatterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
