// Package tool implements the `aranea tool` management commands.
package tool

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
		Use:   "tool",
		Short: "Manage tools",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g), newCreateCmd(g), newUpdateCmd(g), newDeleteCmd(g), newRunsCmd(g), newToggleCmd(g, true), newToggleCmd(g, false))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		search    string
		category  string
		source    string
		riskLevel string
		enabled   string
		limit     int
		offset    int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List tools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for k, v := range map[string]string{
				"search": search, "category": category, "source": source,
				"risk_level": riskLevel, "enabled": enabled,
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
			var result domain.ToolListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Free-text search")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source (builtin|skill|mcp|...)")
	cmd.Flags().StringVar(&riskLevel, "risk", "", "Filter by risk level")
	cmd.Flags().StringVar(&enabled, "enabled", "", "Filter by enabled flag (true|false)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newCreateCmd(g *apiclient.GlobalContext) *cobra.Command {
	var input domain.ToolUpsertInput
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tool catalog entry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var created domain.Tool
			if err := g.Client().Post(cmd.Context(), "/api/v1/tools", input, &created); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), created)
			return nil
		},
	}
	bindToolInputFlags(cmd, &input)
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

func newUpdateCmd(g *apiclient.GlobalContext) *cobra.Command {
	var input domain.ToolUpsertInput
	cmd := &cobra.Command{
		Use:   "update <id-or-key>",
		Short: "Update a tool catalog entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var updated domain.Tool
			if err := g.Client().Put(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0]), input, &updated); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), updated)
			return nil
		},
	}
	bindToolInputFlags(cmd, &input)
	return cmd
}

func newDeleteCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-key>",
		Short: "Soft-delete a tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.Client().Delete(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), args[0])
			return nil
		},
	}
}

func newRunsCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		toolKey string
		agentID string
		status  string
		from    string
		limit   int
		offset  int
	)
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List tool invocation records",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for k, v := range map[string]string{"tool_key": toolKey, "agent_id": agentID, "status": status, "from": from} {
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
			var result domain.ToolRunResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools/runs", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&toolKey, "tool-key", "", "Filter by tool key")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Filter by agent id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&from, "from", "", "Filter by start time")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show a single tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var t domain.Tool
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0]), nil, &t); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), t)
			return nil
		},
	}
}

func newToggleCmd(g *apiclient.GlobalContext, enabled bool) *cobra.Command {
	use := "enable"
	short := "Enable a tool"
	if !enabled {
		use = "disable"
		short = "Disable a tool"
	}
	return &cobra.Command{
		Use:   use + " <id-or-key>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]bool{"enabled": enabled}
			var updated domain.Tool
			if err := g.Client().Patch(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0])+"/enabled", body, &updated); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), updated.Key)
			return nil
		},
	}
}

func bindToolInputFlags(cmd *cobra.Command, input *domain.ToolUpsertInput) {
	cmd.Flags().StringVar(&input.Key, "key", "", "Tool key")
	cmd.Flags().StringVar(&input.DisplayName, "name", "", "Display name")
	cmd.Flags().StringVar(&input.Description, "description", "", "Description")
	cmd.Flags().StringVar(&input.Category, "category", "custom", "Category")
	cmd.Flags().StringVar(&input.Source, "source", "external", "Source")
	cmd.Flags().StringVar(&input.RiskLevel, "risk", "low", "Risk level")
	cmd.Flags().BoolVar(&input.Enabled, "enabled", true, "Enabled")
	cmd.Flags().BoolVar(&input.Readonly, "readonly", false, "Readonly")
	cmd.Flags().BoolVar(&input.RequiresConfirmation, "requires-confirmation", false, "Requires human confirmation")
	cmd.Flags().BoolVar(&input.SupportsStreaming, "supports-streaming", false, "Supports streaming")
	cmd.Flags().BoolVar(&input.SupportsConcurrency, "supports-concurrency", false, "Supports concurrency")
	cmd.Flags().StringVar(&input.ParametersSchemaJSON, "parameters-schema", "{}", "Parameters JSON schema")
	cmd.Flags().StringVar(&input.ResultSchemaJSON, "result-schema", "{}", "Result JSON schema")
	cmd.Flags().StringVar(&input.ConfigSchemaJSON, "config-schema", "{}", "Config JSON schema")
	cmd.Flags().StringVar(&input.ConfigJSON, "config", "{}", "Config JSON")
	cmd.Flags().StringVar(&input.DefaultConfigJSON, "default-config", "{}", "Default config JSON")
	cmd.Flags().StringVar(&input.MetadataJSON, "metadata", "{}", "Metadata JSON")
}
