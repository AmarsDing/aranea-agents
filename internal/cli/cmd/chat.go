package cmd

import (
	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/repl"

	"github.com/spf13/cobra"
)

// NewChatCmd returns the `aranea chat` command.
func NewChatCmd() *cobra.Command {
	var sessionID string
	var agentKey string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "进入交互式对话 REPL",
		Long: `与 Aranea AI 进行交互式对话。

默认连接系统管家 Agent（__system_admin__），支持流式输出、工具调用显示、以及 /help、/cancel、/quit 等交互命令。`,
		Example: `  aranea chat
  aranea chat --session <session-id>
  aranea chat --agent __system_admin__`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())

			base := cc.Client.Base
			token := cc.Client.Token
			if base == "" {
				return &cli.CLIError{Code: "CONFIG_INVALID", Message: "未配置服务器地址，请先运行 aranea login"}
			}

			cfg := repl.Config{
				APIBase:   base,
				Token:     token,
				SessionID: sessionID,
				AgentKey:  agentKey,
			}
			r := repl.New(cfg)
			return r.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "要附加的会话 ID（留空则创建新会话）")
	cmd.Flags().StringVar(&agentKey, "agent", "__system_admin__", "指定 Agent key（默认系统管家）")

	return cmd
}
