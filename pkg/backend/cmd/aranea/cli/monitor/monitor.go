// Package monitor 实现 `aranea monitor audit/tool-runs`。
package monitor

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
	"arenea/backend/internal/domain"
)

// NewCommand 返回父级命令。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Browse audit logs and recent tool invocations",
	}
	cmd.AddCommand(newAuditCmd(g), newToolRunsCmd(g))
	return cmd
}

func newAuditCmd(g *apiclient.GlobalContext) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "List recent audit log entries",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			var resp struct {
				Items []domain.AuditLog `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/monitor/audit", q, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	return cmd
}

func newToolRunsCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		toolKey string
		agentID string
		status  string
		limit   int
	)
	cmd := &cobra.Command{
		Use:   "tool-runs",
		Short: "List recent tool invocations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if toolKey != "" {
				q.Set("tool_key", toolKey)
			}
			if agentID != "" {
				q.Set("agent_id", agentID)
			}
			if status != "" {
				q.Set("status", status)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			var resp domain.ToolRunResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools/runs", q, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&toolKey, "tool", "", "Filter by tool key")
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	return cmd
}
