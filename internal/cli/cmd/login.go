package cmd

import (
	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewLoginCmd creates the `aranea login` command.
func NewLoginCmd() *cobra.Command {
	var user, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "登录后端并将 token 保存到本地配置",
		Long: `登录后端，获取 JWT token 并保存到 config.toml（文件权限 0600）。

示例:
  aranea login --base-url http://127.0.0.1:8080 --user admin --password secret`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())

			admin, token, err := cc.Client.Login(cmd.Context(), user, password)
			if err != nil {
				return err
			}

			if token == "" {
				return &cli.CLIError{
					Code:    "LOGIN_NO_TOKEN",
					Message: "登录成功但未收到 token；请检查后端是否正常",
					Hint:    "确认后端返回了 Set-Cookie 头",
				}
			}

			// Save token to config.
			cc.Cfg.Backend.Token = token
			if err := cc.Cfg.Save(""); err != nil {
				return &cli.CLIError{Code: "CONFIG_SAVE_ERROR", Message: "保存配置失败", Cause: err}
			}

			cfgPath := cc.Cfg.Path()
			if cfgPath == "" {
				cfgPath, _ = config.DefaultPath()
			}

			// Print success (no token in stdout).
			name := ""
			if admin != nil {
				name = admin.Name
			}
			return cc.Printer.PrintSuccess("token saved",
				"user", name,
				"path", cfgPath,
			)
		},
	}

	cmd.Flags().StringVar(&user, "user", "", "用户名 (required)")
	cmd.Flags().StringVar(&password, "password", "", "密码 (required)")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}
