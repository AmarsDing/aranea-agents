// Package login 实现 `aranea login` 与 `aranea logout`。本地单进程部署
// 无需鉴权，本命令也便于将基址固定到活动 profile。
package login

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	cliconfig "arenea/backend/cmd/aranea/cli/config"
	"arenea/backend/cmd/aranea/cli/output"
)

// NewCommand 返回 login/logout 的父级命令。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Configure the active profile (base URL and bearer token)",
		RunE:  runLogin(g),
	}
	cmd.Flags().Bool("logout", false, "Remove the stored token from the active profile")
	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Remove the stored token from the active profile",
		RunE:  runLogout(g),
	})
	return cmd
}

func runLogin(g *apiclient.GlobalContext) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if logout, _ := cmd.Flags().GetBool("logout"); logout {
			return runLogout(g)(cmd, nil)
		}
		cfg, err := cliconfig.Load()
		if err != nil {
			return err
		}
		profile := g.Profile
		if profile == "" {
			profile = cfg.Default
		}
		if profile == "" {
			profile = "default"
		}
		prof, ok := cfg.Profiles[profile]
		if !ok {
			prof = &cliconfig.Profile{}
			cfg.Profiles[profile] = prof
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Fprintf(cmd.OutOrStdout(), "Backend URL [%s]: ", choose(prof.BaseURL, g.BaseURL))
		baseURL, _ := reader.ReadString('\n')
		baseURL = strings.TrimSpace(baseURL)
		if baseURL != "" {
			prof.BaseURL = baseURL
		} else if prof.BaseURL == "" {
			prof.BaseURL = g.BaseURL
		}

		fmt.Fprint(cmd.OutOrStdout(), "Bearer token (leave blank to keep current): ")
		token, _ := reader.ReadString('\n')
		token = strings.TrimSpace(token)
		if token != "" {
			prof.Token = token
		}
		if cfg.Default == "" {
			cfg.Default = profile
		}
		if err := cliconfig.Save(cfg); err != nil {
			return err
		}
		output.Success(cmd.OutOrStdout(), fmt.Sprintf("profile %q saved", profile))
		return nil
	}
}

func runLogout(g *apiclient.GlobalContext) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		cfg, err := cliconfig.Load()
		if err != nil {
			return err
		}
		profile := g.Profile
		if profile == "" {
			profile = cfg.Default
		}
		if profile == "" {
			profile = "default"
		}
		prof, ok := cfg.Profiles[profile]
		if !ok {
			return fmt.Errorf("profile %q not found", profile)
		}
		prof.Token = ""
		if err := cliconfig.Save(cfg); err != nil {
			return err
		}
		output.Success(cmd.OutOrStdout(), fmt.Sprintf("token cleared from profile %q", profile))
		return nil
	}
}

func choose(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
