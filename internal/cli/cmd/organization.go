package cmd

import (
	"fmt"
	"strings"

	organizationv1 "aranea-agents/api/kratos/organization/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// orgLevels 是组织节点合法的 level 取值。
var orgLevels = []string{"company", "department", "position"}

// NewOrganizationCmd creates the `aranea org` command group.
func NewOrganizationCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "org",
		Short: "组织架构管理",
	}
	c.AddCommand(
		orgLsCmd(),
		orgTreeCmd(),
		orgGetCmd(),
		orgCreateCmd(),
		orgUpdateCmd(),
		orgDeleteCmd(),
		orgReorderCmd(),
	)
	return c
}

func orgLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出所有组织节点",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListOrganization(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, n := range resp.Items {
				rows = append(rows, orgNodeToRow(n))
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func orgTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree",
		Short: "以树形展示组织架构",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListOrganizationTree(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0)
			for _, n := range resp.Items {
				rows = appendOrgTreeRows(rows, n, 0)
			}
			return cc.Printer.PrintList(rows, len(rows))
		},
	}
}

func orgGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看组织节点详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.GetOrganization(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(orgNodeToRow(node))
		},
	}
}

func orgCreateCmd() *cobra.Command {
	var key, name, level, parentID, description string
	cmd := &cobra.Command{
		Use:   "create --key <key> --name <name> --level <company|department|position>",
		Short: "创建组织节点",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isValidOrgLevel(level) {
				return &cli.CLIError{Code: "INVALID_LEVEL", Message: fmt.Sprintf("level 必须是 %s 之一", strings.Join(orgLevels, "|"))}
			}
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.CreateOrganization(cmd.Context(), &organizationv1.CreateOrganizationRequest{
				OrgKey:      key,
				Name:        name,
				Level:       level,
				ParentId:    parentID,
				Description: description,
			})
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("组织节点创建成功", "id", node.Id, "org_key", node.OrgKey)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "组织节点唯一标识")
	cmd.Flags().StringVar(&name, "name", "", "组织节点名称")
	cmd.Flags().StringVar(&level, "level", "", "层级：company|department|position")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "父节点 ID")
	cmd.Flags().StringVar(&description, "description", "", "描述")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("level")
	return cmd
}

func orgUpdateCmd() *cobra.Command {
	var key, name, level, parentID, description string
	var sortOrder int32
	var enabled bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "更新组织节点（仅更新指定 flag 的字段）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if f.Changed("level") && !isValidOrgLevel(level) {
				return &cli.CLIError{Code: "INVALID_LEVEL", Message: fmt.Sprintf("level 必须是 %s 之一", strings.Join(orgLevels, "|"))}
			}
			cc := cli.CLIFrom(cmd.Context())
			node, err := cc.Client.GetOrganization(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if f.Changed("key") {
				node.OrgKey = key
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
			updated, err := cc.Client.UpdateOrganization(cmd.Context(), args[0], node)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("组织节点更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "组织节点唯一标识")
	cmd.Flags().StringVar(&name, "name", "", "组织节点名称")
	cmd.Flags().StringVar(&level, "level", "", "层级：company|department|position")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "父节点 ID")
	cmd.Flags().StringVar(&description, "description", "", "描述")
	cmd.Flags().Int32Var(&sortOrder, "sort-order", 0, "排序序号")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "是否启用")
	return cmd
}

func orgDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除组织节点",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除组织节点 %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteOrganization(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("组织节点已删除", "id", args[0])
		},
	}
}

func orgReorderCmd() *cobra.Command {
	var idsCSV string
	cmd := &cobra.Command{
		Use:   "reorder --ids <id1,id2,...>",
		Short: "调整组织节点排序",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := splitCSV(idsCSV)
			if len(ids) == 0 {
				return &cli.CLIError{Code: "INVALID_IDS", Message: "--ids 不能为空，格式：id1,id2,..."}
			}
			cc := cli.CLIFrom(cmd.Context())
			if err := cc.Client.ReorderOrganization(cmd.Context(), ids); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("组织节点排序已更新", "count", fmt.Sprintf("%d", len(ids)))
		},
	}
	cmd.Flags().StringVar(&idsCSV, "ids", "", "逗号分隔的节点 ID 列表")
	return cmd
}

// isValidOrgLevel 校验 level 是否为合法的组织层级。
func isValidOrgLevel(level string) bool {
	return isOneOf(level, orgLevels)
}

// isOneOf 判断 value 是否在 allowed 列表中。
func isOneOf(value string, allowed []string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// splitCSV 按逗号拆分并去除空白与空项。
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// appendOrgTreeRows 将组织树先序展开为带缩进的行。
func appendOrgTreeRows(rows []map[string]string, n *organizationv1.OrganizationTreeNode, depth int) []map[string]string {
	if n == nil || n.Node == nil {
		return rows
	}
	row := orgNodeToRow(n.Node)
	row["name"] = strings.Repeat("  ", depth) + n.Node.Name
	rows = append(rows, row)
	for _, ch := range n.Children {
		rows = appendOrgTreeRows(rows, ch, depth+1)
	}
	return rows
}

// orgNodeToRow converts an OrganizationNode to a display row.
func orgNodeToRow(n *organizationv1.OrganizationNode) map[string]string {
	if n == nil {
		return nil
	}
	enabled := "false"
	if n.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":         n.Id,
		"org_key":    n.OrgKey,
		"name":       n.Name,
		"level":      n.Level,
		"parent_id":  n.ParentId,
		"enabled":    enabled,
		"sort_order": fmt.Sprintf("%d", n.SortOrder),
		"status":     n.Status,
	}
}
