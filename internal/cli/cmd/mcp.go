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
		Short: "MCP ?????",
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
		Short: "???? MCP ???",
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
		Short: "?? MCP ?????",
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
		Short: "?? MCP ???",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &mcpv1.CreateMCPServerRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			mcp, err := cc.Client.CreateMCPServer(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP ??????", "id", mcp.Id, "name", mcp.Name)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "MCP ????????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func mcpUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "?? MCP ???",
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
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			updated, err := cc.Client.UpdateMCPServer(cmd.Context(), args[0], &mcp)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP ??????", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "MCP ????????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func mcpDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "?? MCP ???",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("???? MCP ??? %q?", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "?????"}
				}
			}
			if err := cc.Client.DeleteMCPServer(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("MCP ??????", "id", args[0])
		},
	}
	return cmd
}

func mcpTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "?? MCP ??????",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			result, err := cc.Client.TestMCPServer(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if result.Ok {
				return cc.Printer.PrintSuccess("MCP ????????", "status", result.Status)
			}
			return cc.Printer.PrintSuccess("MCP ????????", "status", result.Status, "message", result.Message)
		},
	}
}

// ??? helpers ??????????????????????????????????????????????????????????????????

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
