package cmd

import (
	"fmt"

	skillv1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewSkillCmd creates the `aranea skill` command group.
func NewSkillCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "skill",
		Short: "Skill 管理",
	}
	c.AddCommand(
		skillLsCmd(),
		skillGetCmd(),
		skillCreateCmd(),
		skillUpdateCmd(),
		skillDeleteCmd(),
		skillEnableCmd(),
		skillDisableCmd(),
		skillPublishCmd(),
	)
	return c
}

func skillLsCmd() *cobra.Command {
	var limit, offset int32
	var search string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "列出所有 Skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListSkills(cmd.Context(), search, limit, offset)
			if err != nil {
				return err
			}
			rows := skillsToRows(resp.Items)
			return cc.Printer.PrintList(rows, int(resp.Total))
		},
	}
	cmd.Flags().Int32Var(&limit, "page-size", 20, "每页数量")
	cmd.Flags().Int32Var(&offset, "offset", 0, "偏移量")
	cmd.Flags().StringVar(&search, "search", "", "搜索关键词")
	return cmd
}

func skillGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看 Skill 详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.GetSkill(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(skillToRow(resp.Skill))
		},
	}
}

func skillCreateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "create --file <file>",
		Short: "创建 Skill（从 JSON 文件）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &skillv1.CreateSkillRequest{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: err.Error()}
			}
			skill, err := cc.Client.CreateSkill(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 创建成功", "id", skill.Id, "slug", skill.Slug)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "JSON 文件路径（- 表示 stdin）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func skillUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "更新 Skill（从 JSON 文件）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &skillv1.UpdateSkillRequest{}
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: err.Error()}
			}
			skill, err := cc.Client.UpdateSkill(cmd.Context(), args[0], req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 更新成功", "id", skill.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "JSON 文件路径（- 表示 stdin）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func skillDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除 Skill（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认删除 Skill %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteSkill(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 已删除", "id", args[0])
		},
	}
}

func skillEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用 Skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			skill, err := cc.Client.ToggleSkillEnabled(cmd.Context(), args[0], true)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 已启用", "id", skill.Id)
		},
	}
}

func skillDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "停用 Skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认停用 Skill %q？", args[0]), true)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			skill, err := cc.Client.ToggleSkillEnabled(cmd.Context(), args[0], false)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 已停用", "id", skill.Id)
		},
	}
}

func skillPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <id>",
		Short: "发布 Skill（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认发布 Skill %q？发布后其他 Agent 可使用此 Skill", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			skill, err := cc.Client.PublishSkill(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Skill 已发布", "id", skill.Id, "slug", skill.Slug)
		},
	}
}

// skillToRow converts a Skill to a display row.
func skillToRow(s *skillv1.Skill) map[string]string {
	if s == nil {
		return nil
	}
	enabled := "false"
	if s.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":      s.Id,
		"slug":    s.Slug,
		"name":    s.Name,
		"status":  s.Status,
		"enabled": enabled,
	}
}

// skillsToRows converts a slice of Skill to display rows.
func skillsToRows(skills []*skillv1.Skill) []map[string]string {
	rows := make([]map[string]string, 0, len(skills))
	for _, s := range skills {
		rows = append(rows, skillToRow(s))
	}
	return rows
}
