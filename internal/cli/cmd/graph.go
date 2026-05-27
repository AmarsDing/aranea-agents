package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/cli"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewGraphCmd creates the `aranea graph` command group.
func NewGraphCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "graph",
		Short: "Graph ??",
	}
	c.AddCommand(
		graphLsCmd(),
		graphGetCmd(),
		graphCreateCmd(),
		graphUpdateCmd(),
		graphDeleteCmd(),
		graphImportCmd(),
		graphExportCmd(),
	)
	return c
}

func graphLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "???? Graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ListGraphs(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]string, 0, len(resp.Items))
			for _, g := range resp.Items {
				rows = append(rows, map[string]string{
					"id":          g.Id,
					"name":        g.Name,
					"description": g.Description,
					"version":     fmt.Sprintf("%d", g.Version),
				})
			}
			return cc.Printer.PrintList(rows, len(resp.Items))
		},
	}
}

func graphGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "?? Graph ??",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.GetGraph(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			g := resp.Graph
			return cc.Printer.PrintDetail(map[string]string{
				"id":          g.Id,
				"name":        g.Name,
				"description": g.Description,
				"entry_point": g.EntryPoint,
				"version":     fmt.Sprintf("%d", g.Version),
			})
		},
	}
}

func graphCreateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "create --file <file>",
		Short: "?? Graph?? JSON ???",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &graphv1.CreateGraphRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			resp, err := cc.Client.CreateGraph(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Graph ???", "id", resp.Graph.GetId(), "name", resp.Graph.GetName())
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Graph ?????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func graphUpdateCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "update <id> --file <file>",
		Short: "?? Graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			req := &graphv1.UpdateGraphRequest{}
			uopts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := uopts.Unmarshal(data, req); err != nil {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: fmt.Sprintf("??????: %v", err)}
			}
			req.Id = args[0]
			resp, err := cc.Client.UpdateGraph(cmd.Context(), args[0], req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Graph ???", "id", resp.Graph.GetId())
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Graph ?????JSON?")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func graphDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "?? Graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			if !cc.AutoYes {
				ok, err := cc.UI.ConfirmYesNo(fmt.Sprintf("???? Graph %q?", args[0]), false)
				if err != nil || !ok {
					return &cli.CLIError{Code: "USER_CANCELED", Message: "?????"}
				}
			}
			if err := cc.Client.DeleteGraph(cmd.Context(), args[0]); err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Graph ???", "id", args[0])
		},
	}
	return cmd
}

func graphImportCmd() *cobra.Command {
	var filePath, name, description string
	cmd := &cobra.Command{
		Use:   "import --file <file>",
		Short: "?? Graph?POST /v1/graph/import?",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			data, err := readFile(filePath)
			if err != nil {
				return &cli.CLIError{Code: "FILE_READ_ERROR", Message: err.Error()}
			}
			if !json.Valid(data) {
				return &cli.CLIError{Code: "FILE_PARSE_ERROR", Message: "??????? JSON"}
			}
			req := &graphv1.ImportGraphRequest{
				Json:        string(data),
				Name:        name,
				Description: description,
			}
			resp, err := cc.Client.ImportGraph(cmd.Context(), req)
			if err != nil {
				return err
			}
			return cc.Printer.PrintSuccess("Graph ???", "id", resp.Graph.GetId(), "name", resp.Graph.GetName())
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Graph JSON ???- ?? stdin")
	cmd.Flags().StringVar(&name, "name", "", "?? Graph ??")
	cmd.Flags().StringVar(&description, "description", "", "?? Graph ??")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func graphExportCmd() *cobra.Command {
	var outputPath string
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "?? Graph ? JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := cli.CLIFrom(cmd.Context())
			resp, err := cc.Client.ExportGraph(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			b, err := protojson.MarshalOptions{Indent: "  "}.Marshal(resp)
			if err != nil {
				return err
			}
			if outputPath != "" && outputPath != "-" {
				if err := os.WriteFile(outputPath, b, 0600); err != nil {
					return err
				}
				return cc.Printer.PrintSuccess("Graph ???", "file", outputPath)
			}
			_, _ = os.Stdout.Write(b)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "-", "???????- ?? stdout")
	return cmd
}
