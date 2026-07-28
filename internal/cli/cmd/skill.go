package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

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
		skillFilesCmd(),
		skillFileGetCmd(),
		skillFilePutCmd(),
		skillFileDeleteCmd(),
		skillImportCmd(),
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

func skillFilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "files <id>",
		Short: "列出 Skill 的文件",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListSkillFiles(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, f := range resp.Items {
				rows = append(rows, map[string]string{
					"path":       f.Path,
					"name":       f.Name,
					"language":   f.Language,
					"size":       fmt.Sprintf("%d", f.Size),
					"updated_at": f.UpdatedAt,
				})
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func skillFileGetCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "file-get <id>",
		Short: "查看 Skill 单个文件内容",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.GetSkillFile(cmd.Context(), args[0], path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), resp.Content)
			return err
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "文件在 Skill 包内的相对路径")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func skillFilePutCmd() *cobra.Command {
	var path, filePath string
	cmd := &cobra.Command{
		Use:   "file-put <id>",
		Short: "上传/覆盖 Skill 单个文件",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			resp, err := cc.Client.UpdateSkillFile(cmd.Context(), args[0], path, string(data))
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("文件已更新", "path", resp.Path)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "文件在 Skill 包内的相对路径")
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "本地文件路径（- 表示 stdin）")
	_ = cmd.MarkFlagRequired("path")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func skillFileDeleteCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "file-delete <id>",
		Short: "删除 Skill 单个文件（需确认）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(
					fmt.Sprintf("确认删除 Skill %q 的文件 %q？", args[0], path), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteSkillFile(cmd.Context(), args[0], path); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("文件已删除", "path", path)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "文件在 Skill 包内的相对路径")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func skillImportCmd() *cobra.Command {
	var filePath string
	var apply bool
	var waitTimeout time.Duration
	cmd := &cobra.Command{
		Use:   "import --file <zip>",
		Short: "导入 Skill ZIP 包（--apply 自动安装通过校验的候选）",
		Long: `上传 Skill ZIP 包创建导入任务，轮询直至分析完成，并列出候选。

默认只预览；传 --apply 时对所有「校验通过且无阻断项」的候选执行 import_passed 决策。
存在警告/阻断/冲突组的候选不会自动处理，请在 Web UI 中手动决策。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			ctx := cmd.Context()
			uploaded, err := cc.Client.ImportSkillZip(ctx, filepath.Base(filePath), data)
			if err != nil {
				return err
			}
			job, err := waitSkillImportJob(ctx, cc, uploaded.JobId, waitTimeout)
			if err != nil {
				return err
			}
			if err := printImportJob(cc, job); err != nil {
				return err
			}
			if !apply {
				return nil
			}
			return applySkillImport(cmd, cc, job)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Skill ZIP 文件路径")
	cmd.Flags().BoolVar(&apply, "apply", false, "分析完成后自动导入通过校验的候选")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 2*time.Minute, "等待导入任务分析完成的最长时间")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// waitSkillImportJob polls the import job until analysis completes or fails.
func waitSkillImportJob(ctx context.Context, cc *cli.Context, jobID string, timeout time.Duration) (*skillv1.SkillImportJob, error) {
	deadline := time.Now().Add(timeout)
	for {
		job, err := cc.Client.GetSkillImportJob(ctx, jobID)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "completed", "applied":
			return job, nil
		case "failed":
			return nil, &cli.CLIError{Code: "IMPORT_FAILED", Message: fmt.Sprintf("导入任务分析失败: %s", job.Message)}
		}
		if time.Now().After(deadline) {
			return nil, &cli.CLIError{Code: "IMPORT_TIMEOUT", Message: fmt.Sprintf("等待导入任务 %s 超时（当前状态: %s）", jobID, job.Status)}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// printImportJob renders the job's candidate list.
func printImportJob(cc *cli.Context, job *skillv1.SkillImportJob) error {
	rows := make([]map[string]string, 0, len(job.Candidates))
	for _, c := range job.Candidates {
		rows = append(rows, map[string]string{
			"candidate_id":      c.CandidateId,
			"name":              c.Name,
			"slug":              c.Slug,
			"validation_status": c.ValidationStatus,
			"warnings":          fmt.Sprintf("%d", len(c.Warnings)),
			"blocks":            fmt.Sprintf("%d", len(c.Blocks)),
		})
	}
	return cc.Printer.PrintList(rows, len(rows))
}

// applySkillImport builds decisions for auto-importable candidates and applies them.
func applySkillImport(cmd *cobra.Command, cc *cli.Context, job *skillv1.SkillImportJob) error {
	decisions, pending := buildImportDecisions(job)
	if len(decisions) == 0 {
		return &cli.CLIError{Code: "NOTHING_TO_IMPORT", Message: "没有可自动导入的候选（均存在警告/阻断/冲突，请在 Web UI 中手动决策）"}
	}
	if !cc.AutoYes {
		ok, err := cc.UI.ConfirmYesNo(
			fmt.Sprintf("确认导入 %d 个候选 Skill？", len(decisions)), false)
		if err != nil || !ok {
			return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
		}
	}
	result, err := cc.Client.ApplySkillImport(cmd.Context(), job.JobId, decisions)
	if err != nil {
		return err
	}
	kv := []string{"created", fmt.Sprintf("%d", len(result.CreatedSkillIds))}
	if len(pending) > 0 {
		kv = append(kv, "pending_manual", fmt.Sprintf("%d", len(pending)))
	}
	if result.Message != "" {
		kv = append(kv, "message", result.Message)
	}
	return cc.Printer.PrintSuccess("导入完成", kv...)
}

// buildImportDecisions maps job candidates to import decisions. Candidates
// that passed validation with no blocks get "import_passed"; the rest are
// returned as pending (require manual resolution in the Web UI).
func buildImportDecisions(job *skillv1.SkillImportJob) (decisions []*skillv1.SkillImportDecision, pending []string) {
	for _, c := range job.Candidates {
		if c.ValidationStatus == "pass" && len(c.Blocks) == 0 {
			decisions = append(decisions, &skillv1.SkillImportDecision{
				CandidateId: c.CandidateId,
				Action:      "import_passed",
			})
			continue
		}
		pending = append(pending, c.CandidateId)
	}
	return decisions, pending
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
