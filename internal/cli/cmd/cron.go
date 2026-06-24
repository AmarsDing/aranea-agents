package cmd

import (
	"fmt"

	cronv1 "aranea-agents/api/kratos/cron/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewCronCmd creates the `aranea cron` command group.
func NewCronCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cron",
		Short: "定时任务管理",
	}
	c.AddCommand(
		cronLsCmd(),
		cronGetCmd(),
		cronAddCmd(),
		cronUpdateCmd(),
		cronDeleteCmd(),
		cronTriggerCmd(),
		cronRunsCmd(),
		cronPauseCmd(),
		cronResumeCmd(),
	)
	return c
}

func cronLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "列出所有定时任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCronTasks(cmd.Context())
			if err != nil {
				return err
			}
			rows := cronTasksToRows(resp.Items)
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func cronGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "查看定时任务详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			task, err := cc.Client.GetCronTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(cronTaskToRow(task))
		},
	}
}

func cronAddCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "add --file <file>",
		Short: "添加定时任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &cronv1.CreateCronTaskRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
			}
			task, err := cc.Client.CreateCronTask(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务创建成功", "id", task.Id, "name", task.Name)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "定时任务配置文件路径（YAML/JSON）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func cronUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "更新定时任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			var task cronv1.CronTask
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, &task); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("文件解析失败: %v", err)}
			}
			updated, err := cc.Client.UpdateCronTask(cmd.Context(), args[0], &task)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务更新成功", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "定时任务配置文件路径（YAML/JSON）")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func cronDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除定时任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("确认删除定时任务 %q？此操作不可撤销", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "操作已取消"}
				}
			}
			if err := cc.Client.DeleteCronTask(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务已删除", "id", args[0])
		},
	}
	return cmd
}

func cronTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <id>",
		Short: "手动触发定时任务",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			run, err := cc.Client.TriggerCronTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务已触发", "run_id", run.Id)
		},
	}
}

func cronRunsCmd() *cobra.Command {
	var taskID string
	var limit int32
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "查看定时任务执行记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListCronTaskRuns(cmd.Context(), taskID, limit)
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, r := range resp.Items {
				rows = append(rows, map[string]string{
					"id":         r.Id,
					"task_id":    r.TaskId,
					"status":     r.Status,
					"trigger":    r.Trigger,
					"started_at": r.StartedAt,
				})
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
	cmd.Flags().StringVar(&taskID, "task-id", "", "定时任务 ID")
	cmd.Flags().Int32Var(&limit, "limit", 20, "返回数量")
	return cmd
}

func cronPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "暂停定时任务（enabled=false）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			task, err := cc.Client.GetCronTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			task.Enabled = false
			updated, err := cc.Client.UpdateCronTask(cmd.Context(), args[0], task)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务已暂停", "id", updated.Id, "enabled", "false")
		},
	}
}

func cronResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "恢复定时任务（enabled=true）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			task, err := cc.Client.GetCronTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			task.Enabled = true
			updated, err := cc.Client.UpdateCronTask(cmd.Context(), args[0], task)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("定时任务已恢复", "id", updated.Id, "enabled", "true")
		},
	}
}

// Row helpers convert proto items to display rows.

func cronTasksToRows(items []*cronv1.CronTask) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, t := range items {
		rows = append(rows, cronTaskToRow(t))
	}
	return rows
}

func cronTaskToRow(t *cronv1.CronTask) map[string]string {
	enabled := "false"
	if t.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":       t.Id,
		"task_key": t.TaskKey,
		"name":     t.Name,
		"status":   t.Status,
		"enabled":  enabled,
	}
}
