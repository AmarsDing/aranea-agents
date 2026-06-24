package cmd

import (
	"fmt"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewMCPCmd creates the `aranea mcp` command group.
func NewMCPCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "MCP 服务器管理",
	}
	c.AddCommand(
		mcpLsCmd(),
		mcpGetCmd(),
		mcpAddCmd(),
		mcpUpdateCmd(),
		mcpDeleteCmd(),
		mcpTestCmd(),
	)
	return c
}

func mcpLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出所有 MCP 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListMCPServers(cmd.Context())
			if err != nil {
				return err
			}
			rows := mcpServersToRows(resp.Items)
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func mcpGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看 MCP 服务器详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			mcp, err := cc.Client.GetMCPServer(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(mcpServerToRow(mcp))
		},
	}
}

func mcpAddCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "add --file <file>",
		Short: "添加 MCP 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &mcpv1.CreateMCPServerRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
			}
			mcp, err := cc.Client.CreateMCPServer(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP 服务器创建成功", "id", mcp.Id, "name", mcp.Name)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "MCP 服务器配置文件路径（YAML/JSON）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func mcpUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "更新 MCP 服务器",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			var mcp mcpv1.MCPServer
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, &mcp); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
			}
			updated, err := cc.Client.UpdateMCPServer(cmd.Context(), args[0], &mcp)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP 服务器更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "MCP 服务器配置文件路径（YAML/JSON）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func mcpDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除 MCP 服务器",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除 MCP 服务器 %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteMCPServer(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP 服务器已删除", "id", args[0])
		},
	}
	return cmd
}

func mcpTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "测试 MCP 服务器连通性",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			result, err := cc.Client.TestMCPServer(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if result.Ok {
				return cc.Printer.PrintSuccess("MCP 服务器测试成功", "status", result.Status)
			}
			return cc.Printer.PrintSuccess("MCP 服务器测试失败", "status", result.Status, "message", result.Message)
		},
	}
}

// Row helpers convert proto items to display rows.

func mcpServersToRows(items []*mcpv1.MCPServer) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, m := range items {
		rows = append(rows, mcpServerToRow(m))
	}
	return rows
}

func mcpServerToRow(m *mcpv1.MCPServer) map[string]string {
	enabled := "false"
	if m.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":      m.Id,
		"key":     m.Key,
		"name":    m.Name,
		"status":  m.Status,
		"enabled": enabled,
	}
}
