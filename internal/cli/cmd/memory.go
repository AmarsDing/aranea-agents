package cmd

import (
	"fmt"

	memoryv1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewMemoryCmd creates the `aranea memory` command group.
func NewMemoryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "memory",
		Short: "记忆管理（L3 事实 / 级联提案 / 检索调试）",
	}
	c.AddCommand(
		memoryFactsCmd(),
		memoryProposalsCmd(),
		memorySearchCmd(),
		memoryRecallDebugCmd(),
	)
	return c
}

func memoryFactsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "facts",
		Short: "L3 记忆事实",
	}
	c.AddCommand(memoryFactsLsCmd())
	return c
}

func memoryFactsLsCmd() *cobra.Command {
	var scopeType, scopeID, kind, status, keyword string
	var limit, offset int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出 L3 记忆事实",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListMemoryFacts(cmd.Context(), scopeType, scopeID, kind, status, keyword, limit, offset)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(memoryFactsToRows(resp.Items), int(resp.Total))
		},
	}
	cmd.Flags().StringVar(&scopeType, "scope-type", "", "作用域类型")
	cmd.Flags().StringVar(&scopeID, "scope-id", "", "作用域 ID（如 agent 过滤用 --scope-type agent --scope-id <id>）")
	cmd.Flags().StringVar(&kind, "kind", "", "事实类型过滤")
	cmd.Flags().StringVar(&status, "status", "", "状态过滤")
	cmd.Flags().StringVar(&keyword, "keyword", "", "关键词过滤")
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	return cmd
}

func memoryProposalsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "proposals",
		Short: "级联变更提案",
	}
	c.AddCommand(
		memoryProposalsLsCmd(),
		memoryProposalsApproveCmd(),
		memoryProposalsRejectCmd(),
	)
	return c
}

func memoryProposalsLsCmd() *cobra.Command {
	var agentID, status string
	var limit int32
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出级联变更提案",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCascadeProposals(cmd.Context(), agentID, status, limit)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(cascadeProposalsToRows(resp.Items), len(resp.Items))
		},
	}
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID")
	cmd.Flags().StringVar(&status, "status", "", "状态过滤")
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	_ = cmd.MarkFlagRequired("agent-id")
	return cmd
}

func memoryProposalsApproveCmd() *cobra.Command {
	var reviewer string
	cmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "批准级联变更提案",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			p, err := cc.Client.ApproveCascadeProposal(cmd.Context(), args[0], reviewer)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("提案已批准", "id", p.Id, "status", p.Status)
		},
	}
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "审批人")
	return cmd
}

func memoryProposalsRejectCmd() *cobra.Command {
	var reviewer, reason string
	cmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "拒绝级联变更提案",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			p, err := cc.Client.RejectCascadeProposal(cmd.Context(), args[0], reviewer, reason)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("提案已拒绝", "id", p.Id, "status", p.Status)
		},
	}
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "审批人")
	cmd.Flags().StringVar(&reason, "reason", "", "拒绝原因")
	return cmd
}

func memorySearchCmd() *cobra.Command {
	var agentID, sessionID, userID, query string
	var limit int32
	cmd := &cobra.Command{
		Use:   "search",
		Short: "跨层复合检索记忆",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &memoryv1.CompositeSearchMemoriesRequest{
				AgentId:   agentID,
				SessionId: sessionID,
				UserId:    userID,
				Query:     query,
				Limit:     limit,
			}
			resp, err := cc.Client.CompositeSearchMemories(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintList(compositeHitsToRows(resp.Items), len(resp.Items))
		},
	}
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session ID")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID")
	cmd.Flags().StringVar(&query, "query", "", "检索内容")
	cmd.Flags().Int32Var(&limit, "limit", 10, "返回数量")
	_ = cmd.MarkFlagRequired("agent-id")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func memoryRecallDebugCmd() *cobra.Command {
	var agentID, sessionID, userID, query string
	var l2Limit, l3Limit int32
	cmd := &cobra.Command{
		Use:   "recall-debug",
		Short: "回忆检索调试（查看 L2/L3 命中与打分）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req := &memoryv1.DebugMemoryRecallRequest{
				AgentId:   agentID,
				SessionId: sessionID,
				UserId:    userID,
				Query:     query,
				L2Limit:   l2Limit,
				L3Limit:   l3Limit,
			}
			resp, err := cc.Client.DebugMemoryRecall(cmd.Context(), req)
			if err != nil {
				return err
			}
			rows := recallHitsToRows(resp.L2Hits)
			rows = append(rows, recallHitsToRows(resp.L3Hits)...)
			return cc.Printer.PrintList(rows, len(rows))
		},
	}
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Agent ID")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session ID")
	cmd.Flags().StringVar(&userID, "user-id", "", "User ID")
	cmd.Flags().StringVar(&query, "query", "", "检索内容")
	cmd.Flags().Int32Var(&l2Limit, "l2-limit", 0, "L2 命中数量上限（0 = 服务端默认）")
	cmd.Flags().Int32Var(&l3Limit, "l3-limit", 0, "L3 命中数量上限（0 = 服务端默认）")
	_ = cmd.MarkFlagRequired("agent-id")
	return cmd
}

// Row helpers convert proto items to display rows.

func memoryFactsToRows(items []*memoryv1.MemoryFact) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, f := range items {
		rows = append(rows, map[string]string{
			"id":         f.Id,
			"scope_type": f.ScopeType,
			"scope_id":   f.ScopeId,
			"kind":       f.FactKind,
			"status":     f.Status,
			"statement":  f.Statement,
		})
	}
	return rows
}

func cascadeProposalsToRows(items []*memoryv1.CascadeProposal) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, p := range items {
		rows = append(rows, map[string]string{
			"id":         p.Id,
			"agent_id":   p.AgentId,
			"status":     p.Status,
			"risk_level": p.RiskLevel,
			"trigger":    fmt.Sprintf("%s.%s", p.TriggerEntityName, p.TriggerAttribute),
			"change":     fmt.Sprintf("%s -> %s", p.OldValue, p.NewValue),
			"created_at": p.CreatedAt,
		})
	}
	return rows
}

func compositeHitsToRows(items []*memoryv1.CompositeSearchHit) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, h := range items {
		rows = append(rows, map[string]string{
			"id":    h.Id,
			"layer": h.Layer,
			"score": fmt.Sprintf("%.3f", h.Score),
			"text":  h.Text,
		})
	}
	return rows
}

func recallHitsToRows(items []*memoryv1.MemoryRecallHit) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, h := range items {
		score := ""
		if h.Scores != nil {
			score = fmt.Sprintf("%.3f", h.Scores.Total)
		}
		rows = append(rows, map[string]string{
			"id":        h.Id,
			"layer":     h.Layer,
			"title":     h.Title,
			"score":     score,
			"statement": h.Statement,
		})
	}
	return rows
}
