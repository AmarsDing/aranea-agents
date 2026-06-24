package cmd

import (
	"fmt"

	teamv1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewTeamCmd creates the `aranea team` command group.
func NewTeamCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "team",
		Short: "Team 管理",
	}
	c.AddCommand(
		teamLsCmd(),
		teamGetCmd(),
		teamCreateCmd(),
		teamUpdateCmd(),
		teamDeleteCmd(),
		teamRunCmd(),
		teamRunsCmd(),
	)
	return c
}

func teamLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出所有 Team",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListTeams(cmd.Context())
			if err != nil {
				return err
			}
			rows := teamsToRows(resp.Items)
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func teamGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看 Team 详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			team, err := cc.Client.GetTeam(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(teamToRow(team))
		},
	}
}

func teamCreateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "create --file <file>",
		Short: "创建 Team（JSON 格式）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			req, err := loadTeamCreateFromFile(filePath)
			if err != nil {
				return err
			}
			team, err := cc.Client.CreateTeam(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Team 创建成功", "id", team.Id, "name", team.DisplayName)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Team 配置文件路径（YAML/JSON，- 表示 stdin）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func teamUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "更新 Team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			var team teamv1.Team
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, &team); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
			}
			updated, err := cc.Client.UpdateTeam(cmd.Context(), args[0], &team)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Team 更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Team 配置文件路径（YAML/JSON）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func teamDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除 Team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除 Team %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteTeam(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Team 已删除", "id", args[0])
		},
	}
	return cmd
}

func teamRunCmd() *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "运行 Team 测试",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.RunTeamTest(cmd.Context(), args[0], content)
			if err != nil {
				return err
			}
			kv := []string{"run_id", resp.Run.GetId()}
			if resp.Reply != "" {
				kv = append(kv, "reply", resp.Reply)
			}
			return cc.Printer.PrintSuccess("Team 运行完成", kv...)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "测试输入内容")
	return cmd
}

func teamRunsCmd() *cobra.Command {
	var limit int32
	cmd := &cobra.Command{
		Use:   "runs [team-id]",
		Short: "查看 Team 运行记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			teamID := ""
			if len(args) > 0 {
				teamID = args[0]
			}
			resp, err := cc.Client.ListTeamRuns(cmd.Context(), teamID, limit)
			if err != nil {
				return err
			}
			rows := teamRunsToRows(resp.Items)
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	return cmd
}

// Row helpers convert proto items to display rows.

func teamsToRows(items []*teamv1.Team) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, t := range items {
		rows = append(rows, teamToRow(t))
	}
	return rows
}

func teamToRow(t *teamv1.Team) map[string]string {
	return map[string]string{
		"id":           t.Id,
		"team_key":     t.TeamKey,
		"display_name": t.DisplayName,
		"status":       t.Status,
	}
}

func teamRunsToRows(items []*teamv1.TeamRun) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, r := range items {
		rows = append(rows, map[string]string{
			"id":         r.Id,
			"team_id":    r.TeamId,
			"status":     r.Status,
			"mode":       r.Mode,
			"started_at": r.StartedAt,
		})
	}
	return rows
}

func loadTeamCreateFromFile(path string) (*teamv1.CreateTeamRequest, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
	}
	req := &teamv1.CreateTeamRequest{}
	uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := uopts.Unmarshal(data, req); err != nil {
		return nil, &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
	}
	return req, nil
}
