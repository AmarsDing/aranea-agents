package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/orgimport"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewImportCmd creates the `aranea import` command group. PGO-4-CLI-01.
func NewImportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "import",
		Short: "组织导入（行业/部门/岗位/Agent）",
		Long: `从 YAML 或 Markdown 规格文件批量创建行业分类、部门、岗位、Agent 与 Team。

支持的输入格式:
  - YAML spec (*.yaml / *.yml)：直接解析结构化规格
  - Markdown prose (*.md)：通过 AI 提取 YAML 规格再导入

示例:
  aranea import org org.yaml --dry-run
  aranea import org org.md --refine --dry-run
  aranea import org org.yaml --apply`,
	}
	c.AddCommand(importOrgCmd())
	return c
}

func importOrgCmd() *cobra.Command {
	var dryRun bool
	var apply bool
	var refine bool
	var outputSpec string
	var outputFmt string
	var timeout int
	var correlationID string

	cmd := &cobra.Command{
		Use:   "org <spec-file>",
		Short: "从规格文件导入组织结构",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			specFile := args[0]

			// --apply overrides --dry-run.
			if apply {
				dryRun = false
			}

			// ── Stage 1: Load & parse ────────────────────────────────────────
			loaderOpts := orgimport.LoadOptions{
				APIURL:   cc.Cfg.Backend.BaseURL,
				APIToken: cc.Cfg.Backend.Token,
				Timeout:  time.Duration(timeout) * time.Second,
			}
			lower := strings.ToLower(specFile)
			if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
				loaderOpts.ExtractViaAPI = true
				if !cc.Quiet {
					cmd.PrintErrln("ℹ 检测到 Markdown 输入，调用 AI 提取结构化规格...")
				}
			}

			spec, err := orgimport.LoadSpec(specFile, loaderOpts)
			if err != nil {
				return &cli.CLIError{Code: "IMPORT_LOAD_ERROR", Message: "加载规格文件失败", Cause: err}
			}

			// Write extracted YAML if requested.
			if outputSpec != "" {
				raw, _ := yaml.Marshal(spec)
				if err := os.WriteFile(outputSpec, raw, 0644); err != nil {
					return &cli.CLIError{Code: "IMPORT_WRITE_ERROR", Message: "写入 output-spec 失败", Cause: err}
				}
				cmd.PrintErrln("✓ 提取的规格已保存至:", outputSpec)
			}

			// ── Stage 2: Validate ────────────────────────────────────────────
			if err := orgimport.ValidateSpec(spec); err != nil {
				return err
			}
			if !cc.Quiet {
				cmd.PrintErrln("✓ 规格校验通过")
			}

			// ── Stage 3: Plan ────────────────────────────────────────────────
			plan := orgimport.BuildPlan(spec, orgimport.EmptyExistingResources{})

			switch outputFmt {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(plan)
			default:
				cmd.Println(orgimport.FormatPlanTree(plan))
			}

			if dryRun {
				cmd.Println("\n[dry-run] 未写入任何数据。使用 --apply 执行。")
				return nil
			}

			// ── Stage 4: Confirm ─────────────────────────────────────────────
			if !cc.AutoYes {
				cmd.Print("继续执行导入? [y/N] ")
				var answer string
				fmt.Fscan(cmd.InOrStdin(), &answer)
				if strings.ToLower(strings.TrimSpace(answer)) != "y" {
					cmd.Println("已取消。")
					return nil
				}
			}

			// ── Stage 5: Apply ───────────────────────────────────────────────
			applierOpts := orgimport.ApplyOptions{
				APIURL:        cc.Cfg.Backend.BaseURL,
				APIToken:      cc.Cfg.Backend.Token,
				DryRun:        false,
				Refine:        refine,
				Timeout:       time.Duration(timeout) * time.Second,
				CorrelationID: correlationID,
			}
			applier := orgimport.NewApplier(applierOpts)
			result, err := applier.Apply(spec)
			if err != nil {
				return &cli.CLIError{Code: "IMPORT_APPLY_ERROR", Message: "导入执行失败", Cause: err}
			}

			// ── Stage 6: Report ──────────────────────────────────────────────
			cmd.Printf("\n导入完成: 创建 %d, 更新 %d, 跳过 %d\n",
				result.Created, result.Updated, result.Skipped)
			if len(result.Errors) > 0 {
				cmd.Println("警告（部分资源失败）:")
				for _, e := range result.Errors {
					cmd.Println("  ✗", e)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "仅打印计划，不写入（默认 true）")
	cmd.Flags().BoolVar(&apply, "apply", false, "实际写入（覆盖 --dry-run）")
	cmd.Flags().BoolVar(&refine, "refine", false, "对每个 description / agent_description 调用 AI 优化")
	cmd.Flags().StringVar(&outputSpec, "output-spec", "", "保存提取的 YAML 规格到此路径（仅 Markdown 输入时有效）")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "输出格式: text | json")
	cmd.Flags().IntVar(&timeout, "timeout", 120, "每次 HTTP 调用超时（秒）")
	// PGO-4-OBS-01: correlation ID for backend audit traces.
	cmd.Flags().StringVar(&correlationID, "correlation-id", "", "审计追踪 ID（默认自动生成 cli-import-<timestamp>）")
	return cmd
}
