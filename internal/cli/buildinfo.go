package cli

// BuildInfo carries version information injected at build time via ldflags.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}
