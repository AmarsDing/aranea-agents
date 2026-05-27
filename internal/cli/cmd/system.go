package cmd

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewSystemCmd creates the `aranea system` command group.
func NewSystemCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "system",
		Short: "系统信息与管理",
	}
	c.AddCommand(systemInfoCmd())
	return c
}

func systemInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "显示后端系统信息（需登录）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			info, err := cc.Client.GetSystemInfo(ctx)
			if err != nil {
				return err
			}

			na := func(s string) string {
				if s == "" {
					return "N/A"
				}
				return s
			}

			fmt.Fprintf(cmd.OutOrStdout(), "version              : %s\n", na(info.Version))
			fmt.Fprintf(cmd.OutOrStdout(), "git_commit           : %s\n", na(info.GitCommit))
			fmt.Fprintf(cmd.OutOrStdout(), "build_time           : %s\n", na(info.BuildTime))
			fmt.Fprintf(cmd.OutOrStdout(), "default_provider     : %s\n", na(info.DefaultProvider))
			fmt.Fprintf(cmd.OutOrStdout(), "default_model        : %s\n", na(info.DefaultModel))
			fmt.Fprintf(cmd.OutOrStdout(), "system_admin_agent_id: %s\n", na(info.SystemAdminAgentID))
			fmt.Fprintf(cmd.OutOrStdout(), "system_admin_key     : %s\n", na(info.SystemAdminAgentKey))
			fmt.Fprintf(cmd.OutOrStdout(), "skill_max_zip_mb     : %d\n", info.SkillMaxZipMB)
			fmt.Fprintf(cmd.OutOrStdout(), "skill_storage_root   : %s\n", na(info.SkillStorageRoot))
			return nil
		},
	}
}
