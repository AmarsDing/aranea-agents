// Package skill 实现 `aranea skill ...`。只读类命令直接请求 /api/v1/skills；
// install / import / apply 流程由 install.go 在客户端编排，因需 clone 仓库、
// 校验 frontmatter 并在提交到 import API 前交互式解决冲突。
package skill

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
		Use:   "skill",
		Short: "Inspect, import and install skills",
	}
	cmd.AddCommand(
		newListCmd(g),
		newGetCmd(g),
		newEnableCmd(g, true),
		newEnableCmd(g, false),
		newDeleteCmd(g),
		newInstallCmd(g),
		newImportCmd(g),
	)
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		search  string
		tags    string
		enabled string
		status  string
		limit   int
		offset  int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if search != "" {
				q.Set("search", search)
			}
			if tags != "" {
				q.Set("tags", tags)
			}
			if enabled != "" {
				q.Set("enabled", enabled)
			}
			if status != "" {
				q.Set("status", status)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			var result domain.SkillListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/skills", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Search keyword")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tag filter")
	cmd.Flags().StringVar(&enabled, "enabled", "", "Filter by enabled flag (true|false)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id-or-slug>",
		Short: "Show a single skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var skill domain.Skill
			if err := g.Client().Get(cmd.Context(), "/api/v1/skills/"+url.PathEscape(args[0]), nil, &skill); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), skill)
			return nil
		},
	}
}

func newEnableCmd(g *apiclient.GlobalContext, enabled bool) *cobra.Command {
	use := "enable"
	short := "Enable a skill"
	if !enabled {
		use = "disable"
		short = "Disable a skill"
	}
	return &cobra.Command{
		Use:   use + " <id-or-slug>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]bool{"enabled": enabled}
			var updated domain.Skill
			if err := g.Client().Patch(cmd.Context(), "/api/v1/skills/"+url.PathEscape(args[0])+"/enabled", body, &updated); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), updated.Slug)
			return nil
		},
	}
}

func newDeleteCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-slug>",
		Short: "Delete a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm(cmd, g, "Delete skill "+args[0]+"?") {
				return nil
			}
			if err := g.Client().Delete(cmd.Context(), "/api/v1/skills/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), "deleted "+args[0])
			return nil
		},
	}
}
