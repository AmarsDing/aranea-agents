package cmd

import (
	"fmt"
	"strconv"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewA2ACmd creates the `aranea a2a` command group.
func NewA2ACmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "a2a",
		Short: "A2A 联邦管理（远程 Agent 发现、注册与审计）",
	}
	c.AddCommand(
		a2aDiscoverCmd(),
		a2aRemoteAgentsCmd(),
		a2aAuditCmd(),
		a2aConfigCmd(),
	)
	return c
}

// a2aDiscoverCmd 实现 `a2a discover --url x`。
// 注：proto 中 POST /v1/a2a/discover 不存在；带 URL 的远程发现走
// POST /v1/a2a/remote-discover（DiscoverRemoteAgent，仅抓取不持久化）。
func a2aDiscoverCmd() *cobra.Command {
	var remoteURL, authType, authConfigJSON string
	cmd := &cobra.Command{
		Use:   "discover --url <remote-url>",
		Short: "从远程 URL 发现 AgentCard（仅查询，不注册）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			card, err := cc.Client.DiscoverA2ARemoteAgent(cmd.Context(), remoteURL, authType, authConfigJSON)
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(a2aAgentCardToRow(card))
		},
	}
	cmd.Flags().StringVar(&remoteURL, "url", "", "远程 A2A 服务 URL")
	cmd.Flags().StringVar(&authType, "auth-type", "", "认证类型（如 api_key、token、mtls）")
	cmd.Flags().StringVar(&authConfigJSON, "auth-config-json", "", "认证配置（JSON 字符串）")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func a2aRemoteAgentsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "remote-agents",
		Short: "管理远程 A2A Agent 注册表",
	}
	c.AddCommand(
		a2aRemoteAgentsLsCmd(),
		a2aRemoteAgentsGetCmd(),
		a2aRemoteAgentsAddCmd(),
		a2aRemoteAgentsDeleteCmd(),
	)
	return c
}

func a2aRemoteAgentsLsCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出已注册的远程 Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListA2ARemoteAgents(cmd.Context(), workspace)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, a := range resp.Items {
				rows = append(rows, a2aRemoteAgentToRow(a))
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "按 workspace 过滤")
	return cmd
}

func a2aRemoteAgentsGetCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "查看远程 Agent 详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			// proto 未提供 GET /v1/a2a/remote-agents/{id}，通过 ls + 本地过滤实现。
			resp, err := cc.Client.ListA2ARemoteAgents(cmd.Context(), workspace)
			if err != nil {
				return err
			}
			for _, a := range resp.Items {
				if a.Id == args[0] {
					return cc.Printer.PrintDetail(a2aRemoteAgentToRow(a))
				}
			}
			return &cli.CLIError{Code: "REMOTE_AGENT_NOT_FOUND", Message: fmt.Sprintf("远程 Agent %q 不存在", args[0])}
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "按 workspace 过滤")
	return cmd
}

func a2aRemoteAgentsAddCmd() *cobra.Command {
	var workspace, remoteURL, cardURL, name, authType, authConfigJSON string
	var disabled bool
	cmd := &cobra.Command{
		Use:   "add --url <remote-url>",
		Short: "注册远程 A2A Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &a2av1.RegisterRemoteAgentRequest{
				Workspace:      workspace,
				RemoteUrl:      remoteURL,
				AgentCardUrl:   cardURL,
				DisplayName:    name,
				AuthType:       authType,
				AuthConfigJson: authConfigJSON,
			}
			if disabled {
				enabled := false
				req.Enabled = &enabled
			}
			agent, err := cc.Client.RegisterA2ARemoteAgent(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("远程 Agent 已注册", "id", agent.Id, "display_name", agent.DisplayName)
		},
	}
	cmd.Flags().StringVar(&remoteURL, "url", "", "远程 A2A 服务 URL")
	cmd.Flags().StringVar(&cardURL, "card-url", "", "AgentCard URL（可选，默认由服务端从 remote-url 推导）")
	cmd.Flags().StringVar(&name, "name", "", "显示名称")
	cmd.Flags().StringVar(&workspace, "workspace", "", "所属 workspace")
	cmd.Flags().StringVar(&authType, "auth-type", "", "认证类型（如 api_key、token、mtls）")
	cmd.Flags().StringVar(&authConfigJSON, "auth-config-json", "", "认证配置（JSON 字符串，读取接口不会回显）")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "注册为禁用状态（默认启用）")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func a2aRemoteAgentsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除远程 Agent 注册",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除远程 Agent %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteA2ARemoteAgent(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("远程 Agent 已删除", "id", args[0])
		},
	}
}

