// Package console 实现 Aranea 的 ADK SubLauncher，驱动交互式 REPL。结构
// 仿照 google.golang.org/adk/cmd/launcher/console，但通过 HTTP 与 Aranea
// 后端通信，而非在进程内运行 ADK runner。这样系统管理员智能体触发的
// 操作均经审计的 /api/v1/* 层，与 Web UI 一致。
package console

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"

	adklauncher "google.golang.org/adk/cmd/launcher"

	"arenea/backend/cmd/aranea/cli/agent"
	"arenea/backend/cmd/aranea/cli/apiclient"
	araneal "arenea/backend/cmd/aranea/launcher"
	"arenea/backend/cmd/aranea/cli/session"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// NewLauncher 构建 console SubLauncher。在此捕获 Aranea Config，Run 时
// 再消费，从而无需将非 ADK 类型的指针挂到 adklauncher.Config 上。
func NewLauncher(cfg *araneal.Config) adklauncher.SubLauncher {
	flags := flag.NewFlagSet("console", flag.ContinueOnError)
	c := &consoleConfig{}
	flags.StringVar(&c.AgentKey, "agent", "__system_admin__", "agent_key to chat with")
	flags.StringVar(&c.SessionID, "session", "", "Resume an existing session id")
	flags.StringVar(&c.Mode, "mode", "default", "Dialog mode (default|plan|code|...)")
	flags.BoolVar(&c.AutoYes, "yes", false, "Auto-confirm any /confirm prompts")
	flags.BoolVar(&c.NoStream, "no-stream", false, "Disable SSE streaming and use POST /chat/messages instead")
	return &consoleLauncher{flags: flags, config: c, arn: cfg}
}

// consoleConfig 保存 console launcher 的解析后 CLI 标志。
type consoleConfig struct {
	AgentKey  string
	SessionID string
	Mode      string
	AutoYes   bool
	NoStream  bool
}

// consoleLauncher 实现 adklauncher.SubLauncher。
type consoleLauncher struct {
	flags  *flag.FlagSet
	config *consoleConfig
	arn    *araneal.Config
}

// Keyword 实现 adklauncher.SubLauncher。
func (l *consoleLauncher) Keyword() string { return "console" }

// SimpleDescription 实现 adklauncher.SubLauncher。
func (l *consoleLauncher) SimpleDescription() string {
	return "interactive REPL chatting with the system administrator agent"
}

// CommandLineSyntax 实现 adklauncher.SubLauncher。自行渲染 flag 集，避免
// 依赖 adk 内部 cli/util 包。
func (l *consoleLauncher) CommandLineSyntax() string {
	var buf bytes.Buffer
	buf.WriteString("Flags:\n")
	l.flags.SetOutput(&buf)
	l.flags.PrintDefaults()
	return buf.String()
}

// Parse 实现 adklauncher.SubLauncher。
func (l *consoleLauncher) Parse(args []string) ([]string, error) {
	if err := l.flags.Parse(args); err != nil {
		return nil, fmt.Errorf("console: %w", err)
	}
	return l.flags.Args(), nil
}

// Run 实现 adklauncher.SubLauncher。交互式 console 的核心：解析目标
// 智能体、确保存在会话，然后循环读取用户输入，将每行转发到聊天
// 流式 API，并把 SSE 事件渲染回终端。
func (l *consoleLauncher) Run(ctx context.Context, _ *adklauncher.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	g := l.arn.Client
	if g == nil {
		return errors.New("console launcher: no API client configured")
	}

	agentRef, err := agent.Resolve(ctx, g, l.config.AgentKey)
	if err != nil {
		// 尽力回退：选取列表中第一个智能体，使全新安装（尚未种子化
		// __system_admin__）时用户仍有一个可用的 REPL。
		fallback, fbErr := pickFallbackAgent(ctx, g)
		if fbErr != nil {
			return fmt.Errorf("agent %q not found and no fallback available: %w", l.config.AgentKey, err)
		}
		fmt.Fprintf(os.Stderr, "warning: agent %q not found, falling back to %q\n", l.config.AgentKey, fallback.AgentKey)
		agentRef = fallback
	}

	sess, err := session.EnsureSession(ctx, g, l.config.SessionID, agentRef.ID, "CLI console with "+agentRef.DisplayName)
	if err != nil {
		return err
	}

	printBanner(agentRef, sess, l.config.Mode)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)

	for {
		fmt.Print("\x1b[36mYou\x1b[0m > ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			fmt.Println()
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if handled, exit := l.handleSlash(ctx, g, &sess, line); handled {
			if exit {
				return nil
			}
			continue
		}
		if err := l.send(ctx, g, sess.ID, agentRef.AgentKey, line); err != nil {
			fmt.Fprintf(os.Stderr, "\x1b[31merror\x1b[0m %v\n", err)
		}
	}
}

