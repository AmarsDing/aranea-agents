package cli

import (
	"context"
	"io"
	"log"
	"os"

	"aranea-agents/internal/cli/client"
	"aranea-agents/internal/cli/config"
	"aranea-agents/internal/cli/output"
	"aranea-agents/internal/cli/ui"
)

type contextKey struct{}

// Context holds per-command dependencies threaded via cobra.Command.Context().
type Context struct {
	Cfg     *config.CLIConfig
	Client  *client.Client
	Printer output.Printer
	UI      ui.UI
	Logger  *log.Logger
	Debug   bool
	Quiet   bool
	AutoYes bool
	BI      BuildInfo
}

// ContextOpts carries optional overrides when building a Context.
type ContextOpts struct {
	Debug     bool
	Quiet     bool
	AutoYes   bool
	BuildInfo BuildInfo
}

// newContext creates a fully initialized Context from a config and options.
func newContext(_ context.Context, cfg *config.CLIConfig, opts ContextOpts) *Context {
	u := ui.Detect(os.Stdin, os.Stdout, os.Stderr, cfg.UI.Color == "never")
	noColor := cfg.UI.Color == "never" || cfg.UI.Color == "auto" && !u.IsTTY

	format := output.FormatText
	if cfg.UI.Output == "json" {
		format = output.FormatJSON
	}

	var logOut io.Writer = os.Stderr
	if !opts.Debug {
		logOut = io.Discard
	}
	logger := log.New(logOut, "[aranea] ", log.LstdFlags)

	c := client.NewClient(
		cfg.Backend.BaseURL,
		cfg.Backend.Token,
		opts.BuildInfo.Version,
		opts.Debug,
		func(format string, args ...any) {
			logger.Printf(format, args...)
		},
	)

	printer := output.NewPrinter(format, opts.Quiet, noColor, os.Stdout)

	return &Context{
		Cfg:     cfg,
		Client:  c,
		Printer: printer,
		UI:      u,
		Logger:  logger,
		Debug:   opts.Debug,
		Quiet:   opts.Quiet,
		AutoYes: opts.AutoYes,
		BI:      opts.BuildInfo,
	}
}

// WithCLI stores a Context in the Go context.
func WithCLI(parent context.Context, c *Context) context.Context {
	return context.WithValue(parent, contextKey{}, c)
}

// CLIFrom retrieves the Context from the Go context.
// Panics if not set – commands must be registered under a PersistentPreRunE that sets it.
func CLIFrom(ctx context.Context) *Context {
	v := ctx.Value(contextKey{})
	if v == nil {
		panic("cli.Context not in context; missing PersistentPreRunE setup")
	}
	return v.(*Context)
}