func a2aAuditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "audit",
		Short: "A2A 调用审计日志",
	}
	c.AddCommand(a2aAuditLsCmd())
	return c
}

func a2aAuditLsCmd() *cobra.Command {
	var caller, callee string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出 A2A 调用审计记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListA2AAudit(cmd.Context(), caller, callee, limit, offset)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, e := range resp.Items {
				rows = append(rows, map[string]string{
					"id":              e.Id,
					"invoke_id":       e.InvokeId,
					"caller_agent_id": e.CallerAgentId,
					"callee_agent_id": e.CalleeAgentId,
					"capability":      e.Capability,
					"status":          e.Status,
					"duration_ms":     fmt.Sprintf("%d", e.DurationMs),
					"workspace":       e.Workspace,
					"created_at":      e.CreatedAt,
				})
			}
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&caller, "caller", "", "按调用方 Agent ID 过滤")
	cmd.Flags().StringVar(&callee, "callee", "", "按被调方 Agent ID 过滤")
	cmd.Flags().Int32Var(&limit, "limit", 0, "返回数量上限（0 表示服务端默认）")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

// a2aConfigCmd 只提供 get 子命令。
// 注：proto 中 GetA2AConfig 明确为 read-only 运行时配置，无 PUT /v1/a2a/config
// 端点，故不实现 config-set。
func a2aConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "A2A 运行时配置（只读）",
	}
	c.AddCommand(a2aConfigGetCmd())
	return c
}

func a2aConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "查看 A2A 运行时配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			cfg, err := cc.Client.GetA2AConfig(cmd.Context())
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(map[string]string{
				"public_base_url":        cfg.PublicBaseUrl,
				"public_base_url_source": cfg.PublicBaseUrlSource,
			})
		},
	}
}

// a2aAgentCardToRow converts an A2AAgentCard to a display row.
func a2aAgentCardToRow(card *a2av1.A2AAgentCard) map[string]string {
	if card == nil {
		return nil
	}
	return map[string]string{
		"agent_id":     card.AgentId,
		"display_name": card.DisplayName,
		"workspace":    card.Workspace,
		"enabled":      strconv.FormatBool(card.Enabled),
		"capabilities": fmt.Sprintf("%d", len(card.Capabilities)),
		"source":       card.Source,
		"endpoint_url": card.EndpointUrl,
		"remote_url":   card.RemoteUrl,
		"updated_at":   card.UpdatedAt,
	}
}

// a2aRemoteAgentToRow converts an A2ARemoteAgent to a display row.
func a2aRemoteAgentToRow(a *a2av1.A2ARemoteAgent) map[string]string {
	if a == nil {
		return nil
	}
	return map[string]string{
		"id":             a.Id,
		"display_name":   a.DisplayName,
		"workspace":      a.Workspace,
		"remote_url":     a.RemoteUrl,
		"agent_card_url": a.AgentCardUrl,
		"auth_type":      a.AuthType,
		"enabled":        strconv.FormatBool(a.Enabled),
		"healthy":        strconv.FormatBool(a.Healthy),
		"last_health_at": a.LastHealthAt,
		"created_at":     a.CreatedAt,
	}
}
