package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aranea-agents/internal/agentbridge/acpshim"
	"aranea-agents/internal/agentbridge/acpshim/openaicompat"
	"aranea-agents/pkg/loggateway"
)

// acpShimCommand 是宿主进程的内置通用 shim 子命令名（`admin acp-shim …`）。
// 用途：把任意 OpenAI 兼容端点包装成 ACP over stdio 子进程，取代按厂商
// 单独编译的 acp-shim-* 二进制——新增 OpenAI 兼容编程工具零代码、纯注册表
// 配置（command 填 "@self"，args 形如 ["acp-shim","--base-url=…","--model=…",
// "--api-key-env=…"]，编程工具设置页提供一键模板）。
const acpShimCommand = "acp-shim"

// maybeRunACPShim 在 kratos 启动前拦截 `admin acp-shim` 子命令：命中则以
// 纯 stdio 桥进程运行（不加载配置、不起服务），退出码即进程退出码。
func maybeRunACPShim() (handled bool, code int) {
	if len(os.Args) < 2 || os.Args[1] != acpShimCommand {
		return false, 0
	}
	return true, runACPShim(os.Args[2:])
}

// acpShimConfig 通用 shim 子命令参数。
type acpShimConfig struct {
	baseURL           string
	model             string
	apiKeyEnv         string
	provider          string
	system            string
	heartbeat         time.Duration
	logLevel          string
	fallbackNonStream bool
}

// parseACPShimArgs 解析子命令参数（独立函数以便单测覆盖校验逻辑；
// stderr 注入避免测试污染进程 stderr）。
func parseACPShimArgs(args []string, stderr io.Writer) (acpShimConfig, error) {
	cfg := acpShimConfig{}
	fs := flag.NewFlagSet(acpShimCommand, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.baseURL, "base-url", "", "OpenAI 兼容端点（必填），如 https://api.deepseek.com")
	fs.StringVar(&cfg.model, "model", "", "模型名（必填），如 deepseek-chat")
	fs.StringVar(&cfg.apiKeyEnv, "api-key-env", "OPENAI_API_KEY", "存放 API key 的环境变量名（宿主经注册表 env 透传注入）")
	fs.StringVar(&cfg.provider, "provider", "generic", "vendor 方言分支：generic|deepseek|grok")
	fs.StringVar(&cfg.system, "system", "", "system prompt（可空）")
	fs.DurationVar(&cfg.heartbeat, "heartbeat", 30*time.Second, "显式心跳周期（0=关；M4 §A 默认 30s）")
	fs.StringVar(&cfg.logLevel, "log-level", "info", "日志级别（走 stderr，stdout 纯净红线）")
	fs.BoolVar(&cfg.fallbackNonStream, "fallback-non-stream", false, "流式失败且未产出增量时回退非流式重发一次（deepseek 档默认开启）")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.baseURL == "" {
		return cfg, fmt.Errorf("--base-url 必填")
	}
	if cfg.model == "" {
		return cfg, fmt.Errorf("--model 必填")
	}
	return cfg, nil
}

func runACPShim(args []string) int {
	cfg, err := parseACPShimArgs(args, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", acpShimCommand, err)
		return 2
	}
	lg := acpshim.NewStderrLogger(acpShimCommand, cfg.logLevel)

	apiKey := os.Getenv(cfg.apiKeyEnv)
	if apiKey == "" {
		lg.Error("API key 未注入（宿主经注册表 env 透传）", loggateway.Str("env", cfg.apiKeyEnv))
		return 2
	}

	provider := openaicompat.ParseProvider(cfg.provider)
	llm := openaicompat.New(cfg.baseURL, apiKey, provider, nil, lg)
	if cfg.fallbackNonStream {
		llm.WithNonStreamFallback(true)
	}
	tools := acpshim.NewRegistry(lg, 0)
	broker := acpshim.NewBroker(lg, 0, acpshim.NewPermissionGuard(0, 0, 0))

	srv, err := acpshim.NewServer(acpshim.Config{
		Provider:          provider.String(),
		Model:             cfg.model,
		SystemPrompt:      cfg.system,
		LLM:               llm,
		Tools:             tools,
		Approver:          broker,
		HeartbeatInterval: cfg.heartbeat,
		LG:                lg,
	})
	if err != nil {
		lg.Error("server init failed", loggateway.Err(err))
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	lg.Info("acp-shim ready",
		loggateway.Str("base_url", cfg.baseURL),
		loggateway.Str("model", cfg.model),
		loggateway.Str("provider", provider.String()))
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		lg.Error("serve failed", loggateway.Err(err))
		return 1
	}
	fmt.Fprintln(os.Stderr, "acp-shim: connection closed, exiting")
	return 0
}
