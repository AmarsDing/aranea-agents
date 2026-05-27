package cmd

import (
	"fmt"

	channelv1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewChannelCmd creates the `aranea channel` command group.
func NewChannelCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "channel",
		Short: "Channel ??",
	}
	c.AddCommand(
		channelLsCmd(),
		channelGetCmd(),
		channelAddCmd(),
		channelUpdateCmd(),
		channelDeleteCmd(),
		channelTestCmd(),
		channelToggleCmd(),
	)
	return c
}

func channelLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "???? Channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListChannels(cmd.Context())
			if err != nil {
				return err
			}
			rows := channelsToRows(resp.Items)
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func channelGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "?? Channel ??",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			channel, err := cc.Client.GetChannel(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return cc.Printer.PrintDetail(channelToRow(channel))
		},
	}
}

func channelAddCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "add --file <file>",
		Short: "?? Channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &channelv1.CreateChannelRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			channel, err := cc.Client.CreateChannel(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Channel ???", "id", channel.Id, "name", channel.Name)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Channel ?????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func channelUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "?? Channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &channelv1.UpdateChannelRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			req.Id = args[0]
			updated, err := cc.Client.UpdateChannel(cmd.Context(), args[0], req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Channel ???", "id", updated.Id)
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Channel ?????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func channelDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "?? Channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("???? Channel %q?", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "?????"}
				}
			}
			if err := cc.Client.DeleteChannel(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Channel ???", "id", args[0])
		},
	}
	return cmd
}

func channelTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "?? Channel ???",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			result, err := cc.Client.TestChannel(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if result.Ok {
				return cc.Printer.PrintSuccess("Channel ?????", "status", result.Status)
			}
			return cc.Printer.PrintSuccess("Channel ?????", "status", result.Status, "message", result.Message)
		},
	}
}

func channelToggleCmd() *cobra.Command {
	var enable bool
	cmd := &cobra.Command{
		Use:   "toggle <id>",
		Short: "?? Channel ??/??",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			channel, err := cc.Client.ToggleChannel(cmd.Context(), args[0], enable)
			if err != nil {
				return err
			}
			state := "??"
			if channel.Enabled {
				state = "??"
			}
			return cc.Printer.PrintSuccess("Channel ?"+state, "id", channel.Id)
		},
	}
	cmd.Flags().BoolVar(&enable, "enable", true, "???true?????false?")
	return cmd
}

// ??? helpers ??????????????????????????????????????????????????????????????????

func channelsToRows(items []*channelv1.Channel) []map[string]string {
	rows := make([]map[string]string, 0, len(items))
	for _, ch := range items {
		rows = append(rows, channelToRow(ch))
	}
	return rows
}

func channelToRow(ch *channelv1.Channel) map[string]string {
	enabled := "false"
	if ch.Enabled {
		enabled = "true"
	}
	return map[string]string{
		"id":       ch.Id,
		"key":      ch.Key,
		"name":     ch.Name,
		"provider": ch.Provider,
		"enabled":  enabled,
		"status":   ch.Status,
	}
}
