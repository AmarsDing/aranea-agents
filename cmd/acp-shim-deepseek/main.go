// acp-shim-deepseek 把 DeepSeek（OpenAI 兼容端点）包装成 ACP over stdio 子进程
// （M4 设计 §A/§C：acpshim 框架 + openaicompat → api.deepseek.com）。
// 装配 <100 行；一切差异收敛在 acpshim 与 openaicompat 内部。
package main

import (
	"context"
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

func main() {
	os.Exit(run())
}

func run() int {
	var (
		baseURL   = flag.String("base-url", "https://api.deepseek.com", "OpenAI 兼容端点")
		model     = flag.String("model", "deepseek-chat", "模型名（宿主经 CodingAgent.Args 透传）")
		system    = flag.String("system", "", "system prompt（可空）")
		heartbeat = flag.Duration("heartbeat", 30*time.Second, "显式心跳周期（0=关；M4 §A 默认 30s）")
		logLevel  = flag.String("log-level", "info", "日志级别（走 stderr，stdout 纯净红线）")
	)
	flag.Parse()

	lg := acpshim.NewStderrLogger("acp-shim-deepseek", *logLevel)

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		lg.Error("DEEPSEEK_API_KEY 未注入（宿主经 env 白名单 extras 透传）")
		return 2
	}

	llm := openaicompat.New(*baseURL, apiKey, openaicompat.ProviderDeepseek, nil, lg)
	tools := acpshim.NewRegistry(lg, 0)
	broker := acpshim.NewBroker(lg, 0, acpshim.NewPermissionGuard(0, 0, 0))

	srv, err := acpshim.NewServer(acpshim.Config{
		Provider:          "deepseek",
		Model:             *model,
		SystemPrompt:      *system,
		LLM:               llm,
		Tools:             tools,
		Approver:          broker,
		HeartbeatInterval: *heartbeat,
		LG:                lg,
	})
	if err != nil {
		lg.Error("server init failed", loggateway.Err(err))
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lg.Info("acp-shim-deepseek ready",
		loggateway.Str("base_url", *baseURL), loggateway.Str("model", *model))
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		lg.Error("serve failed", loggateway.Err(err))
		return 1
	}
	fmt.Fprintln(os.Stderr, "acp-shim-deepseek: connection closed, exiting")
	return 0
}
