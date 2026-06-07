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
				return fmt.Errorf("--kind and --ref are required")
			}
			cc := cli.CLIFrom(cmd.Context())

			resp, err := cc.Client.PackExport(cmd.Context(), kind, ref)
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}

			outPath := output
			if outPath == "" {
				outPath = resp.Name + ".arpack"
			}
			if err := os.WriteFile(outPath, resp.Data, 0644); err != nil {
				return fmt.Errorf("write file failed: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Exported %s pack to %s (%d bytes)\n", kind, outPath, len(resp.Data))
			return nil
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
				return fmt.Errorf("read file failed: %w", err)
			}

			cc := cli.CLIFrom(cmd.Context())

			if strategy == "" {
				strategy = "skip"
			}
			resp, err := cc.Client.PackImport(cmd.Context(), data, strategy)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Import result:\n")
			fmt.Fprintf(os.Stderr, "  Org nodes: %d\n", resp.OrgNodes)
			fmt.Fprintf(os.Stderr, "  Agents created: %d, updated: %d, skipped: %d\n", resp.AgentsCreated, resp.AgentsUpdated, resp.AgentsSkipped)
			fmt.Fprintf(os.Stderr, "  Graphs created: %d\n", resp.GraphsCreated)
			fmt.Fprintf(os.Stderr, "  Teams created: %d, updated: %d, skipped: %d\n", resp.TeamsCreated, resp.TeamsUpdated, resp.TeamsSkipped)
			fmt.Fprintf(os.Stderr, "  Strategy: %s\n", resp.ConflictStrategy)
			if len(resp.Failures) > 0 {
				fmt.Fprintf(os.Stderr, "  Failures:\n")
				for _, f := range resp.Failures {
					fmt.Fprintf(os.Stderr, "    %s/%s: %s\n", f.EntityType, f.Key, f.Reason)
				}
			}
			return nil
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
				return fmt.Errorf("read file failed: %w", err)
			}

			cc := cli.CLIFrom(cmd.Context())

			resp, err := cc.Client.PackValidate(cmd.Context(), data)
			if err != nil {
				return fmt.Errorf("validate failed: %w", err)
			}

			if resp.Valid {
				fmt.Fprintf(os.Stderr, "Pack is valid.\n")
			} else {
				fmt.Fprintf(os.Stderr, "Pack is INVALID.\n")
			}
			if len(resp.Errors) > 0 {
				fmt.Fprintf(os.Stderr, "Errors:\n")
				for _, e := range resp.Errors {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
			}
			if len(resp.MissingSkills) > 0 {
				fmt.Fprintf(os.Stderr, "Missing skills: %v\n", resp.MissingSkills)
			}
			if len(resp.MissingFuncRefs) > 0 {
				fmt.Fprintf(os.Stderr, "Missing func_refs: %v\n", resp.MissingFuncRefs)
			}
			if len(resp.Conflicts) > 0 {
				fmt.Fprintf(os.Stderr, "Conflicts:\n")
				for _, c := range resp.Conflicts {
					fmt.Fprintf(os.Stderr, "  - %s/%s\n", c.EntityType, c.Key)
				}
			}
			return nil
		},
	}
	return c
}
