// Aranea CLI 入口点。
//
// 本二进制遵循 前端/25 cli.md §1 中记录的双层命令模型，布局仿照
// google/adk-go 的 `adkgo` 与 `adk` 二进制：
//
//   * Cobra 管理管理类子命令（agent / skill / tool / ...），
//     与 /api/v1/* 下既有 Aranea REST 端点一一对应。
//   * ADK 的 launcher.Launcher / launcher.SubLauncher 管理运行时模式
//     （`console` REPL、`web` 等）。当第一个位置参数为 launcher 关键字时
//     会绕过 Cobra 层，将请求交给 launcher 链，以便用 ADK 自身的
//     session / runner / agent 抽象驱动智能体运行时。
//
// 无参数时 `aranea` 默认使用 `console` launcher，与希望直接与系统管理
// 员智能体（`__system_admin__`）对话的用户预期一致。
package main

import (
	"context"
	"fmt"
	"os"

	"arenea/backend/cmd/aranea/cli"
	"arenea/backend/cmd/aranea/launcher/full"
)

// launcherKeywords 是应路由到 ADK launcher 链而非 Cobra 的首参标记。
// 与 launcher/{console,web,...} 下各 SubLauncher 实现所注册
// 的关键字一致。
var launcherKeywords = map[string]struct{}{
	"console": {},
	"web":     {},
}

func main() {
	args := os.Args[1:]

	// 当首参为已知 launcher 关键字，或没有提供任何参数时（默认交互式
	// console），路由到 ADK launcher 链。
	if shouldRouteToLauncher(args) {
		if err := runLauncher(args); err != nil {
			fmt.Fprintln(os.Stderr, "aranea:", err)
			os.Exit(1)
		}
		return
	}

	// 否则由 Cobra 处理。cobra.Command.Execute 会自行写入错误信息，
	// 此处仅传播退出码。
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

func shouldRouteToLauncher(args []string) bool {
	if len(args) == 0 {
		return true
	}
	first := args[0]
	_, ok := launcherKeywords[first]
	return ok
}

func runLauncher(args []string) error {
	ctx := context.Background()
	cfg, err := full.BuildConfig(ctx)
	if err != nil {
		return fmt.Errorf("build launcher config: %w", err)
	}
	if len(args) == 0 {
		args = []string{"console"}
	}
	l := full.NewLauncher(cfg)
	return l.Execute(ctx, cfg.ADK(), args)
}
