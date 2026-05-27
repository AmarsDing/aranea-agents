package cmd

import (
	"fmt"

	toolv1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewToolCmd creates the `aranea tool` command group.
func NewToolCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tool",
		Short: "Tool 管理",
	}
	c.AddCommand(
		toolLsCmd(),
		toolGetCmd(),
		toolEnableCmd(),
		toolDisableCmd(),
	)
	return c
}

func toolLsCmd() *cobra.Command {
	var limit, offset int32
	var search string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出所有 Tool",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListTools(cmd.Context(), search, limit, offset)
			if err != nil {
				return err
			}
			rows := toolsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "page-size", 20, "每页数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&search, "search", "", "搜索关键词")
	return cmd
}

func toolGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看 Tool 详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			tool, err := cc.Client.GetTool(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(toolToRow(tool))
		},
	}
}

func toolEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用 Tool（高风险，需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认启用 Tool %q？启用后 Agent 可调用此工具", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			tool, err := cc.Client.ToggleToolEnabled(cmd.Context(), args[0], true, "")
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Tool 已启用", "id", tool.Id, "key", tool.Key)
		},
	}
}

func toolDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "停用 Tool（高风险，需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认停用 Tool %q？停用后 Agent 无法使用此工具", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			tool, err := cc.Client.ToggleToolEnabled(cmd.Context(), args[0], false, "")
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Tool 已停用", "id", tool.Id, "key", tool.Key)
		},
	}
}

// toolToRow converts a Tool to a display row.
func toolToRow(t *toolv1.Tool) map[string]string {
	if t == nil {
		return nil
	}
	enabled := "false"
	if t.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":           t.Id,
		"key":          t.Key,
		"display_name": t.DisplayName,
		"enabled":      enabled,
		"risk_level":   t.RiskLevel,
	}
}

// toolsToRows converts a slice of Tool to display rows.
func toolsToRows(tools []*toolv1.Tool) []map[string]string {
	rows := make([]map[string]string, 0, len(tools))
	for _, t := range tools {
		rows = append(rows, toolToRow(t))
	}
	return rows
}