func pickFallbackAgent(ctx context.Context, g *apiclient.GlobalContext) (domain.Agent, error) {
	var resp domain.AgentListResult
	if err := g.Client().Get(ctx, "/api/v1/agents", nil, &resp); err != nil {
		return domain.Agent{}, err
	}
	if len(resp.Items) == 0 {
		return domain.Agent{}, errors.New("no agents available")
	}
	return resp.Items[0], nil
}

func printBanner(a domain.Agent, s domain.Session, mode string) {
	fmt.Printf("\x1b[1mAranea console\x1b[0m\n")
	fmt.Printf("  agent  : %s (%s)\n", a.DisplayName, a.AgentKey)
	fmt.Printf("  session: %s\n", s.ID)
	fmt.Printf("  mode   : %s\n", mode)
	fmt.Printf("Type /help for commands, /quit to exit.\n\n")
}

// handleSlash 实现 前端/25 cli.md §1.7 所定义的轻量斜杠命令（子集）。
// 返回 (handled, exit) —— 仅对 /quit、/exit 时 exit 为 true，供 Run 退出循环。
func (l *consoleLauncher) handleSlash(ctx context.Context, g *apiclient.GlobalContext, sess *domain.Session, line string) (bool, bool) {
	if !strings.HasPrefix(line, "/") {
		return false, false
	}
	parts := strings.Fields(line)
	switch parts[0] {
	case "/help", "/?":
		fmt.Println("/help                  show this message")
		fmt.Println("/quit, /exit           leave the REPL")
		fmt.Println("/clear                 clear screen")
		fmt.Println("/session new           start a fresh session with the same agent")
		fmt.Println("/agent <key>           switch to another agent (creates a new session)")
		fmt.Println("/run <cli args...>     run an `aranea` sub-command in-process")
	case "/quit", "/exit":
		return true, true
	case "/clear":
		fmt.Print("\x1b[2J\x1b[H")
	case "/session":
		if len(parts) >= 2 && parts[1] == "new" {
			ns, err := session.EnsureSession(ctx, g, "", sess.AgentID, "CLI console (new)")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			} else {
				*sess = ns
				fmt.Printf("started session %s\n", ns.ID)
			}
		}
	case "/agent":
		if len(parts) < 2 {
			fmt.Println("usage: /agent <agent_key>")
			break
		}
		a, err := agent.Resolve(ctx, g, parts[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			break
		}
		ns, err := session.EnsureSession(ctx, g, "", a.ID, "CLI console with "+a.DisplayName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			break
		}
		*sess = ns
		fmt.Printf("switched to %s (%s)\n", a.DisplayName, a.AgentKey)
	case "/run":
		fmt.Println("(/run is not implemented in this build — open another terminal)")
	default:
		fmt.Printf("unknown slash command: %s\n", parts[0])
	}
	return true, false
}

// send 流式发送单条用户消息并将智能体回复打印到 stdout。若设置
// NoStream 则回退到同步端点，以便在关闭流式传输的后端上仍可使用 console。
func (l *consoleLauncher) send(ctx context.Context, g *apiclient.GlobalContext, sessionID, agentKey, content string) error {
	in := service.SendMessageInput{
		SessionID: sessionID,
		AgentKey:  agentKey,
		Content:   content,
		Options:   service.SendMessageOptions{DialogMode: l.config.Mode},
	}
	if l.config.NoStream {
		var out service.SendMessageResult
		if err := g.Client().Post(ctx, "/api/v1/chat/messages", in, &out); err != nil {
			return err
		}
		fmt.Printf("\x1b[35m%s\x1b[0m > %s\n\n", out.AgentMessage.ModelName, out.AgentMessage.Content)
		return nil
	}
	return l.stream(ctx, g, in)
}

// stream 向 /api/v1/chat/messages/stream 发起 POST 并增量渲染 SSE 流。
// 实现刻意保持最小：解析 `event:` 与 `data:` 行并按事件类型分发。
func (l *consoleLauncher) stream(ctx context.Context, g *apiclient.GlobalContext, in service.SendMessageInput) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := strings.TrimRight(g.Client().BaseURL(), "/") + "/api/v1/chat/messages/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if token := g.Client().Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	fmt.Print("\x1b[35magent\x1b[0m > ")
	reader := bufio.NewReader(resp.Body)
	currentEvent := "message"
	wroteAny := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			currentEvent = "message"
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		switch currentEvent {
		case "delta":
			var d struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(data), &d); err == nil {
				fmt.Print(d.Content)
				wroteAny = true
			}
		case "done":
			var d map[string]domain.Message
			if err := json.Unmarshal([]byte(data), &d); err == nil {
				if msg, ok := d["agent_message"]; ok && !wroteAny {
					fmt.Print(msg.Content)
				}
			}
		case "error":
			var d struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(data), &d)
			fmt.Fprintf(os.Stderr, "\n\x1b[31merror\x1b[0m %s\n", d.Message)
		}
	}
	fmt.Println()
	return nil
}
