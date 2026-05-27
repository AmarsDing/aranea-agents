package cmd

import (
	"fmt"
	"os"
	"time"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/pkginstall"
	"github.com/spf13/cobra"
)

// NewPkgCmd creates the `aranea pkg` command group.
func NewPkgCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "pkg",
		Short: "Package 安装管理（从 URL 安装 aranea-package）",
	}
	c.AddCommand(
		pkgInstallCmd(),
		pkgValidateCmd(),
	)
	return c
}

func pkgInstallCmd() *cobra.Command {
	var (
		ref      string
		decision string
		dryRun   bool
		strict   bool
		keepTemp bool
		timeout  int
	)
	cmd := &cobra.Command{
		Use:   "install <url>",
		Short: "从 URL 安装 aranea package",
		Long: `从 Git 仓库 URL 安装 aranea package。

安装顺序：
  [1/6] MCP 服务器
  [2/6] Skills
  [3/6] 行业/部门/岗位
  [4/6] Agents
  [5/6] Teams
  [6/6] Graphs

示例：
  aranea pkg install https://github.com/example/my-package
  aranea pkg install https://github.com/example/my-package --ref v1.0 --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			url := args[0]
			jsonOutput := cc.Cfg != nil && cc.Cfg.UI.Output == "json"

			// Clone the package.
			if !cc.Quiet && !jsonOutput {
				fmt.Fprintf(os.Stdout, "正在克隆 %s ...\n", url)
			}
			pkgDir, cleanup, err := pkginstall.FetchFromURL(url, ref, cc.Quiet)
			if err != nil {
				return &cli.CLIError{Code: "PKG_FETCH_ERROR", Message: err.Error()}
			}
			if !keepTemp {
				defer cleanup()
			} else if !jsonOutput {
				fmt.Fprintf(os.Stdout, "临时目录保留在：%s\n", pkgDir)
			}

			// Load manifest.
			manifest, err := pkginstall.LoadManifestFromDir(pkgDir)
			if err != nil {
				return &cli.CLIError{Code: "PKG_MANIFEST_ERROR", Message: err.Error()}
			}
			if err := pkginstall.ValidateManifest(manifest); err != nil {
				return &cli.CLIError{Code: "PKG_MANIFEST_INVALID", Message: err.Error()}
			}

			if !jsonOutput {
				fmt.Fprintf(os.Stdout, "Package: %s v%s — %s\n",
					manifest.Metadata.Name, manifest.Metadata.Version, manifest.Metadata.Description)
			}
			if dryRun && !jsonOutput {
				fmt.Fprintln(os.Stdout, "[dry-run] 不执行实际安装")
			}

			// Override decision for all skills if provided.
			if decision != "" {
				for i := range manifest.Spec.Skills {
					if manifest.Spec.Skills[i].Decision == "" {
						manifest.Spec.Skills[i].Decision = decision
					}
				}
			}

			ins := &pkginstall.Installer{
				APIURL:  cc.Cfg.Backend.BaseURL,
				Token:   cc.Cfg.Backend.Token,
				DryRun:  dryRun,
				Strict:  strict,
				Quiet:   cc.Quiet,
				Timeout: time.Duration(timeout) * time.Second,
				OnStep: func(step, total int, name, status string) {
					if !cc.Quiet && !jsonOutput {
						fmt.Fprintf(os.Stdout, "[%d/%d] %-20s %s\n", step, total, name, status)
					}
				},
			}

			result, err := ins.Install(pkgDir, manifest)
			if result == nil && err != nil {
				return &cli.CLIError{Code: "PKG_INSTALL_ERROR", Message: err.Error()}
			}

			if result != nil && jsonOutput {
				if printErr := cc.Printer.PrintDetail(result); printErr != nil && err == nil {
					err = printErr
				}
			} else if result != nil && !cc.Quiet {
				fmt.Fprintf(os.Stdout, "\n安装完成：创建 %d，更新 %d，跳过 %d\n",
					result.Created, result.Updated, result.Skipped)
				if len(result.Errors) > 0 {
					fmt.Fprintf(os.Stdout, "警告（%d 个错误）:\n", len(result.Errors))
					for _, e := range result.Errors {
						fmt.Fprintf(os.Stdout, "  ✗ %s\n", e)
					}
				}
			}
			if err != nil {
				return &cli.CLIError{Code: "PKG_INSTALL_ERROR", Message: err.Error()}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Git 分支/Tag（默认使用仓库默认分支）")
	cmd.Flags().StringVar(&decision, "decision", "", "冲突策略：skip|keep|refine")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅预览，不执行实际安装")
	cmd.Flags().BoolVar(&strict, "strict", true, "安装出现任何错误时返回非 0（可用 --strict=false 关闭）")
	cmd.Flags().BoolVar(&keepTemp, "keep-temp", false, "安装后保留临时目录")
	cmd.Flags().IntVar(&timeout, "timeout", 180, "超时秒数")
	return cmd
}

func pkgValidateCmd() *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "validate <url>",
		Short: "仅校验 manifest，不安装",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			url := args[0]

			pkgDir, cleanup, err := pkginstall.FetchFromURL(url, ref, cc.Quiet)
			if err != nil {
				return &cli.CLIError{Code: "PKG_FETCH_ERROR", Message: err.Error()}
			}
			defer cleanup()

			manifest, err := pkginstall.LoadManifestFromDir(pkgDir)
			if err != nil {
				return &cli.CLIError{Code: "PKG_MANIFEST_ERROR", Message: err.Error()}
			}
			if err := pkginstall.ValidateManifest(manifest); err != nil {
				return &cli.CLIError{Code: "PKG_MANIFEST_INVALID", Message: err.Error()}
			}

			return cc.Printer.PrintSuccess("Manifest 校验通过",
				"name", manifest.Metadata.Name,
				"version", manifest.Metadata.Version,
				"mcp_servers", fmt.Sprintf("%d", len(manifest.Spec.MCPServers)),
				"skills", fmt.Sprintf("%d", len(manifest.Spec.Skills)),
				"industries", fmt.Sprintf("%d", len(manifest.Spec.Industries)),
				"agents", fmt.Sprintf("%d", len(manifest.Spec.Agents)),
				"teams", fmt.Sprintf("%d", len(manifest.Spec.Teams)),
				"graphs", fmt.Sprintf("%d", len(manifest.Spec.Graphs)),
			)
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Git 分支/Tag")
	return cmd
}
