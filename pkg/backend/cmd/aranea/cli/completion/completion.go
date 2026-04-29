// Package completion 将 Cobra 内置的 shell 补全脚本生成器暴露为
// `aranea completion <shell>` 命令树。
package completion

import (
	"os"

	"github.com/spf13/cobra"
)

// NewCommand 返回父级 completion 命令。显式接好 Cobra 支持的四种目标，
// 用户可通过子命令自身 tab 补全发现。
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts (bash|zsh|fish|powershell)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenBashCompletionV2(os.Stdout, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenZshCompletion(os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenFishCompletion(os.Stdout, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		},
	})
	return cmd
}
