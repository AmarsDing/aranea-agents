package cmd

import (
	"fmt"
	"os"
	"strings"

	agentv1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewAgentCmd creates the `aranea agent` command group.
func NewAgentCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent",
		Short: "Agent 管理",
	}
	c.AddCommand(
		agentLsCmd(),
		agentGetCmd(),
		agentCreateCmd(),
		agentUpdateCmd(),
		agentDeleteCmd(),
		agentEnableCmd(),
		agentDisableCmd(),
		agentToolsCmd(),
		agentToolsSetCmd(),
	)
	return c
}

func agentLsCmd() *cobra.Command {
	var limit, offset int32
	var search string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出所有 Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListAgents(cmd.Context(), search, limit, offset)
			if err != nil {
				return err
			}
			rows := agentsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "page-size", 20, "每页数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&search, "search", "", "搜索关键词")
	return cmd
}

func agentGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看 Agent 详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			agent, err := cc.Client.GetAgent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(agentToRow(agent))
		},
	}
}

func agentCreateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "create --file <file>",
		Short: "创建 Agent（从 YAML/JSON 文件）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req, err := loadCreateAgentRequestFromFile(filePath)
			if err != nil {
				return err
			}
			agent, err := cc.Client.CreateAgent(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 创建成功", "id", agent.Id, "name", agent.DisplayName)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "YAML/JSON 文件路径（- 表示 stdin）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func agentUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "更新 Agent（从 YAML/JSON 文件）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			agent, err := loadAgentFromFile(filePath)
			if err != nil {
				return err
			}
			updated, err := cc.Client.UpdateAgent(cmd.Context(), args[0], agent)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "YAML/JSON 文件路径（- 表示 stdin）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func agentDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除 Agent（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认删除 Agent %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteAgent(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 已删除", "id", args[0])
		},
	}
}

func agentEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用 Agent（status=active）",
		Long: `启用 Agent。
代码考古 A5: agent.proto 中无独立 enable/disable RPC，改为调用 PATCH /v1/agents/{id}，
传入 status=active 字段（proto UpdateAgent 的 body 为 agent）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			agent, err := cc.Client.EnableAgent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 已启用", "id", agent.Id, "status", agent.Status)
		},
	}
}

func agentDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "停用 Agent（status=inactive）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认停用 Agent %q？", args[0]), true)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			agent, err := cc.Client.DisableAgent(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 已停用", "id", agent.Id, "status", agent.Status)
		},
	}
}

func agentToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <agent-id>",
		Short: "查看 Agent 有效工具集",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			view, err := cc.Client.GetAgentEffectiveTools(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, t := range view.Items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%v\n", t.ToolKey, t.DisplayName, t.Enabled)
			}
			return nil
		},
	}
}

func agentToolsSetCmd() *cobra.Command {
	var allow, deny string
	cmd := &cobra.Command{
		Use:   "tools-set <agent-id>",
		Short: "设置 Agent 工具策略（allow/deny）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			// Convert comma-separated to string slices.
			allowList := csvToStringSlice(allow)
			denyList := csvToStringSlice(deny)
			req := &agentv1.UpdateAgentToolPolicyRequest{
				AgentId: args[0],
				Allow:   allowList,
				Deny:    denyList,
			}
			_, err := cc.Client.UpdateAgentToolPolicy(cmd.Context(), args[0], req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Agent 工具策略已更新", "agent_id", args[0])
		},
	}
	cmd.Flags().StringVar(&allow, "allow", "", "允许的工具（逗号分隔）")
	cmd.Flags().StringVar(&deny, "deny", "", "禁止的工具（逗号分隔）")
	return cmd
}

// csvToStringSlice splits a comma-separated string into a slice.
func csvToStringSlice(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// loadCreateAgentRequestFromFile loads a CreateAgentRequest from file.
func loadCreateAgentRequestFromFile(path string) (*agentv1.CreateAgentRequest, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
	}
	req := &agentv1.CreateAgentRequest{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(data, req); err != nil {
		return nil, &cli.CLIError{Code: "FILE_PARSE_ERROR",
			Message: fmt.Sprintf("文件解析失败: %v", err)}
	}
	return req, nil
}

// loadAgentFromFile loads an Agent from file.
func loadAgentFromFile(path string) (*agentv1.Agent, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
	}
	agent := &agentv1.Agent{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(data, agent); err != nil {
		return nil, &cli.CLIError{Code: "FILE_PARSE_ERROR",
			Message: fmt.Sprintf("文件解析失败: %v", err)}
	}
	return agent, nil
}

// readFile reads file content; "-" reads stdin.
func readFile(path string) ([]byte, error) {
	if path == "-" {
		return readStdin()
	}
	return os.ReadFile(path)
}

func readStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

// agentToRow converts an Agent to a display row.
func agentToRow(a *agentv1.Agent) map[string]string {
	if a == nil {
		return nil
	}
	return map[string]string{
		"id":           a.Id,
		"agent_key":    a.AgentKey,
		"display_name": a.DisplayName,
		"provider":     a.Provider,
		"model":        a.Model,
		"status":       a.Status,
		"created_at":   a.CreatedAt,
	}
}

// agentsToRows converts a slice of Agent to display rows.
func agentsToRows(agents []*agentv1.Agent) []map[string]string {
	rows := make([]map[string]string, 0, len(agents))
	for _, a := range agents {
		rows = append(rows, agentToRow(a))
	}
	return rows
}
