// acp-shim-grok 把 xAI Grok（OpenAI 兼容端点）包装成 ACP over stdio 子进程
// （M4 设计 §A/§C：与 acp-shim-deepseek 同构克隆，差异仅 base_url/模型名/prompt）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aranea-agents/internal/agentbridge/acpshim"
	"aranea-agents/internal/agentbridge/acpshim/openaicompat"
	"aranea-agents/pkg/loggateway"
)

// options 命令行参数（parseOptions 输出；抽出以便单测退出码语义）。
type options struct {
	baseURL   string
	model     string
	system    string
	heartbeat time.Duration
	logLevel  string
}

// parseOptions 解析命令行。ContinueOnError 语义（评审 2026-11-09 D-LOW）：
// --help/-h 打印用法并返回 flag.ErrHelp（调用方映射为 exit 0）；
// 非法参数返回其他错误（调用方映射为 exit 2）。用法走 stderr，stdout 纯净红线。
func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("acp-shim-grok", flag.ContinueOnError)
	var o options
	fs.StringVar(&o.baseURL, "base-url", "https://api.x.ai/v1", "OpenAI 兼容端点")
	fs.StringVar(&o.model, "model", "grok-4", "模型名（宿主经 CodingAgent.Args 透传）")
	fs.StringVar(&o.system, "system", "", "system prompt（可空）")
	fs.DurationVar(&o.heartbeat, "heartbeat", 30*time.Second, "显式心跳周期（0=关；M4 §A 默认 30s）")
	fs.StringVar(&o.logLevel, "log-level", "info", "日志级别（走 stderr，stdout 纯净红线）")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	return o, nil
}

func main() {
	os.Exit(run())
}

func run() int {
	o, err := parseOptions(os.Args[1:])
	if err != nil {
		// --help/-h 属正常用法请求，exit 0；其余解析错误 exit 2。
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	lg := acpshim.NewStderrLogger("acp-shim-grok", o.logLevel)

	apiKey := os.Getenv("XAI_API_KEY")
	if apiKey == "" {
		lg.Error("XAI_API_KEY 未注入（宿主经 env 白名单 extras 透传）")
		return 2
	}

	llm := openaicompat.New(o.baseURL, apiKey, openaicompat.ProviderGrok, nil, lg)
	tools := acpshim.NewRegistry(lg, 0)
	broker := acpshim.NewBroker(lg, 0, acpshim.NewPermissionGuard(0, 0, 0))

	srv, err := acpshim.NewServer(acpshim.Config{
		Provider:          "grok",
		Model:             o.model,
		SystemPrompt:      o.system,
		LLM:               llm,
		Tools:             tools,
		Approver:          broker,
		HeartbeatInterval: o.heartbeat,
		LG:                lg,
	})
	if err != nil {
		lg.Error("server init failed", loggateway.Err(err))
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lg.Info("acp-shim-grok ready",
		loggateway.Str("base_url", o.baseURL), loggateway.Str("model", o.model))
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		lg.Error("serve failed", loggateway.Err(err))
		return 1
	}
	fmt.Fprintln(os.Stderr, "acp-shim-grok: connection closed, exiting")
	return 0
}
