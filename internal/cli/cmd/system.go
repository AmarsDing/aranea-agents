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

			return cc.Printer.PrintDetail(map[string]string{
				"version":                na(info.Version),
				"git_commit":             na(info.GitCommit),
				"build_time":             na(info.BuildTime),
				"default_provider":       na(info.DefaultProvider),
				"default_model":          na(info.DefaultModel),
				"system_admin_agent_id":  na(info.SystemAdminAgentID),
				"system_admin_key":       na(info.SystemAdminAgentKey),
				"skill_max_zip_mb":       fmt.Sprintf("%d", info.SkillMaxZipMB),
				"skill_storage_root":     na(info.SkillStorageRoot),
			})
		},
	}
}
