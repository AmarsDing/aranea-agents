package cmd

import (
	"fmt"
	"strconv"

	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewEvaluationCmd creates the `aranea eval` command group.
func NewEvaluationCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "eval",
		Short: "评测管理（数据集 / 运行 / 结果）",
	}
	c.AddCommand(
		evalDatasetsCmd(),
		evalRunsCmd(),
		evalResultsCmd(),
	)
	return c
}

func evalDatasetsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "datasets",
		Short: "评测数据集管理",
	}
	c.AddCommand(
		evalDatasetsLsCmd(),
		evalDatasetsGetCmd(),
		evalDatasetsCreateCmd(),
	)
	return c
}

func evalDatasetsLsCmd() *cobra.Command {
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出评测数据集",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListDatasets(cmd.Context(), limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(evalDatasetsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func evalDatasetsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看评测数据集详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			ds, err := cc.Client.GetDataset(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(evalDatasetToRow(ds))
		},
	}
}

func evalDatasetsCreateCmd() *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建评测数据集",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &evaluationv1.CreateDatasetRequest{
				Name:        name,
				Description: description,
			}
			ds, err := cc.Client.CreateDataset(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("评测数据集创建成功", "id", ds.Id, "name", ds.Name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "数据集名称")
	cmd.Flags().StringVar(&description, "description", "", "数据集描述")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func evalRunsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "runs",
		Short: "评测运行管理",
	}
	c.AddCommand(
		evalRunsLsCmd(),
		evalRunsGetCmd(),
		evalRunsCreateCmd(),
	)
	return c
}

func evalRunsLsCmd() *cobra.Command {
	var datasetID, agentID string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出评测运行",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListEvalRuns(cmd.Context(), datasetID, agentID, limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(evalRunsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "按数据集 ID 过滤")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "按 Agent ID 过滤")
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func evalRunsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看评测运行详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			run, err := cc.Client.GetEvalRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(evalRunToRow(run))
		},
	}
}

func evalRunsCreateCmd() *cobra.Command {
	var datasetID, agentID, metrics string
	var numRuns int32
	cmd := &cobra.Command{
		Use:   "create",
		Short: "发起一次评测运行",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &evaluationv1.RunEvaluationRequest{
				DatasetId: datasetID,
				AgentId:   agentID,
				Metrics:   metrics,
				NumRuns:   numRuns,
			}
			run, err := cc.Client.RunEvaluation(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("评测运行已创建", "id", run.Id, "status", run.Status)
		},
	}
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "数据集 ID")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID")
	cmd.Flags().StringVar(&metrics, "metrics", "", "逗号分隔的指标列表（空 = 全部）")
	cmd.Flags().Int32Var(&numRuns, "num-runs", 1, "每个用例重复次数")
	_ = cmd.MarkFlagRequired("dataset-id")
	_ = cmd.MarkFlagRequired("agent-id")
	return cmd
}

func evalResultsCmd() *cobra.Command {
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "results <run-id>",
		Short: "查看评测运行的用例结果",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.GetRunResults(cmd.Context(), args[0], limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(evalCaseResultsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

// Row helpers convert proto items to display rows.

func evalDatasetToRow(d *evaluationv1.EvalDataset) map[string]string {
	if d == nil {
		return nil
	}
	return map[string]string{
		"id":         d.Id,
		"name":       d.Name,
		"case_count": fmt.Sprintf("%d", d.CaseCount),
		"workspace":  d.Workspace,
		"created_at": d.CreatedAt,
	}
}

func evalDatasetsToRows(items []*evaluationv1.EvalDataset) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, d := range items {
		rows = append(rows, evalDatasetToRow(d))
	}
	return rows
}

func evalRunToRow(r *evaluationv1.EvalRun) map[string]string {
	if r == nil {
		return nil
	}
	return map[string]string{
		"id":                 r.Id,
		"dataset_id":         r.DatasetId,
		"agent_id":           r.AgentId,
		"status":             r.Status,
		"progress":           fmt.Sprintf("%d/%d", r.CompletedCases, r.TotalCases),
		"exact_match_score":  formatScore(r.ExactMatchScore),
		"llm_judge_score":    formatScore(r.LlmJudgeScore),
		"tool_call_accuracy": formatScore(r.ToolCallAccuracy),
		"created_at":         r.CreatedAt,
	}
}

func evalRunsToRows(items []*evaluationv1.EvalRun) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, r := range items {
		rows = append(rows, evalRunToRow(r))
	}
	return rows
}

func evalCaseResultsToRows(items []*evaluationv1.EvalCaseResult) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, r := range items {
		exactMatch := "false"
		if r.ExactMatch {
			exactMatch = "true"
		}
		containsMatch := "false"
		if r.ContainsMatch {
			containsMatch = "true"
		}
		rows = append(rows, map[string]string{
			"id":              r.Id,
			"case_id":         r.CaseId,
			"exact_match":     exactMatch,
			"contains_match":  containsMatch,
			"llm_judge_score": formatScore(r.LlmJudgeScore),
			"error_message":   r.ErrorMessage,
		})
	}
	return rows
}

func formatScore(f float32) string {
	return strconv.FormatFloat(float64(f), 'f', 3, 32)
}
