package cmd

import (
	"fmt"

	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewPluginCmd creates the `aranea plugin` command group.
func NewPluginCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "plugin",
		Short: "Plugin 管理",
	}
	c.AddCommand(
		pluginLsCmd(),
		pluginEnableCmd(),
		pluginDisableCmd(),
		pluginOrderSetCmd(),
		pluginConfigSetCmd(),
	)
	return c
}

func pluginLsCmd() *cobra.Command {
	var search, category string
	var page, pageSize int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出所有 Plugin",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListPlugins(cmd.Context(), search, category, page, pageSize)
			if err != nil {
				return err
			}
			rows := pluginsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "搜索关键词")
	cmd.Flags().StringVar(&category, "category", "", "分类过滤")
	cmd.Flags().Int32Var(&page, "page", 1, "页码")
	cmd.Flags().Int32Var(&pageSize, "page-size", 20, "每页数量")
	return cmd
}

func pluginEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用 Plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			plugin, err := cc.Client.TogglePluginEnabled(cmd.Context(), args[0], true)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Plugin 已启用", "name", plugin.Name, "enabled", "true")
		},
	}
}

func pluginDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <id>",
		Short: "禁用 Plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认禁用 Plugin %q？", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			plugin, err := cc.Client.TogglePluginEnabled(cmd.Context(), args[0], false)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Plugin 已禁用", "name", plugin.Name, "enabled", "false")
		},
	}
	return cmd
}

func pluginOrderSetCmd() *cobra.Command {
	var order int32
	cmd := &cobra.Command{
		Use:   "order-set <id>",
		Short: "设置 Plugin 排序",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			plugin, err := cc.Client.UpdatePluginSortOrder(cmd.Context(), args[0], order)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Plugin 排序已更新", "name", plugin.Name, "sort_order", fmt.Sprintf("%d", plugin.SortOrder))
		},
	}
	cmd.Flags().Int32Var(&order, "order", 0, "排序值")
	_ = cmd.MarkFlagRequired("order")
	return cmd
}

func pluginConfigSetCmd() *cobra.Command {
	var configJSON string
	cmd := &cobra.Command{
		Use:   "config-set <id>",
		Short: "设置 Plugin 配置 JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			plugin, err := cc.Client.UpdatePluginConfig(cmd.Context(), args[0], configJSON)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Plugin 配置已更新", "name", plugin.Name)
		},
	}
	cmd.Flags().StringVar(&configJSON, "config", "{}", "配置 JSON 字符串")
	return cmd
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func pluginsToRows(items []*pluginv1.Plugin) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, p := range items {
		enabled := "false"
		if p.Enabled {
			enabled = "true"
		}
		rows = append(rows, map[string]string{
			"id":       p.Id,
			"key":      p.Key,
			"name":     p.Name,
			"category": p.Category,
			"enabled":  enabled,
		})
	}
	return rows
}
