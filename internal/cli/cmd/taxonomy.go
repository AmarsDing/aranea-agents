package cmd

import (
	"fmt"
	"strings"

	taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewTaxonomyCmd creates the `aranea taxonomy` command group.
func NewTaxonomyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "taxonomy",
		Short: "分类体系管理",
	}
	c.AddCommand(
		taxonomyLsCmd(),
		taxonomyTreeCmd(),
		taxonomyGetCmd(),
		taxonomyCreateCmd(),
		taxonomyUpdateCmd(),
		taxonomyDeleteCmd(),
		taxonomyReorderCmd(),
	)
	return c
}

func taxonomyLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出所有分类节点",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListTaxonomy(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, n := range resp.Items {
				rows = append(rows, taxonomyNodeToRow(n))
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func taxonomyTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree",
		Short: "以树形展示分类体系",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListTaxonomyTree(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0)
			for _, n := range resp.Items {
				rows = appendTaxonomyTreeRows(rows, n, 0)
			}
			return cc.Printer.PrintList(rows, len(rows))
		},
	}
}

func taxonomyGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看分类节点详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.GetTaxonomy(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(taxonomyNodeToRow(node))
		},
	}
}

func taxonomyCreateCmd() *cobra.Command {
	var key, name, level, parentID, description string
	cmd := &cobra.Command{
		Use:   "create --key <key> --name <name>",
		Short: "创建分类节点",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.CreateTaxonomy(cmd.Context(), &taxonomyv1.CreateTaxonomyRequest{
				Key:         key,
				Name:        name,
				Level:       level,
				ParentId:    parentID,
				Description: description,
			})
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("分类节点创建成功", "id", node.Id, "key", node.Key)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "分类节点唯一标识")
	cmd.Flags().StringVar(&name, "name", "", "分类节点名称")
	cmd.Flags().StringVar(&level, "level", "", "层级（自由文本）")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "父节点 ID")
	cmd.Flags().StringVar(&description, "description", "", "描述")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func taxonomyUpdateCmd() *cobra.Command {
	var key, name, level, parentID, description string
	var sortOrder int32
	var enabled bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "更新分类节点（仅更新指定 flag 的字段）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.GetTaxonomy(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if f.Changed("key") {
				node.Key = key
			}
			if f.Changed("name") {
				node.Name = name
			}
			if f.Changed("level") {
				node.Level = level
			}
			if f.Changed("parent-id") {
				node.ParentId = parentID
			}
			if f.Changed("description") {
				node.Description = description
			}
			if f.Changed("sort-order") {
				node.SortOrder = sortOrder
			}
			if f.Changed("enabled") {
				node.Enabled = enabled
			}
			updated, err := cc.Client.UpdateTaxonomy(cmd.Context(), args[0], node)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("分类节点更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "分类节点唯一标识")
	cmd.Flags().StringVar(&name, "name", "", "分类节点名称")
	cmd.Flags().StringVar(&level, "level", "", "层级（自由文本）")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "父节点 ID")
	cmd.Flags().StringVar(&description, "description", "", "描述")
	cmd.Flags().Int32Var(&sortOrder, "sort-order", 0, "排序序号")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "是否启用")
	return cmd
}

func taxonomyDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除分类节点",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除分类节点 %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteTaxonomy(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("分类节点已删除", "id", args[0])
		},
	}
}

func taxonomyReorderCmd() *cobra.Command {
	var idsCSV string
	cmd := &cobra.Command{
		Use:   "reorder --ids <id1,id2,...>",
		Short: "调整分类节点排序",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := splitCSV(idsCSV)
			if len(ids) == 0 {
				return &cli.CLIError{Code: "INVALID_IDS", Message: "--ids 不能为空，格式：id1,id2,..."}
			}
			cc := cli.CLIFrom(cmd.Context())
			if err := cc.Client.ReorderTaxonomy(cmd.Context(), ids); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("分类节点排序已更新", "count", fmt.Sprintf("%d", len(ids)))
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "逗号分隔的节点 ID 列表")
	return cmd
}

// appendTaxonomyTreeRows 将分类树先序展开为带缩进的行。
func appendTaxonomyTreeRows(rows []map[string]string, n *taxonomyv1.TaxonomyTreeNode, depth int) []map[string]string {
	if n == nil || n.Node == nil {
		return rows
	}
	row := taxonomyNodeToRow(n.Node)
	row["name"] = strings.Repeat("  ", depth) + n.Node.Name
	rows = append(rows, row)
	for _, ch := range n.Children {
		rows = appendTaxonomyTreeRows(rows, ch, depth+1)
	}
	return rows
}

// taxonomyNodeToRow converts a TaxonomyNode to a display row.
func taxonomyNodeToRow(n *taxonomyv1.TaxonomyNode) map[string]string {
	if n == nil {
		return nil
	}
	enabled := "false"
	if n.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":         n.Id,
		"key":        n.Key,
		"name":       n.Name,
		"level":      n.Level,
		"parent_id":  n.ParentId,
		"enabled":    enabled,
		"sort_order": fmt.Sprintf("%d", n.SortOrder),
		"status":     n.Status,
	}
}
