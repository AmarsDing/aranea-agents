package cmd

import (
	"fmt"
	"os"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewSessionCmd creates the `aranea session` command group.
func NewSessionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "session",
		Short: "会话管理",
	}
	c.AddCommand(
		sessionLsCmd(),
		sessionGetCmd(),
		sessionSendCmd(),
		sessionMessagesCmd(),
		sessionArchiveCmd(),
		sessionRestoreCmd(),
		sessionPinCmd(),
		sessionUnpinCmd(),
		sessionCompactCmd(),
		sessionExportCmd(),
	)
	return c
}

func sessionLsCmd() *cobra.Command {
	var agentID string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出会话",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.SearchSessions(cmd.Context(), agentID, limit, offset)
			if err != nil {
				return err
			}
			rows := sessionsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "过滤指定 Agent ID")
	cmd.Flags().Int32Var(&limit, "limit", 20, "最多返回条数")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func sessionGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看会话详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			sess, err := cc.Client.GetSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(sessionToRow(sess))
		},
	}
}

func sessionSendCmd() *cobra.Command {
	var sessionID, agentID, content string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "向会话发送消息（需要 --yes）",
		Long: `向指定会话发送消息。这是一个外部副作用操作，必须传入 --yes 或 -y 确认。

注意：send 使用 POST /v1/chat/messages 接口，不走 WebSocket。
      如需流式对话体验，使用 aranea chat 命令进入 REPL。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				return &cli.CLIError{
					Code:    "CONFIRMATION_REQUIRED",
					Message: "session send 是外部副作用操作，必须传入 --yes 确认",
				}
			}
			if content == "" {
				return &cli.CLIError{Code: "MISSING_CONTENT", Message: "--content 不能为空"}
			}
			req := &chatv1.SendChatMessageRequest{
				SessionId: sessionID,
				Content:   content,
			}
			if agentID != "" {
				req.AgentKey = &agentID
			}
			_, err := cc.Client.SendChatMessage(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("消息已发送", "session_id", sessionID)
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "会话 ID")
	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID（新建会话时必填）")
	cmd.Flags().StringVar(&content, "content", "", "消息内容")
	_ = cmd.MarkFlagRequired("session")
	_ = cmd.MarkFlagRequired("content")
	return cmd
}

func sessionMessagesCmd() *cobra.Command {
	var limit int32
	cmd := &cobra.Command{
		Use:   "messages <session-id>",
		Short: "列出会话消息",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListSessionMessages(cmd.Context(), args[0], limit)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, m := range resp.Items {
				rows = append(rows, map[string]string{
					"id":         m.Id,
					"role":       m.Role,
					"content":    truncate(m.ContentMarkdown, 80),
					"created_at": m.CreatedAt,
				})
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "最多返回条数")
	return cmd
}

func sessionArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "归档会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if err := cc.Client.ArchiveSession(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("会话已归档", "id", args[0])
		},
	}
}

func sessionRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>",
		Short: "恢复已归档的会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			sess, err := cc.Client.RestoreSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("会话已恢复", "id", sess.Id, "status", sess.Status)
		},
	}
}

func sessionPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin <id>",
		Short: "置顶会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			sess, err := cc.Client.PinSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("会话已置顶", "id", sess.Id)
		},
	}
}

func sessionUnpinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <id>",
		Short: "取消置顶会话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			sess, err := cc.Client.UnpinSession(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("会话已取消置顶", "id", sess.Id)
		},
	}
}

func sessionCompactCmd() *cobra.Command {
	var preserve string
	cmd := &cobra.Command{
		Use:   "compact <id>",
		Short: "压缩会话上下文（生成摘要，释放上下文窗口）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.CompactSession(cmd.Context(), args[0], preserve)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("会话压缩完成",
				"id", args[0],
				"compacted", fmt.Sprintf("%t", resp.Compacted),
				"from_turn", fmt.Sprintf("%d", resp.FromTurn),
				"to_turn", fmt.Sprintf("%d", resp.ToTurn),
				"tokens_before", fmt.Sprintf("%d", resp.EstimatedTokensBefore),
				"tokens_after", fmt.Sprintf("%d", resp.EstimatedTokensAfter))
		},
	}
	cmd.Flags().StringVar(&preserve, "preserve-instruction", "", "压缩时保留的指示（如必须保留的关键信息）")
	return cmd
}

func sessionExportCmd() *cobra.Command {
	var format, out string
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "导出会话内容（markdown/json）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "markdown" && format != "json" {
				return &cli.CLIError{Code: "INVALID_FORMAT", Message: "--format 仅支持 markdown 或 json"}
			}
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ExportSession(cmd.Context(), args[0], format)
			if err != nil {
				return err
			}
			if out == "" {
				_, err := fmt.Fprint(cmd.OutOrStdout(), resp.Content)
				return err
			}
			if err := os.WriteFile(out, []byte(resp.Content), 0o644); err != nil {
				return &cli.CLIError{Code: "FILE_WRITE_ERROR", Message: err.Error()}
			}
			return cc.Printer.PrintSuccess("会话已导出", "file", out, "format", format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "导出格式：markdown | json")
	cmd.Flags().StringVarP(&out, "out", "o", "", "输出文件路径（缺省输出到 stdout）")
	return cmd
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len([]rune(s)) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen]) + "..."
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func sessionsToRows(items []*sessionv1.Session) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, s := range items {
		rows = append(rows, sessionToRow(s))
	}
	return rows
}

func sessionToRow(s *sessionv1.Session) map[string]string {
	return map[string]string{
		"id":          s.Id,
		"title":       s.Title,
		"agent_id":    s.AgentId,
		"status":      s.Status,
		"messages":    fmt.Sprintf("%d", s.MessageCount),
		"last_msg_at": s.LastMessageAt,
	}
}
