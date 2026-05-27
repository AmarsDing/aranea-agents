package cmd

import (
	"context"
	"fmt"
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

			// Always print local version.
			bTime := buildTime
			if bTime == "" {
				bTime = "unknown"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "aranea %s (commit=%s build=%s)\n",
				version, commit, bTime)

			// Probe backend reachability.
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			err := cc.Client.CheckReachability(ctx)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "backend: unreachable (%s)\n", cc.Cfg.Backend.BaseURL)
				// Do not set exit code for unreachability in version command.
				return nil
			}

			// Try to get system info if we have a token.
			if cc.Cfg.Backend.Token != "" {
				info, err := cc.Client.GetSystemInfo(ctx)
				if err == nil && info != nil {
					backendVer := info.Version
					if backendVer == "" {
						backendVer = "unknown"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "backend: %s ✓ (version=%s)\n",
						cc.Cfg.Backend.BaseURL, backendVer)
					return nil
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "backend: %s ✓\n", cc.Cfg.Backend.BaseURL)
			return nil
		},
	}
	return cmd
}
