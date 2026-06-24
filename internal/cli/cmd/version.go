package cmd

import (
	"context"
	"time"

	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
)

// NewVersionCmd creates the `aranea version` command.
func NewVersionCmd(version, commit, buildTime string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "显示版本信息（含后端可达性探测）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())

			bTime := buildTime
			if bTime == "" {
				bTime = "unknown"
			}

			row := map[string]string{
				"cli_version": version,
				"commit":      commit,
				"build_time":  bTime,
			}

			// Probe backend reachability.
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			err := cc.Client.CheckReachability(ctx)
			if err != nil {
				row["backend_url"] = cc.Cfg.Backend.BaseURL
				row["backend_status"] = "unreachable"
				return cc.Printer.PrintDetail(row)
			}
			row["backend_status"] = "reachable"

			// Try to get system info if we have a token.
			if cc.Cfg.Backend.Token != "" {
				info, err := cc.Client.GetSystemInfo(ctx)
				if err == nil && info != nil {
					backendVer := info.Version
					if backendVer == "" {
						backendVer = "unknown"
					}
					row["backend_url"] = cc.Cfg.Backend.BaseURL
					row["backend_version"] = backendVer
					return cc.Printer.PrintDetail(row)
				}
			}

			row["backend_url"] = cc.Cfg.Backend.BaseURL
			return cc.Printer.PrintDetail(row)
		},
	}
	return cmd
}
