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
		Short: "??????",
	}
	c.AddCommand(
		cronLsCmd(),
		cronGetCmd(),
		cronAddCmd(),
		cronUpdateCmd(),
		cronDeleteCmd(),
		cronTriggerCmd(),
		cronRunsCmd(),
	)
	return c
}

func cronLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "????????",
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
		Short: "????????",
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
		Short: "??????",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &cronv1.CreateCronTaskRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			task, err := cc.Client.CreateCronTask(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("???????", "id", task.Id, "name", task.Name)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "???????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func cronUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "??????",
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
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			updated, err := cc.Client.UpdateCronTask(cmd.Context(), args[0], &task)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("???????", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "???????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func cronDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "??????",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("???????? %q?", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "?????"}
				}
			}
			if err := cc.Client.DeleteCronTask(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("???????", "id", args[0])
		},
	}
	return cmd
}

func cronTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger <id>",
		Short: "????????",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			run, err := cc.Client.TriggerCronTask(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("???????", "run_id", run.Id)
		},
	}
}

func cronRunsCmd() *cobra.Command {
	var taskID string
	var limit int32
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "??????????",
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
	cmd.Flags().StringVar(&taskID, "task-id", "", "?????? ID")
	cmd.Flags().Int32Var(&limit, "limit", 20, "??????")
	return cmd
}

// ??? helpers ??????????????????????????????????????????????????????????????????

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
