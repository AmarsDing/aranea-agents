package cmd

import (
	"fmt"
	"strconv"
	"strings"

	modelcatalogv1 "aranea-agents/api/kratos/model_catalog/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// modelCatalogSyncPolicies 是 sync_policy 的合法取值（见 proto 注释）。
var modelCatalogSyncPolicies = []string{"off", "scheduled"}

// modelCatalogAutoApplyModes 是 auto_apply 的合法取值（见 proto 注释）。
var modelCatalogAutoApplyModes = []string{"none", "metadata_and_pricing", "full_spec", "full_spec_and_runtime_overlay"}

// NewModelCatalogCmd creates the `aranea model-catalog` command group.
func NewModelCatalogCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "model-catalog",
		Short: "模型目录管理",
	}
	c.AddCommand(
		modelCatalogLsCmd(),
		modelCatalogGetCmd(),
		modelCatalogPolicyCmd(),
		modelCatalogPolicySetCmd(),
		modelCatalogSyncCmd(),
	)
	return c
}

func modelCatalogLsCmd() *cobra.Command {
	var q string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出模型目录中的 Provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCatalogProviders(cmd.Context(), q, limit, offset)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, p := range resp.Items {
				rows = append(rows, map[string]string{
					"id":          p.Id,
					"name":        p.Name,
					"model_count": fmt.Sprintf("%d", p.ModelCount),
					"api":         p.Api,
					"doc":         p.Doc,
				})
			}
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "搜索关键词")
	cmd.Flags().Int32Var(&limit, "limit", 0, "返回数量上限（0 表示服务端默认）")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func modelCatalogGetCmd() *cobra.Command {
	var q string
	var includeDeprecated bool
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "get <provider-id>",
		Short: "查看指定 Provider 的模型列表",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCatalogModels(cmd.Context(), args[0], q, includeDeprecated, limit, offset)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, m := range resp.Items {
				rows = append(rows, map[string]string{
					"id":                m.Id,
					"name":              m.Name,
					"status":            m.Status,
					"context_tokens":    fmt.Sprintf("%d", m.ContextTokens),
					"input_usd_per_1m":  strconv.FormatFloat(m.InputUsdPer_1M, 'f', -1, 64),
					"output_usd_per_1m": strconv.FormatFloat(m.OutputUsdPer_1M, 'f', -1, 64),
				})
			}
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "搜索关键词")
	cmd.Flags().BoolVar(&includeDeprecated, "include-deprecated", false, "包含已废弃模型")
	cmd.Flags().Int32Var(&limit, "limit", 0, "返回数量上限（0 表示服务端默认）")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func modelCatalogPolicyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "policy",
		Short: "查看模型目录同步策略",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			policy, err := cc.Client.GetModelCatalogPolicy(cmd.Context())
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(modelCatalogPolicyToRow(policy))
		},
	}
}

func modelCatalogPolicySetCmd() *cobra.Command {
	var sourceURL, syncPolicy, autoApply string
	var syncIntervalHours int32
	cmd := &cobra.Command{
		Use:   "policy-set",
		Short: "更新模型目录同步策略（仅更新指定 flag 的字段）",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if f.Changed("sync-policy") && !isOneOf(syncPolicy, modelCatalogSyncPolicies) {
				return &cli.CLIError{Code: "INVALID_SYNC_POLICY", Message: fmt.Sprintf("sync-policy 必须是 %s 之一", strings.Join(modelCatalogSyncPolicies, "|"))}
			}
			if f.Changed("auto-apply") && !isOneOf(autoApply, modelCatalogAutoApplyModes) {
				return &cli.CLIError{Code: "INVALID_AUTO_APPLY", Message: fmt.Sprintf("auto-apply 必须是 %s 之一", strings.Join(modelCatalogAutoApplyModes, "|"))}
			}
			cc := cli.CLIFrom(cmd.Context())
			policy, err := cc.Client.GetModelCatalogPolicy(cmd.Context())
			if err != nil {
				return err
			}
			req := &modelcatalogv1.UpdateModelCatalogPolicyRequest{
				SourceUrl:         policy.SourceUrl,
				SyncPolicy:        policy.SyncPolicy,
				SyncIntervalHours: policy.SyncIntervalHours,
				AutoApply:         policy.AutoApply,
			}
			if f.Changed("source-url") {
				req.SourceUrl = sourceURL
			}
			if f.Changed("sync-policy") {
				req.SyncPolicy = syncPolicy
			}
			if f.Changed("sync-interval-hours") {
				req.SyncIntervalHours = syncIntervalHours
			}
			if f.Changed("auto-apply") {
				req.AutoApply = autoApply
			}
			updated, err := cc.Client.UpdateModelCatalogPolicy(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("模型目录策略已更新", "sync_policy", updated.SyncPolicy, "auto_apply", updated.AutoApply)
		},
	}
	cmd.Flags().StringVar(&sourceURL, "source-url", "", "目录来源 URL")
	cmd.Flags().StringVar(&syncPolicy, "sync-policy", "", "同步策略：off|scheduled")
	cmd.Flags().Int32Var(&syncIntervalHours, "sync-interval-hours", 0, "同步间隔（小时）")
	cmd.Flags().StringVar(&autoApply, "auto-apply", "", "自动应用级别：none|metadata_and_pricing|full_spec|full_spec_and_runtime_overlay")
	return cmd
}

func modelCatalogSyncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "触发模型目录同步",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.SyncModelCatalog(cmd.Context(), dryRun)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("模型目录同步已触发",
				"ok", strconv.FormatBool(resp.Ok),
				"message", resp.Message,
				"log_id", resp.LogId,
			)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅预演，不实际写入")
	return cmd
}

// modelCatalogPolicyToRow converts a ModelCatalogPolicy to a display row.
func modelCatalogPolicyToRow(p *modelcatalogv1.ModelCatalogPolicy) map[string]string {
	if p == nil {
		return nil
	}
	return map[string]string{
		"source_url":          p.SourceUrl,
		"sync_policy":         p.SyncPolicy,
		"sync_interval_hours": fmt.Sprintf("%d", p.SyncIntervalHours),
		"auto_apply":          p.AutoApply,
	}
}
