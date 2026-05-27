// Package repl implements an interactive REPL for aranea chat.
package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"aranea-agents/internal/cli/client"

	"github.com/google/uuid"
	"github.com/peterh/liner"
)

const banner = `
 █████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗██╔══██╗██╔══██╗████╗  ██║██╔════╝██╔══██╗
███████║██████╔╝███████║██╔██╗ ██║█████╗  ███████║
██╔══██║██╔══██╗██╔══██║██║╚██╗██║██╔══╝  ██╔══██║
██║  ██║██║  ██║██║  ██║██║ ╚████║███████╗██║  ██║
╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚══════╝╚═╝  ╚═╝

输入消息与 AI 对话。/help 查看命令，/quit 退出。
`

// Config holds REPL startup parameters.
type Config struct {
	// APIBase is the backend base URL, e.g. "http://localhost:8000".
	APIBase string
	// Token is the JWT for authentication.
	Token string
	// SessionID is the session to attach to. If empty, a new one is used.
	SessionID string
	// AgentKey is the initial agent key. Defaults to empty (server picks default).
	AgentKey string
	// Output is where the REPL writes; defaults to os.Stdout.
	Output io.Writer
}

// REPL is the interactive chat REPL.
type REPL struct {
	cfg       Config
	sessionID string
	agentKey  string
	dryRun    bool
	conn      *client.WSConn
	ctx       context.Context
	cancel    context.CancelFunc
	renderer  *renderState
	output    io.Writer
}

// New creates a REPL from the given config.
func New(cfg Config) *REPL {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	sid := cfg.SessionID
	if sid == "" {
		sid = uuid.NewString()
	}
	return &REPL{
		cfg:       cfg,
		sessionID: sid,
		agentKey:  cfg.AgentKey,
		output:    out,
	}
}

// Run starts the REPL loop. It blocks until the user quits or ctx is cancelled.
func (r *REPL) Run(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	defer r.cancel()

	r.renderer = newRenderState(r.output)

	// Print banner.
	fmt.Fprint(r.output, banner)
	fmt.Fprintf(r.output, "会话 ID: %s\n\n", r.sessionID)

	// Connect to WebSocket.
	wsCli := &client.WSClient{
		Base:  r.cfg.APIBase,
		Token: r.cfg.Token,
	}
	conn, err := wsCli.Dial(r.ctx, r.sessionID)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	r.conn = conn
	defer conn.Close()

	// Start render goroutine.
	go r.renderLoop()

	// Set up liner.
	l := liner.NewLiner()
	l.SetCtrlCAborts(true)
	l.SetTabCompletionStyle(liner.TabPrints)
	loadHistory(l)
	defer func() {
		saveHistory(l)
		l.Close()
	}()

	return r.inputLoop(l)
}

func (r *REPL) inputLoop(l *liner.State) error {
	for {
		prompt := "你 > "
		if r.dryRun {
			prompt = "[dry-run] 你 > "
		}

		input, err := l.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				return nil
			}
			return nil
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		l.AppendHistory(input)

		if strings.HasPrefix(input, "/") {
			res := handleSlash(input, r, r.output)
			if res.Quit {
				return nil
			}
			continue
		}

		if err := r.sendMessage(input); err != nil {
			fmt.Fprintf(r.output, "[发送错误] %v\n", err)
		}
	}
}

func (r *REPL) sendMessage(content string) error {
	if r.dryRun {
		fmt.Fprintf(r.output, "[dry-run] 不发送: %s\n", content)
		return nil
	}
	payload := map[string]any{
		"content": content,
	}
	if r.agentKey != "" {
		payload["agent_key"] = r.agentKey
	}
	return r.conn.Send(r.ctx, buildEnvelope("user_message", payload))
}

func (r *REPL) renderLoop() {
	for env := range r.conn.Events() {
		r.renderer.Handle(env)
	}
}

// buildEnvelope constructs an upstream envelope.
func buildEnvelope(typ string, payload map[string]any) client.Envelope {
	return client.Envelope{
		Type:    typ,
		Payload: payload,
	}
}
