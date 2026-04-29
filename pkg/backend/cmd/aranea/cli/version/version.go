// Package version 实现 `aranea version`。打印本地 CLI 构建的简要信息，
// 在可达时还会打印后端报告版本。构建流水线尚未通过 -ldflags 注入
// Git 元数据，故暂未嵌入；日后可在不改动本命令对外的行为的前提下补充。
package version

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
)

// CLIVersion 是 `aranea version` 打印的语义化版本。CI 构建中可通过
// -ldflags "-X .CLIVersion=..." 在链接时覆盖。
var CLIVersion = "0.1.0-dev"

// NewCommand 返回 `aranea version` 的 cobra.Command。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print client and backend version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := struct {
				Client struct {
					Version string `json:"version"`
					GoOS    string `json:"go_os"`
					GoArch  string `json:"go_arch"`
					Go      string `json:"go"`
				} `json:"client"`
				Backend struct {
					Reachable bool   `json:"reachable"`
					BaseURL   string `json:"base_url"`
					Status    string `json:"status,omitempty"`
				} `json:"backend"`
			}{}
			info.Client.Version = CLIVersion
			info.Client.GoOS = runtime.GOOS
			info.Client.GoArch = runtime.GOARCH
			info.Client.Go = runtime.Version()
			info.Backend.BaseURL = g.BaseURL

			// 尽力探测后端；不应因此使命令失败。
			var probe map[string]string
			if err := g.Client().Get(cmd.Context(), "/healthz", nil, &probe); err == nil {
				info.Backend.Reachable = true
				info.Backend.Status = probe["status"]
			}

			if output.Format() == "text" {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "aranea %s (%s/%s, %s)\n", info.Client.Version, info.Client.GoOS, info.Client.GoArch, info.Client.Go)
				if info.Backend.Reachable {
					fmt.Fprintf(w, "backend: %s [%s]\n", info.Backend.BaseURL, info.Backend.Status)
				} else {
					fmt.Fprintf(w, "backend: %s [unreachable]\n", info.Backend.BaseURL)
				}
				return nil
			}
			output.Render(cmd.OutOrStdout(), info)
			return nil
		},
	}
}
