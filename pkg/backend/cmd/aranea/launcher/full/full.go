// Package full 组装驱动 Aranea CLI 运行时半边的 launcher 链。结构仿照
// google.golang.org/adk/cmd/launcher/full，但只注册对当前二进制有意义
// 的 SubLauncher。目前 CLI 随附 console launcher；web/A2A 等是自然扩展点，
// 添加时无需改 main.go。
package full

import (
	"context"

	adklauncher "google.golang.org/adk/cmd/launcher"
	adkuniversal "google.golang.org/adk/cmd/launcher/universal"

	"arenea/backend/cmd/aranea/cli/apiclient"
	araneal "arenea/backend/cmd/aranea/launcher"
	"arenea/backend/cmd/aranea/launcher/console"
	"arenea/backend/cmd/aranea/launcher/web"
)

// BuildConfig 生成供 SubLauncher 使用的 Aranea launcher.Config。解析逻辑
// 与 Cobra 路径一致（标志/环境变量/~/.aranea/config.toml 行为一致），
// 随后将解析得到的 HTTP 客户端注入 launcher 链。
func BuildConfig(_ context.Context) (*araneal.Config, error) {
	g := apiclient.NewGlobalContext()
	if err := g.Resolve(); err != nil {
		return nil, err
	}
	return &araneal.Config{
		Client:  g,
		BaseURL: g.BaseURL,
		Token:   g.Token,
	}, nil
}

// NewLauncher 返回注册了全部 Aranea SubLauncher 的 universal launcher。
// console 最先注册，以便无关键字时 universal.NewLauncher 将其作为默认；
// web 为 前端/25 cli.md §1.4 所述的进程内后端演练环境。
func NewLauncher(cfg *araneal.Config) adklauncher.Launcher {
	return adkuniversal.NewLauncher(
		console.NewLauncher(cfg),
		web.NewLauncher(cfg),
	)
}
