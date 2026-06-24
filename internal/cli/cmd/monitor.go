package cmd

import (
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewMonitorCmd creates the `aranea monitor` command group.
func NewMonitorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "monitor",
		Short: "监控与审计",
	}
	c.AddCommand(
		monitorAuditLogsCmd(),
		monitorEventsCmd(),
		monitorTracesCmd(),
	)
	return c
}

func monitorAuditLogsCmd() *cobra.Command {
	var limit, offset int32
	var action, resource, actor, keyword string
	cmd := &cobra.Command{
		Use:     "audit-logs",
		Aliases: []string{"audit-log"},
		Short:   "查看审计日志",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListAuditLogs(cmd.Context(), limit, offset, action, resource, actor, keyword)
			if err != nil {
				return err
			}
			rows := auditLogsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "最多返回条数")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&action, "action", "", "过滤操作类型")
	cmd.Flags().StringVar(&resource, "resource", "", "过滤资源类型")
	cmd.Flags().StringVar(&actor, "actor", "", "过滤操作者")
	cmd.Flags().StringVar(&keyword, "keyword", "", "关键词搜索")
	return cmd
}

func monitorEventsCmd() *cobra.Command {
	var limit, offset int32
	var eventType, agentID, status string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "查看监控事件",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListMonitorEvents(cmd.Context(), limit, offset, eventType, agentID, status)
			if err != nil {
				return err
			}
			rows := monitorRowsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "最多返回条数")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&eventType, "event-type", "", "过滤事件类型")
	cmd.Flags().StringVar(&agentID, "agent", "", "过滤 Agent ID")
	cmd.Flags().StringVar(&status, "status", "", "过滤状态")
	return cmd
}

func monitorTracesCmd() *cobra.Command {
	var limit, offset int32
	var agentID, provider, model, status string
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "查看调用链追踪",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListMonitorTraces(cmd.Context(), limit, offset, agentID, provider, model, status)
			if err != nil {
				return err
			}
			rows := monitorRowsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "最多返回条数")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&agentID, "agent", "", "过滤 Agent ID")
	cmd.Flags().StringVar(&provider, "provider", "", "过滤 Provider")
	cmd.Flags().StringVar(&model, "model", "", "过滤模型")
	cmd.Flags().StringVar(&status, "status", "", "过滤状态")
	return cmd
}

// Row helpers convert proto items to display rows.

func auditLogsToRows(items []*monitorv1.AuditLog) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, a := range items {
		rows = append(rows, map[string]string{
			"id":         a.Id,
			"action":     a.Action,
			"resource":   a.Resource,
			"resource_id": a.ResourceId,
			"actor":      a.Actor,
			"severity":   a.Severity,
			"created_at": a.CreatedAt,
		})
	}
	return rows
}

func monitorRowsToRows(items []*monitorv1.MonitorPlatformRow) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, m := range items {
		enabled := "false"
		if m.Enabled {
			enabled = "true"
		}
		rows = append(rows, map[string]string{
			"id":         m.Id,
			"resource":   m.Resource,
			"key":        m.Key,
			"name":       m.Name,
			"status":     m.Status,
			"enabled":    enabled,
			"agent_id":   m.AgentId,
			"provider":   m.Provider,
			"model":      m.Model,
			"created_at": m.CreatedAt,
		})
	}
	return rows
}
