// Package channel 实现 `aranea channel ls/catalog`。
package channel

import (
	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/cli/apiclient"
	"arenea/backend/cmd/aranea/cli/output"
	"arenea/backend/internal/domain"
)

// NewCommand 返回父级命令。
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Inspect channels and the channel catalog",
	}
	cmd.AddCommand(newListCmd(g), newCatalogCmd(g))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List configured channel runtimes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp struct {
				Items []domain.ChannelRuntimeConfig `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/channels", nil, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
}

func newCatalogCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "List the channel catalog",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp struct {
				Items []domain.ChannelCatalogItem `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/channels/catalog", nil, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
}
