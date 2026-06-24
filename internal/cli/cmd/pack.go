package cmd

import (
	"fmt"
	"os"

	"aranea-agents/internal/cli"

	"github.com/spf13/cobra"
)

// NewPackCmd creates the `aranea pack` command group.
func NewPackCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "pack",
		Short: "Aranea Pack (.arpack) 导出/导入/校验",
		Long: `管理 Aranea Pack 场景包，支持 Agent、Team、行业场景的导出、导入和校验。

示例:
  aranea pack export --kind agent --ref <agent_id> -o agent.arpack
  aranea pack export --kind team --ref <team_id> -o team.arpack
  aranea pack export --kind industry --ref finance -o finance.arpack
  aranea pack import finance.arpack --strategy skip
  aranea pack validate finance.arpack`,
	}
	c.AddCommand(packExportCmd())
	c.AddCommand(packImportCmd())
	c.AddCommand(packValidateCmd())
	return c
}

func packExportCmd() *cobra.Command {
	var kind string
	var ref string
	var output string

	c := &cobra.Command{
		Use:   "export",
		Short: "导出为 .arpack 场景包",
		RunE: func(cmd *cobra.Command, args []string) error {
			if kind == "" || ref == "" {
				return &cli.CLIError{Code: "MISSING_ARGS", Message: "--kind and --ref are required"}
			}
			cc := cli.CLIFrom(cmd.Context())

			resp, err := cc.Client.PackExport(cmd.Context(), kind, ref)
			if err != nil {
				return err
			}

			outPath := output
			if outPath == "" {
				outPath = resp.Name + ".arpack"
			}
			if err := os.WriteFile(outPath, resp.Data, 0644); err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: fmt.Sprintf("write file failed: %v", err)}
			}
			return cc.Printer.PrintSuccess("Pack 导出成功",
				"kind", kind, "file", outPath, "bytes", fmt.Sprintf("%d", len(resp.Data)))
		},
	}
	c.Flags().StringVar(&kind, "kind", "", "Export granularity: agent, team, or industry")
	c.Flags().StringVar(&ref, "ref", "", "Entity ID or key (agent_id, team_id, industry_key)")
	c.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <name>.arpack)")
	return c
}

func packImportCmd() *cobra.Command {
	var strategy string

	c := &cobra.Command{
		Use:   "import <file.arpack>",
		Short: "导入 .arpack 场景包",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: fmt.Sprintf("read file failed: %v", err)}
			}

			cc := cli.CLIFrom(cmd.Context())

			if strategy == "" {
				strategy = "skip"
			}
			resp, err := cc.Client.PackImport(cmd.Context(), data, strategy)
			if err != nil {
				return err
			}

			return cc.Printer.PrintSuccess("Pack 导入完成",
				"org_nodes", fmt.Sprintf("%d", resp.OrgNodes),
				"agents_created", fmt.Sprintf("%d", resp.AgentsCreated),
				"agents_updated", fmt.Sprintf("%d", resp.AgentsUpdated),
				"agents_skipped", fmt.Sprintf("%d", resp.AgentsSkipped),
				"graphs_created", fmt.Sprintf("%d", resp.GraphsCreated),
				"teams_created", fmt.Sprintf("%d", resp.TeamsCreated),
				"teams_updated", fmt.Sprintf("%d", resp.TeamsUpdated),
				"teams_skipped", fmt.Sprintf("%d", resp.TeamsSkipped),
				"strategy", resp.ConflictStrategy)
		},
	}
	c.Flags().StringVar(&strategy, "strategy", "skip", "Conflict strategy: skip, overwrite, duplicate")
	return c
}

func packValidateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "validate <file.arpack>",
		Short: "校验 .arpack 场景包（dry-run）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: fmt.Sprintf("read file failed: %v", err)}
			}

			cc := cli.CLIFrom(cmd.Context())

			resp, err := cc.Client.PackValidate(cmd.Context(), data)
			if err != nil {
				return err
			}

			valid := "false"
			if resp.Valid {
				valid = "true"
			}
			return cc.Printer.PrintSuccess("Pack 校验完成",
				"valid", valid,
				"errors", fmt.Sprintf("%d", len(resp.Errors)),
				"missing_skills", fmt.Sprintf("%d", len(resp.MissingSkills)),
				"missing_func_refs", fmt.Sprintf("%d", len(resp.MissingFuncRefs)),
				"conflicts", fmt.Sprintf("%d", len(resp.Conflicts)))
		},
	}
	return c
}
