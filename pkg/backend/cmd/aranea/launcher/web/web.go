// Package web 实现 Aranea 的 ADK SubLauncher，在进程内启动后端 HTTP 服务。
// 结构仿照 google.golang.org/adk/cmd/launcher/web，但实际服务装配委托给
// arenea/backend/internal/app.Run，使独立 `aranea-server` 二进制与内嵌
// launcher 行为一致。对应 前端/25 cli.md §1.4 / §5 —— 通过 `aranea web`
// 关键字启动本地演练环境。
package web

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	adklauncher "google.golang.org/adk/cmd/launcher"

	araneal "arenea/backend/cmd/aranea/launcher"
	"arenea/backend/internal/app"
)

// NewLauncher 构建 web SubLauncher。cfg 被捕获以读取解析后的后端配置
//（HTTP 地址覆盖、自定义 DB 路径）而不去改动 ADK 的 launcher.Config 结构体。
func NewLauncher(cfg *araneal.Config) adklauncher.SubLauncher {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	c := &webConfig{}
	flags.StringVar(&c.Addr, "addr", "", "HTTP listen address (default: HTTP_ADDR env or :8080)")
	flags.StringVar(&c.DBPath, "db", "", "SQLite database file (default: DB_PATH env or data/arenea.db)")
	flags.BoolVar(&c.Quiet, "quiet", false, "Suppress server logs (the ready banner still prints)")
	return &webLauncher{flags: flags, config: c, arn: cfg}
}

// webConfig 保存 web launcher 解析后的 CLI 标志。
type webConfig struct {
	Addr   string
	DBPath string
	Quiet  bool
}

// webLauncher 实现 adklauncher.SubLauncher。
type webLauncher struct {
	flags  *flag.FlagSet
	config *webConfig
	arn    *araneal.Config
}

// Keyword 实现 adklauncher.SubLauncher。
func (l *webLauncher) Keyword() string { return "web" }

// SimpleDescription 实现 adklauncher.SubLauncher。
func (l *webLauncher) SimpleDescription() string {
	return "boot the Aranea backend HTTP server (admin REST + chat SSE) in-process"
}

// CommandLineSyntax 实现 adklauncher.SubLauncher。自行渲染 flag 集，避免
// 依赖 adk 内部 cli/util 包。
func (l *webLauncher) CommandLineSyntax() string {
	var buf bytes.Buffer
	buf.WriteString("Flags:\n")
	l.flags.SetOutput(&buf)
	l.flags.PrintDefaults()
	return buf.String()
}

// Parse 实现 adklauncher.SubLauncher。
func (l *webLauncher) Parse(args []string) ([]string, error) {
	if err := l.flags.Parse(args); err != nil {
		return nil, fmt.Errorf("web: %w", err)
	}
	return l.flags.Args(), nil
}

// Run 实现 adklauncher.SubLauncher。在 ADK launcher 生命周期与
// arenea/backend/internal/app.Run 之间桥接，负责：
//
//   - 解析 DB 路径（标志 → CLI 配置 → 环境变量 → 默认）
//   - 在设置 --quiet 时静默服务日志
//   - 通过 Ready 通道发布实际监听的地址，以便打印单条准确横幅（
//     在 --addr=:0 时指向实际端口尤为重要）。
func (l *webLauncher) Run(ctx context.Context, _ *adklauncher.Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	logger := log.Default()
	if l.config.Quiet {
		logger = log.New(quietWriter{}, "", 0)
	}

	ready := make(chan app.ListenInfo, 1)
	go func() {
		select {
		case info := <-ready:
			fmt.Fprintf(os.Stdout, "aranea backend ready on http://%s\n", info.Addr)
		case <-ctx.Done():
		}
	}()

	return app.Run(ctx, app.ServerOptions{
		Addr:          l.config.Addr,
		DBPath:        l.config.DBPath,
		SkipTelemetry: false,
		Ready:         ready,
		Logger:        logger,
	})
}

// quietWriter 在 --quiet 时吞掉服务日志输出，而无需 import io/ioutil。
type quietWriter struct{}

func (quietWriter) Write(p []byte) (int, error) { return len(p), nil }
