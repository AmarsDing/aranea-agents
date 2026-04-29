// Package launcher 承载 Aranea 在 ADK launcher.Config 周边的粘合逻辑。
// console / web 等子 launcher 位于子包，并对 *launcher.Config（ADK 类型）消费，
// 以与 universal.NewLauncher 兼容。
//
// Aranea 的设计（前端/25 cli.md §1.4）在 ADK 配置之外扩展了远程 CLI
// 所需的字段：后端基址、bearer token 以及可选的进程内嵌入标志。
// Launcher 通过闭包接收扩展后的 Config，而不去改动 ADK 结构体本身。
package launcher

import (
	adklauncher "google.golang.org/adk/cmd/launcher"

	"arenea/backend/cmd/aranea/cli/apiclient"
)

// Config 携带 Aranea launcher 链运行所需的全部信息。大部分即 ADK
// 的 launcher.Config（universal/console 所期望）；Aranea 专用字段为附加。
type Config struct {
	Adk      adklauncher.Config
	Client   *apiclient.GlobalContext
	BaseURL  string
	Token    string
	Embedded bool
}

// ADK 返回内嵌的 ADK 配置，便于调用方传给 adklauncher.Launcher.Execute，
// 而无需将 Aranea 类型泄露给 ADK。
func (c *Config) ADK() *adklauncher.Config { return &c.Adk }
