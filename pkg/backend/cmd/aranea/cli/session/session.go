// Package session 实现 `aranea session ls/get/send/archive/delete` ——
// 可脚本化的一次性智能体调用入口。
package session

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/agent"
	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

// NewCommand 返回父级命令。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage chat sessions and send one-shot prompts",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g), newSendCmd(g), newArchiveCmd(g), newDeleteCmd(g))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		agentID, teamID, status, keyword string
		limit, offset                    int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for k, v := range map[string]string{
				"agent_id": agentID, "team_id": teamID,
				"status": status, "keyword": keyword,
			} {
				if v != "" {
					q.Set(k, v)
				}
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			var resp domain.SessionListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/sessions", q, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent id")
	cmd.Flags().StringVar(&teamID, "team", "", "Filter by team id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&keyword, "keyword", "", "Free-text search")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var s domain.Session
			if err := g.Client().Get(cmd.Context(), "/api/v1/sessions/"+url.PathEscape(args[0]), nil, &s); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), s)
			return nil
		},
	}
}

func newSendCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		agentKey  string
		teamID    string
		sessionID string
		mode      string
	)
	cmd := &cobra.Command{
		Use:   "send <message>",
		Short: "Send a one-shot prompt and print the agent response",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentKey == "" && teamID == "" {
				agentKey = "__system_admin__"
			}
			if agentKey != "" {
				if a, err := agent.Resolve(cmd.Context(), g, agentKey); err == nil {
					agentKey = a.AgentKey
				}
			}
			in := service.SendMessageInput{
				SessionID: sessionID,
				AgentKey:  agentKey,
				TeamID:    teamID,
				Content:   args[0],
				Options:   service.SendMessageOptions{DialogMode: mode},
			}
			var out service.SendMessageResult
			if err := g.Client().Post(cmd.Context(), "/api/v1/chat/messages", in, &out); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentKey, "agent", "", "Target agent key (defaults to __system_admin__)")
	cmd.Flags().StringVar(&teamID, "team", "", "Target team id")
	cmd.Flags().StringVar(&sessionID, "session", "", "Existing session id (otherwise a new one is created)")
	cmd.Flags().StringVar(&mode, "mode", "", "Dialog mode (default|plan|code|...)")
	return cmd
}

func newArchiveCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.Client().Post(cmd.Context(), "/api/v1/sessions/"+url.PathEscape(args[0])+"/archive", nil, nil); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), "archived "+args[0])
			return nil
		},
	}
}

func newDeleteCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.Client().Delete(cmd.Context(), "/api/v1/sessions/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), "deleted "+args[0])
			return nil
		},
	}
}

// EnsureSession 在 sessionID 非空时返回已有会话，否则为给定智能体新建一条。
// 导出供 console launcher 复用同一路径。
func EnsureSession(ctx context.Context, g *apiclient.GlobalContext, sessionID, agentID, title string) (domain.Session, error) {
	if sessionID != "" {
		var s domain.Session
		if err := g.Client().Get(ctx, "/api/v1/sessions/"+url.PathEscape(sessionID), nil, &s); err != nil {
			return domain.Session{}, err
		}
		return s, nil
	}
	if agentID == "" {
		return domain.Session{}, errors.New("either session id or agent id is required")
	}
	if title == "" {
		title = "CLI session"
	}
	var s domain.Session
	body := domain.Session{
		OwnerType: "agent",
		AgentID:   agentID,
		Title:     title,
		Status:    "active",
	}
	if err := g.Client().Post(ctx, "/api/v1/sessions", body, &s); err != nil {
		return domain.Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}
