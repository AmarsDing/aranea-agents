package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	cli "aranea-agents/internal/cli"
	"aranea-agents/internal/cli/client"
	cmdpkg "aranea-agents/internal/cli/cmd"
	"aranea-agents/internal/cli/config"
	"aranea-agents/internal/cli/output"
	"aranea-agents/internal/cli/repl"
	"aranea-agents/internal/cli/ui"

	"github.com/spf13/cobra"
)

// pgoImportEnabled mirrors conf.PGOCLIImportEnabled() without importing
// internal/conf (which pulls in heavy kratos proto deps into the CLI binary).
func pgoImportEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("PGO_CLI_IMPORT_ENABLED")))
	return v != "0" && v != "false" && v != "no"
}

// Version and Commit are injected by ldflags at build time.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = ""
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "aranea: internal error; rerun with --debug or report this command if it persists")
			os.Exit(cli.ExitNetworkError)
		}
	}()
	bi := cli.BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
	ctx := context.Background()
	if err := execute(ctx, bi); err != nil {
		os.Exit(cli.ExitCodeOf(err))
	}
}

func execute(ctx context.Context, bi cli.BuildInfo) error {
	root := newRoot(ctx, bi)
	return root.ExecuteContext(ctx)
}

func newRoot(ctx context.Context, bi cli.BuildInfo) *cobra.Command {
	var (
		cfgPath    string
		baseURL    string
		token      string
		outputFmt  string
		quiet      bool
		debug      bool
		autoYes    bool
		noColor    bool
		timeoutSec int
	)

	root := &cobra.Command{
		Use:           "aranea",
		Short:         "Aranea 终端控制台",
		Long:          "Aranea CLI — 通过 HTTP API 管理 Agent / Skill / Tool 等资源",
		Version:       bi.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// No subcommand given — launch the interactive REPL.
			cc := cli.CLIFrom(cmd.Context())
			r := repl.New(repl.Config{
				APIBase:  cc.Client.Base,
				Token:    cc.Client.Token,
				AgentKey: "__system_admin__",
			})
			return r.Run(cmd.Context())
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip for __complete, help, version
			if cmd.Name() == "__complete" || cmd.Name() == "help" {
				return nil
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				_ = err
				cfg, _ = config.Load("")
				if cfg == nil {
					cfg = &config.CLIConfig{}
				}
			}
			cfg.OverrideFromEnv()
			cfg.OverrideFromFlags(baseURL, token, outputFmt, noColor)

			u := ui.Detect(os.Stdin, os.Stdout, os.Stderr, cfg.UI.Color == "never")
			noColorActual := cfg.UI.Color == "never" || cfg.UI.Color == "auto" && !u.IsTTY

			format := output.FormatText
			if cfg.UI.Output == "json" {
				format = output.FormatJSON
			}

			var logOut io.Writer = os.Stderr
			if !debug {
				logOut = io.Discard
			}
			logger := log.New(logOut, "[aranea] ", log.LstdFlags)

			c := client.NewClientWithTimeout(
				cfg.Backend.BaseURL,
				cfg.Backend.Token,
				bi.Version,
				debug,
				func(format string, args ...any) {
					logger.Printf(format, args...)
				},
				time.Duration(timeoutSec)*time.Second,
			)

			printer := output.NewPrinter(format, quiet, noColorActual, os.Stdout)

			cc := &cli.Context{
				Cfg:     cfg,
				Client:  c,
				Printer: printer,
				UI:      u,
				Logger:  logger,
				Debug:   debug,
				Quiet:   quiet,
				AutoYes: autoYes,
				BI:      bi,
			}
			cmd.SetContext(cli.WithCLI(cmd.Context(), cc))
			return nil
		},
	}

	// Global persistent flags.
	f := root.PersistentFlags()
	f.StringVar(&cfgPath, "config", "", "config file path (default: platform config dir)")
	f.StringVar(&baseURL, "base-url", "", "override [backend].base_url")
	f.StringVar(&token, "token", "", "override [backend].token")
	f.StringVarP(&outputFmt, "output", "o", "", "output format: text | json")
	f.BoolVarP(&quiet, "quiet", "q", false, "minimal output")
	f.BoolVarP(&autoYes, "yes", "y", false, "skip interactive confirmations")
	f.BoolVar(&debug, "debug", false, "log HTTP requests/responses to stderr")
	f.BoolVar(&noColor, "no-color", false, "disable ANSI colors (also: NO_COLOR=1)")
	f.IntVar(&timeoutSec, "timeout", 60, "global HTTP timeout in seconds")

	// Register subcommands.
	root.AddCommand(
		cmdpkg.NewVersionCmd(bi.Version, bi.Commit, bi.BuildTime),
		cmdpkg.NewLoginCmd(),
		cmdpkg.NewConfigCmd(),
		cmdpkg.NewSystemCmd(),
		cmdpkg.NewAgentCmd(),
		cmdpkg.NewSkillCmd(),
		cmdpkg.NewToolCmd(),
		// Phase B: P1 resource commands.
		cmdpkg.NewTeamCmd(),
		cmdpkg.NewPluginCmd(),
		cmdpkg.NewMCPCmd(),
		cmdpkg.NewCronCmd(),
		cmdpkg.NewChannelCmd(),
		cmdpkg.NewGraphCmd(),
		cmdpkg.NewSessionCmd(),
		// Phase A: package install.
		cmdpkg.NewPkgCmd(),
		// Phase D: chat / REPL.
		cmdpkg.NewChatCmd(),
	)
	// PGO_CLI_IMPORT_ENABLED defaults to on; set to 0/false/no to disable.
	if pgoImportEnabled() {
		root.AddCommand(cmdpkg.NewImportCmd())
	}
	return root
}
