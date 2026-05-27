package cmd

import (
	"fmt"
	"os"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/config"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates the `aranea config` command group.
func NewConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "管理本地 CLI 配置",
		Long:  "查看、修改、定位 aranea CLI 的本地配置文件（TOML）。",
	}
	c.AddCommand(
		configPathCmd(),
		configGetCmd(),
		configSetCmd(),
	)
	return c
}

func configPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "显示配置文件路径",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			path := cc.Cfg.Path()
			if path == "" {
				var err error
				path, err = config.DefaultPath()
				if err != nil {
					return err
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func configGetCmd() *cobra.Command {
	var showToken bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "获取配置项的值",
		Long: `获取配置项的值。

支持的 key:
  backend.base_url    后端地址
  backend.token       登录凭证（默认隐藏，--show-token 显示明文）
  backend.workspace_id 工作区 ID（预留）
  ui.output           默认输出格式
  ui.color            颜色模式

示例:
  aranea config get backend.base_url
  aranea config get backend.token`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			key := args[0]
			val, err := getConfigValue(cc.Cfg, key, showToken)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showToken, "show-token", false, "显示 token 明文（危险）")
	return cmd
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "设置配置项的值",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			key, val := args[0], args[1]
			if err := setConfigValue(cc.Cfg, key, val); err != nil {
				return err
			}
			if err := cc.Cfg.Save(""); err != nil {
				return &cli.CLIError{Code: "CONFIG_SAVE_ERROR", Message: "保存配置失败", Cause: err}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", key, val)
			return nil
		},
	}
}

func getConfigValue(cfg *config.CLIConfig, key string, showToken bool) (string, error) {
	switch key {
	case "backend.base_url":
		return cfg.Backend.BaseURL, nil
	case "backend.token":
		if showToken {
			fmt.Fprintln(os.Stderr, "警告: 显示 token 明文，请妥善保管")
			return cfg.Backend.Token, nil
		}
		return config.MaskToken(cfg.Backend.Token), nil
	case "backend.workspace_id":
		return cfg.Backend.WorkspaceID, nil
	case "ui.output":
		return cfg.UI.Output, nil
	case "ui.color":
		return cfg.UI.Color, nil
	case "skill.default_decision":
		return cfg.Skill.DefaultDecision, nil
	default:
		return "", &cli.CLIError{
			Code:    "CONFIG_KEY_UNKNOWN",
			Message: fmt.Sprintf("未知配置项: %q", key),
			Hint:    "运行 `aranea config get --help` 查看支持的 key",
		}
	}
}

func setConfigValue(cfg *config.CLIConfig, key, val string) error {
	switch key {
	case "backend.base_url":
		cfg.Backend.BaseURL = val
	case "backend.token":
		cfg.Backend.Token = val
	case "backend.workspace_id":
		cfg.Backend.WorkspaceID = val
	case "ui.output":
		if val != "text" && val != "json" {
			return &cli.CLIError{
				Code:    "CONFIG_VALUE_INVALID",
				Message: fmt.Sprintf("ui.output 只支持 text 或 json，got %q", val),
			}
		}
		cfg.UI.Output = val
	case "ui.color":
		if val != "auto" && val != "always" && val != "never" {
			return &cli.CLIError{
				Code:    "CONFIG_VALUE_INVALID",
				Message: fmt.Sprintf("ui.color 只支持 auto|always|never，got %q", val),
			}
		}
		cfg.UI.Color = val
	default:
		return &cli.CLIError{
			Code:    "CONFIG_KEY_UNKNOWN",
			Message: fmt.Sprintf("未知配置项: %q", key),
		}
	}
	return nil
}
