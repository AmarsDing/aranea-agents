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
		Short: "Team ??",
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
		Short: "???? Team",
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
		Short: "?? Team ??",
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
		Short: "?? Team?? JSON ???",
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
			return cc.Printer.PrintSuccess("Team ???", "id", team.Id, "name", team.DisplayName)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Team ?????JSON??- ?? stdin")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func teamUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "?? Team",
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
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			updated, err := cc.Client.UpdateTeam(cmd.Context(), args[0], &team)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Team ???", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Team ?????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func teamDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "?? Team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("???? Team %q?", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "?????"}
				}
			}
			if err := cc.Client.DeleteTeam(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Team ???", "id", args[0])
		},
	}
	return cmd
}

func teamRunCmd() *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "?? Team ??",
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
			return cc.Printer.PrintSuccess("Team ?????", kv...)
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "??????")
	return cmd
}

func teamRunsCmd() *cobra.Command {
	var limit int32
	cmd := &cobra.Command{
		Use:   "runs [team-id]",
		Short: "?? Team ????",
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
	cmd.Flags().Int32Var(&limit, "limit", 20, "??????")
	return cmd
}

// ??? helpers ??????????????????????????????????????????????????????????????????

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
		return nil, &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
	}
	return req, nil
}
