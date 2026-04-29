// Package plugin 实现 `aranea plugin ls/get/enable/disable`。
package plugin

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
		Use:   "plugin",
		Short: "Inspect and toggle runtime plugins",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g), newToggleCmd(g, true), newToggleCmd(g, false))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		search        string
		category      string
		enabled       string
		callbackPoint string
		limit         int
		offset        int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List plugins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for k, v := range map[string]string{
				"search": search, "category": category,
				"enabled": enabled, "callback_point": callbackPoint,
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
			var result domain.PluginListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/plugins", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Free-text search")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&enabled, "enabled", "", "Filter by enabled flag (true|false)")
	cmd.Flags().StringVar(&callbackPoint, "callback", "", "Filter by callback point")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show a single plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var p domain.Plugin
			if err := g.Client().Get(cmd.Context(), "/api/v1/plugins/"+url.PathEscape(args[0]), nil, &p); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

func newToggleCmd(g *apiclient.GlobalContext, enabled bool) *cobra.Command {
	use := "enable"
	short := "Enable a plugin"
	if !enabled {
		use = "disable"
		short = "Disable a plugin"
	}
	return &cobra.Command{
		Use:   use + " <id-or-key>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]bool{"enabled": enabled}
			var updated domain.Plugin
			if err := g.Client().Patch(cmd.Context(), "/api/v1/plugins/"+url.PathEscape(args[0])+"/enabled", body, &updated); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), updated.Key)
			return nil
		},
	}
}
