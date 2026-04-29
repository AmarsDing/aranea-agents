// Package system 实现 `aranea system health`。
package system

import (
	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
)

// NewCommand 返回父级命令。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Inspect backend health",
	}
	cmd.AddCommand(newHealthCmd(g))
	return cmd
}

func newHealthCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Print the backend's health probe response",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp map[string]any
			if err := g.Client().Get(cmd.Context(), "/healthz", nil, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
}
